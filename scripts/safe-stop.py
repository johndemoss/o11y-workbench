#!/usr/bin/env python3
"""Stop a launcher-managed local process without the pid-reuse race.

The launcher records a small JSON PID file:

    {"pid": 1234, "start_ticks": 5678, "argv": ["/path/to/binary", "--addr", "..."]}

This helper opens a pidfd handle first, verifies the recorded identity against
/proc, and only then signals through that handle. If any check fails, or the
platform cannot provide a race-free handle, it refuses to signal and leaves the
PID file in place.
"""

import json
import os
import select
import signal
import sys
import time

WAIT_SECONDS = 5.0


def fail(message):
    print(f"ERROR: {message}", file=sys.stderr)
    sys.exit(1)


def read_record(pidfile):
    try:
        with open(pidfile, encoding="utf-8") as handle:
            record = json.load(handle)
    except FileNotFoundError:
        return None
    except (OSError, ValueError) as exc:
        fail(f"cannot read {pidfile}: {exc}")
    if not isinstance(record, dict):
        fail(f"{pidfile} does not contain a PID record")
    return record


def read_start_ticks(pid):
    try:
        with open(f"/proc/{pid}/stat", encoding="utf-8", errors="surrogateescape") as handle:
            stat = handle.read()
    except OSError as exc:
        fail(f"cannot read /proc/{pid}/stat: {exc}")
    tail = stat.rsplit(") ", 1)
    if len(tail) != 2:
        fail(f"cannot parse /proc/{pid}/stat")
    fields = tail[1].split()
    if len(fields) < 20:
        fail(f"cannot parse /proc/{pid}/stat")
    try:
        return int(fields[19])
    except ValueError:
        fail(f"cannot parse the start time of pid {pid}")


def read_cmdline(pid):
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as handle:
            data = handle.read()
    except OSError as exc:
        fail(f"cannot read /proc/{pid}/cmdline: {exc}")
    parts = data.split(b"\0")
    if parts and parts[-1] == b"":
        parts = parts[:-1]
    return [part.decode("utf-8", "surrogateescape") for part in parts]


def stop_child(pid, parent, expected):
    """Stop only our still-identifiable, freshly spawned child through a pidfd."""
    try:
        handle = os.pidfd_open(pid)
    except ProcessLookupError:
        return 0
    except (OSError, AttributeError) as exc:
        fail(f"cannot open a handle for child {pid}: {exc}")
    try:
        try:
            owner = os.stat(f"/proc/{pid}").st_uid
            with open(f"/proc/{pid}/stat", encoding="utf-8", errors="surrogateescape") as stream:
                fields = stream.read().rsplit(") ", 1)[1].split()
            ppid = int(fields[1])
        except (OSError, IndexError, ValueError) as exc:
            fail(f"cannot verify child {pid}: {exc}")
        if owner != os.getuid() or ppid != parent or read_cmdline(pid) != expected:
            fail(f"child {pid} no longer matches its expected identity; refusing to signal")
        try:
            signal.pidfd_send_signal(handle, signal.SIGTERM)
        except ProcessLookupError:
            return 0
        except OSError as exc:
            fail(f"cannot signal child {pid}: {exc}")
        if not select.select([handle], [], [], WAIT_SECONDS)[0]:
            fail(f"child {pid} did not exit within {WAIT_SECONDS:g}s")
    finally:
        os.close(handle)
    return 0


def main():
    if len(sys.argv) >= 5 and sys.argv[1] == "--child":
        try:
            pid, parent = int(sys.argv[2]), int(sys.argv[3])
        except ValueError:
            fail("invalid child or parent pid")
        return stop_child(pid, parent, sys.argv[4:])
    if len(sys.argv) != 3:
        fail("usage: safe-stop.py PIDFILE EXPECTED_BINARY")
    pidfile = sys.argv[1]
    expected_binary = sys.argv[2]

    record = read_record(pidfile)
    if record is None:
        return 0

    pid = record.get("pid")
    if not isinstance(pid, int) or isinstance(pid, bool) or pid <= 0:
        fail(f"{pidfile} has no usable pid; refusing to signal anything")

    if not hasattr(os, "pidfd_open") or not hasattr(signal, "pidfd_send_signal"):
        fail("this platform cannot open a race-free process handle; stop the process manually")

    try:
        handle = os.pidfd_open(pid)
    except ProcessLookupError:
        # The process is already gone; drop the stale record.
        os.unlink(pidfile)
        return 0
    except OSError as exc:
        fail(f"cannot open a handle for pid {pid}: {exc}")

    try:
        try:
            owner = os.stat(f"/proc/{pid}").st_uid
        except OSError as exc:
            fail(f"cannot inspect pid {pid}: {exc}")
        if owner != os.getuid():
            fail(f"pid {pid} is owned by uid {owner}, not {os.getuid()}; leaving it alone")

        recorded_ticks = record.get("start_ticks")
        if not isinstance(recorded_ticks, int) or isinstance(recorded_ticks, bool):
            fail(f"{pidfile} has no recorded start time; refusing to signal pid {pid}")
        if read_start_ticks(pid) != recorded_ticks:
            fail(f"pid {pid} does not match the recorded start time; refusing to signal it")

        recorded_argv = record.get("argv")
        if not isinstance(recorded_argv, list) or not all(isinstance(item, str) for item in recorded_argv):
            fail(f"{pidfile} has no recorded command line; refusing to signal pid {pid}")
        if read_cmdline(pid) != recorded_argv:
            fail(f"pid {pid} does not match the recorded command line; refusing to signal it")
        if not recorded_argv or recorded_argv[0] != expected_binary:
            fail(f"{pidfile} records a different executable; refusing to signal pid {pid}")

        try:
            signal.pidfd_send_signal(handle, signal.SIGTERM)
        except OSError as exc:
            fail(f"cannot signal pid {pid}: {exc}")

        deadline = time.monotonic() + WAIT_SECONDS
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                fail(f"pid {pid} did not exit within {WAIT_SECONDS:g}s; leaving {pidfile} in place")
            readable, _, _ = select.select([handle], [], [], min(0.2, remaining))
            if readable:
                break
    finally:
        os.close(handle)

    os.unlink(pidfile)
    return 0


if __name__ == "__main__":
    sys.exit(main())
