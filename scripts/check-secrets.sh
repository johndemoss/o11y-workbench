#!/usr/bin/env bash
# Scan without printing candidate secret values or uploading repository content.
set -euo pipefail
umask 077

cd "$(git rev-parse --show-toplevel)"
GITLEAKS_BIN="${GITLEAKS_BIN:-gitleaks}"
command -v "$GITLEAKS_BIN" >/dev/null || {
  echo "Install Gitleaks v8.24.3 (see CONTRIBUTING.md)." >&2
  exit 2
}
AUDIT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/o11y-secrets.XXXXXX")"
trap 'rm -rf "$AUDIT_DIR"' EXIT

# Only tracked paths: do not scan node_modules, private scratchpads or build output.
# Never dereference symlinks that might point outside the publication tree.
python3 - "$AUDIT_DIR/tree" <<'PY'
import os
import pathlib
import shutil
import subprocess
import sys
root = pathlib.Path(sys.argv[1])
root.mkdir()
for name in subprocess.check_output(["git", "ls-files", "-z"]).split(b"\0"):
    if not name:
        continue
    path = pathlib.Path(os.fsdecode(name))
    if path.is_symlink() or not path.is_file():
        continue
    dest = root / path
    dest.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(path, dest)
PY

failed=0
for scope in tree history; do
  if [[ "$scope" == tree ]]; then
    args=(dir "$AUDIT_DIR/tree")
  else
    args=(git . --log-opts=--all)
  fi
  rc=0
  "$GITLEAKS_BIN" "${args[@]}" --redact=100 --report-format json \
    --report-path "$AUDIT_DIR/$scope.json" >"$AUDIT_DIR/$scope.log" 2>&1 || rc=$?
  if [[ "$rc" -eq 0 ]]; then
    echo "$scope: no secret matches detected"
  elif [[ "$rc" -eq 1 && -f "$AUDIT_DIR/$scope.json" ]]; then
    # Deliberately omit Match, Secret, context, author and commit messages.
    python3 - "$scope" "$AUDIT_DIR/$scope.json" <<'PY'
import json
import sys
findings = json.load(open(sys.argv[2]))
print(f"{sys.argv[1]}: {len(findings)} possible secret(s); publication blocked")
for finding in findings:
    print(json.dumps({key: finding.get(key) for key in
                      ("RuleID", "File", "StartLine", "Commit")}))
PY
    failed=1
  else
    echo "$scope: scanner failed (exit $rc); publication blocked. Diagnose locally with --redact=100." >&2
    failed=2
  fi
done
exit "$failed"
