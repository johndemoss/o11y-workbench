#!/usr/bin/env bash
# Fail if container images or host publishes are not pinned or loopback-only.
# This does not scan history and does not prove absence of secrets.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

python3 - <<'PY'
import pathlib, re, sys
fail = False
digest = "sha256:e40d61f10c9a617cca1ffd1be4d7e2b3a4b0f0001798ff07e58cabe82e618cea"
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
odv_digest = "sha256:e1bcd2a895c0e62b1ae97988cbf132d270074cb239b5c37efe59348a60a82ce8"
if pathlib.Path(".github/workflows/build-otel-desktop-viewer.yml").exists():
    print("build-otel-desktop-viewer workflow must not publish viewer binaries", file=sys.stderr)
    fail = True
compose_path = pathlib.Path("docker-compose.yml")
if compose_path.exists():
    compose = compose_path.read_text()
    if "otel-desktop-viewer:" in compose:
        if odv_digest not in compose:
            print(f"docker-compose.yml: otel-desktop-viewer image is not pinned to {odv_digest}", file=sys.stderr)
            fail = True
        if "ghcr.io/ctrlspice/otel-desktop-viewer:v0.7.0" not in compose:
            print("docker-compose.yml: otel-desktop-viewer must use upstream ghcr.io/ctrlspice/otel-desktop-viewer:v0.7.0", file=sys.stderr)
            fail = True
        if re.search(r"ghcr\.io/ctrlspice/otel-desktop-viewer[^@\n]*:latest", compose):
            print("docker-compose.yml: otel-desktop-viewer must not use a floating latest tag", file=sys.stderr)
            fail = True
        if "Dockerfile.otel-desktop-viewer" in compose:
            print("docker-compose.yml: must not build a local otel-desktop-viewer image", file=sys.stderr)
            fail = True
sys.exit(1 if fail else 0)
PY

echo "publication tree: container images pinned and publishes loopback-only"