#!/usr/bin/env bash
# Shared runtime-state helpers for local launchers (serve.sh, demo-sender.sh).
#
# Local process state lives in a private, per-checkout directory instead of a
# predictable path under a shared temporary directory. Sourcing this file only
# defines functions; callers opt in explicitly.
#
#   runtime_prepare PROJECT_DIR
#   runtime_acquire_lock LOCK_DIR LABEL
#   runtime_release_lock LOCK_DIR
#   runtime_install_binary SOURCE DEST
#   runtime_verify_executable PATH
#   runtime_stop PIDFILE EXPECTED_ARGV...
#   runtime_record_pid PIDFILE PID EXPECTED_ARGV...
#
# shellcheck shell=bash

# runtime_stat_mode PATH -> octal mode string (for example 700 or 1777).
runtime_stat_mode() {
  if stat -c '%a' "$1" >/dev/null 2>&1; then
    stat -c '%a' "$1"
  elif stat -f '%Lp' "$1" >/dev/null 2>&1; then
    stat -f '%Lp' "$1"
  else
    return 1
  fi
}

# runtime_stat_uid PATH -> numeric owner uid.
runtime_stat_uid() {
  if stat -c '%u' "$1" >/dev/null 2>&1; then
    stat -c '%u' "$1"
  elif stat -f '%u' "$1" >/dev/null 2>&1; then
    stat -f '%u' "$1"
  else
    return 1
  fi
}

runtime_platform() {
  uname -s
}

# Reject any ancestor of RUN_DIR that other users could replace.
runtime_check_ancestors() {
  local path=$1
  local current=$path
  while true; do
    local parent
    parent=$(dirname "$current")
    [ "$parent" = "$current" ] && break
    current=$parent
    [ -e "$current" ] || continue
    local mode
    mode=$(runtime_stat_mode "$current") || {
      echo "ERROR: cannot read permissions for $current" >&2
      return 1
    }
    local numeric=$((8#$mode))
    if [ $((numeric & 8#022)) -ne 0 ] && [ $((numeric & 8#01000)) -eq 0 ]; then
      echo "ERROR: $current is group/other-writable and not sticky, so another user could replace the runtime directory." >&2
      echo "       Set O11Y_WORKBENCH_RUNTIME_DIR to a directory only you can write." >&2
      return 1
    fi
    [ "$current" = "/" ] && break
  done
  return 0
}

# runtime_prepare PROJECT_DIR -> sets and validates RUN_DIR (created at mode 0700).
runtime_prepare() {
  local project_dir=$1
  umask 077
  if [ -n "${O11Y_WORKBENCH_RUNTIME_DIR:-}" ]; then
    RUN_DIR="${O11Y_WORKBENCH_RUNTIME_DIR}"
    case "$RUN_DIR" in
      /*) ;;
      *)
        echo "ERROR: O11Y_WORKBENCH_RUNTIME_DIR must be an absolute path (got '$RUN_DIR')" >&2
        return 1
        ;;
    esac
  else
    RUN_DIR="$project_dir/.run"
  fi
  local parent name
  parent=$(cd -P -- "$(dirname "$RUN_DIR")" && pwd -P) || {
    echo "ERROR: runtime directory parent must exist and be accessible" >&2
    return 1
  }
  name=$(basename "$RUN_DIR")
  RUN_DIR="$parent/$name"
  runtime_check_ancestors "$RUN_DIR" || return 1


  if [ -L "$RUN_DIR" ]; then
    echo "ERROR: $RUN_DIR is a symlink; refusing to use it as the runtime directory." >&2
    return 1
  fi

  if [ -e "$RUN_DIR" ] && [ ! -d "$RUN_DIR" ]; then
    echo "ERROR: $RUN_DIR exists and is not a directory." >&2
    return 1
  fi

  if [ ! -d "$RUN_DIR" ]; then
    # mkdir is atomic and never moves a losing initializer inside the winner.
    if ! mkdir -m 700 "$RUN_DIR" 2>/dev/null && [ ! -d "$RUN_DIR" ]; then
      echo "ERROR: cannot create runtime directory $RUN_DIR" >&2
      return 1
    fi
  fi

  if [ -L "$RUN_DIR" ] || [ ! -d "$RUN_DIR" ]; then
    echo "ERROR: $RUN_DIR is not a usable runtime directory." >&2
    return 1
  fi

  local owner mode
  owner=$(runtime_stat_uid "$RUN_DIR") || {
    echo "ERROR: cannot read the owner of $RUN_DIR" >&2
    return 1
  }
  if [ "$owner" != "$(id -u)" ]; then
    echo "ERROR: $RUN_DIR is owned by uid $owner, not $(id -u)." >&2
    return 1
  fi
  mode=$(runtime_stat_mode "$RUN_DIR") || {
    echo "ERROR: cannot read the permissions of $RUN_DIR" >&2
    return 1
  }
  if [ "$mode" != "700" ]; then
    echo "ERROR: $RUN_DIR has mode $mode; expected 700 so other users cannot read or replace local process state." >&2
    return 1
  fi

  export RUN_DIR
  return 0
}

# runtime_acquire_lock LOCK_DIR LABEL -> creates the lock or fails with guidance.
runtime_acquire_lock() {
  local lock_dir=$1 label=$2
  if mkdir "$lock_dir" 2>/dev/null; then
    return 0
  fi
  echo "ERROR: another launcher holds the $label lock ($lock_dir)." >&2
  echo "       Wait for it to finish; if no launcher is running, remove that directory." >&2
  return 1
}

runtime_release_lock() {
  local lock_dir=$1
  [ -n "$lock_dir" ] || return 0
  rmdir "$lock_dir" 2>/dev/null || true
}

# runtime_verify_executable PATH -> owned, non-symlink, executable regular file.
runtime_verify_executable() {
  local path=$1
  if [ -L "$path" ]; then
    echo "ERROR: $path is a symlink; refusing to run it." >&2
    return 1
  fi
  if [ ! -f "$path" ] || [ ! -x "$path" ]; then
    echo "ERROR: $path is not an executable regular file." >&2
    return 1
  fi
  local owner
  owner=$(runtime_stat_uid "$path") || {
    echo "ERROR: cannot read the owner of $path" >&2
    return 1
  }
  if [ "$owner" != "$(id -u)" ]; then
    echo "ERROR: $path is owned by uid $owner, not $(id -u)." >&2
    return 1
  fi
  return 0
}

# runtime_install_binary SOURCE DEST -> mode 0700 plus atomic rename within RUN_DIR.
runtime_install_binary() {
  local source=$1 dest=$2
  chmod 700 "$source" || return 1
  mv -f "$source" "$dest" || return 1
  chmod 700 "$dest" || return 1
  return 0
}

runtime_json_string() {
  local value=$1
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  printf '"%s"' "$value"
}

runtime_proc_start_ticks() {
  local pid=$1 stat rest
  stat=$(cat "/proc/$pid/stat" 2>/dev/null) || return 1
  rest=${stat##*) }
  [ "$rest" != "$stat" ] || return 1
  # Field 3 is the first token after comm, so starttime (field 22) is the 20th.
  # shellcheck disable=SC2086
  set -- $rest
  [ $# -ge 20 ] || return 1
  printf '%s' "${20}"
}

runtime_proc_cmdline() {
  local pid=$1
  python3 - "$pid" <<'PY'
import sys
try:
    with open(f"/proc/{sys.argv[1]}/cmdline", "rb") as handle:
        data = handle.read()
except OSError:
    sys.exit(1)
parts = [part.decode("utf-8", "surrogateescape") for part in data.split(b"\0") if part]
if not parts:
    sys.exit(1)
sys.stdout.write("\n".join(parts))
PY
}

# runtime_pidfile_pid PIDFILE -> prints the recorded pid, or fails.
runtime_pidfile_pid() {
  local content
  content=$(cat "$1" 2>/dev/null) || return 1
  case "$content" in
    *'"pid":'*) ;;
    *) return 1 ;;
  esac
  content=${content#*'"pid":'}
  content=${content%%,*}
  content=${content//[!0-9]/}
  [ -n "$content" ] || return 1
  printf '%s' "$content"
}

runtime_pidfd_available() {
  [ "$(runtime_platform)" = "Linux" ] || return 1
  [ -d /proc ] || return 1
  command -v python3 >/dev/null 2>&1 || return 1
  python3 -c 'import os, signal; f = os.pidfd_open(os.getpid()); signal.pidfd_send_signal(f, 0); open("/proc/self/stat").read(); open("/proc/self/cmdline", "rb").read(); os.close(f)' >/dev/null 2>&1
}

# runtime_record_pid PIDFILE PID EXPECTED_ARGV...
# Verifies a freshly started daemon owns the expected command line, then records
# the pid and (on Linux) its start time so a later stop cannot signal a reused pid.
runtime_record_pid() {
  local pidfile=$1 pid=$2
  shift 2
  local expected=("$@")
  [ "${#expected[@]}" -gt 0 ] || return 1

  local start_ticks="null"
  if [ "$(runtime_platform)" = "Linux" ]; then
    local want got attempt=0
    want=$(printf '%s\n' "${expected[@]}")
    want=${want%$'\n'}
    while [ "$attempt" -lt 40 ]; do
      got=$(runtime_proc_cmdline "$pid" 2>/dev/null || true)
      got=${got%$'\n'}
      if [ -n "$got" ] && [ "$got" = "$want" ]; then
        break
      fi
      if ! kill -0 "$pid" 2>/dev/null; then
        echo "ERROR: process $pid exited before its command line could be verified." >&2
        return 1
      fi
      sleep 0.05
      attempt=$((attempt + 1))
    done
    if [ -z "$got" ] || [ "$got" != "$want" ]; then
      echo "ERROR: could not verify the command line of pid $pid." >&2
      return 1
    fi
    start_ticks=$(runtime_proc_start_ticks "$pid") || {
      echo "ERROR: cannot read the start time of pid $pid." >&2
      return 1
    }
  fi

  local argv_json="" value escaped
  for value in "${expected[@]}"; do
    escaped=$(runtime_json_string "$value")
    if [ -z "$argv_json" ]; then
      argv_json="$escaped"
    else
      argv_json="$argv_json, $escaped"
    fi
  done

  local tmp="$pidfile.tmp.$$"
  printf '{"pid": %s, "start_ticks": %s, "argv": [%s]}\n' "$pid" "$start_ticks" "$argv_json" > "$tmp" || return 1
  chmod 600 "$tmp" || return 1
  mv -f "$tmp" "$pidfile" || return 1
  return 0
}

# runtime_stop PIDFILE EXPECTED_BINARY
# Succeeds when nothing is running or a verified process was stopped. Never
# signals a pid whose live identity does not match the recorded process.
runtime_stop() {
  local pidfile=$1
  local binary=$2

  if [ ! -e "$pidfile" ]; then
    return 0
  fi
  if [ -L "$pidfile" ] || [ ! -f "$pidfile" ]; then
    echo "ERROR: refusing to use $pidfile because it is not a regular file." >&2
    return 1
  fi

  if [ -z "$binary" ]; then
    echo "ERROR: runtime_stop needs the expected executable path." >&2
    return 1
  fi

  if [ "$(runtime_platform)" = "Linux" ]; then
    command -v python3 >/dev/null 2>&1 || {
      echo "ERROR: python3 is required to stop the launcher safely on Linux." >&2
      return 1
    }
    local helper
    helper="$(dirname "${BASH_SOURCE[0]}")/safe-stop.py"
    [ -f "$helper" ] || {
      echo "ERROR: missing helper $helper" >&2
      return 1
    }
    if python3 "$helper" "$pidfile" "$binary"; then
      return 0
    fi
    return 1
  fi

  # macOS: no race-free process handle is available, so never signal by pid.
  local pid
  pid=$(runtime_pidfile_pid "$pidfile" || true)
  if [ -z "$pid" ]; then
    echo "ERROR: cannot read a pid from $pidfile; leaving it in place." >&2
    return 1
  fi
  if ps -p "$pid" >/dev/null 2>&1; then
    echo "ERROR: pid $pid is still running, and this platform cannot be stopped safely." >&2
    echo "       Stop it manually (for example: kill $pid) and then remove $pidfile." >&2
    return 1
  fi
  rm -f "$pidfile"
  return 0
}

# Executed by Make only for Podman Compose commands; sourcing remains inert.
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  set -euo pipefail
  [ "${1:-}" = compose ] || exit 2
  shift
  project_dir=$(cd -P -- "$(dirname "$0")/.." && pwd -P)
  runtime_prepare "$project_dir"
  config_dir=$(mktemp -d "$RUN_DIR/docker-config.XXXXXX")
  trap 'rm -rf -- "$config_dir"' EXIT
  printf '{}\n' > "$config_dir/config.json"
  export DOCKER_CONFIG="$config_dir"
  "$@"
fi
