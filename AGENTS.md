# o11y workbench — Agent Guide

## Server Launch Protocol

**Always use `scripts/serve.sh` to start the web server after making changes.**

```bash
./scripts/serve.sh
```

This builds the frontend + Go binary, starts the server, and **waits until it's accepting connections** before returning. Exit 0 = ready, exit 1 = failure.

**Do not** use `go run`, `go build && ./binary & sleep N`, or any other chaining pattern — those are unreliable in automation and will hang or fail silently.

Full protocol documentation: `.sisyphus/agent-guidance.md`

## Health Check

Once the server is running, `GET /api/health` returns `{"status": "ok"}`. Poll this to confirm readiness.

## Cleanup

Kill the server when done:
```bash
kill "$(cat /tmp/o11y-workbench-web.pid)" 2>/dev/null
```
