#!/usr/bin/env bash
# Fail if container images or host publishes are not pinned or loopback-only.
# This does not scan history and does not prove absence of secrets.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

python3 - <<'PY'
import pathlib, re, sys
fail = False
digest = "sha256:d57bfe8eee2378f31cb1193239fbcac521d54a5a071fca2bfc106916a32b892d"
for path in list(pathlib.Path(".").glob("Dockerfile*")) + list(pathlib.Path(".").glob("docker-compose*.yml")) + [pathlib.Path("Makefile")]:
    text = path.read_text()
    if "opentelemetry-collector-contrib" in text and digest not in text:
        print(f"{path}: collector image is not pinned to {digest}", file=sys.stderr)
        fail = True
    in_ports = False
    for lineno, line in enumerate(text.splitlines(), 1):
        stripped = line.strip()
        if stripped.startswith("ports:"):
            in_ports = True
            continue
        if in_ports and stripped and not stripped.startswith("-") and not stripped.startswith("#"):
            in_ports = False
        if in_ports and re.search(r"\d+:\d+", stripped) and "127.0.0.1:" not in stripped and "[::1]:" not in stripped:
            print(f"{path}:{lineno}: host port is not loopback-only", file=sys.stderr)
            fail = True
        if path.name == "Makefile" and re.search(r"(^|\s)-p \d+:\d+", stripped) and "127.0.0.1:" not in stripped:
            print(f"{path}:{lineno}: published port is not loopback-only", file=sys.stderr)
            fail = True
for path in pathlib.Path(".").glob("Dockerfile*"):
    for lineno, line in enumerate(path.read_text().splitlines(), 1):
        if line.startswith("FROM ") and "@sha256:" not in line:
            print(f"{path}:{lineno}: base image is not digest-pinned", file=sys.stderr)
            fail = True
workflow = pathlib.Path(".github/workflows/build-otel-desktop-viewer.yml").read_text()
if "d540ad65882786fd795adb584201d8ad87810180" not in workflow:
    print("desktop-viewer workflow is not pinned to v0.7.0", file=sys.stderr)
    fail = True
sys.exit(1 if fail else 0)
PY

echo "publication tree: container images pinned and publishes loopback-only"