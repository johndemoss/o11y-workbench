# Security policy

## Intended deployment

This is a **trusted, single-user development workbench**, not an authenticated
multi-user service. Public source availability does not make the HTTP server safe
to expose to the Internet, an untrusted LAN, or arbitrary reverse-proxy clients.
Only the current main branch is maintained; there is no supported-release SLA.

- The web binary and local launcher bind to `127.0.0.1:18080` by default.
  Explicit `--addr` overrides are an operator responsibility.
- Go's `http.CrossOriginProtection` rejects unsafe cross-origin browser requests.
  This is **not authentication**, an SSRF defense, a DNS-rebinding defense, or a
  resource quota. Non-browser callers without an Origin header remain allowed.
- The API can generate traffic, connect to caller-selected OTLP/Remote TAP
  endpoints, process imported data, and invoke configured local OTTL worker
  processes. Do not grant access to untrusted clients or import untrusted
  collector configs.
- HTTP request bodies are capped at 32 MiB.
  Generation counts, rates and durations still have no comprehensive budget.
  Run the workbench with restricted network access and OS resource limits.
- Per-run destinations are isolated; server shutdown cancels/joins generated jobs
  and finalizes/closes TAP capture. These lifecycle guarantees do not provide
  multi-user authorization, quotas or comprehensive per-handler budgets; do not
  infer multi-tenant safety from routing isolation.
- Use synthetic telemetry. Real traces, logs, attributes, headers, imported
  configurations, TAP captures, screenshots and terminal output can contain
  credentials or personal/customer data. Do not commit them.
- Docker listens on all container interfaces intentionally. Compose examples
  and `make otel-start` publish host ports on `127.0.0.1` only. Workbench
  images run as uid 65532 with a read-only root and a tmpfs `/tmp`. The
  upstream collector keeps its own user and is memory-limited.
- The optional upstream OTel Desktop Viewer image runs as root inside its
  container (its executable is under `/root`). Its Compose service uses a
  read-only root, tmpfs `/tmp`, no new privileges, resource limits, and
  loopback-only published ports. Do not expose its OTLP or UI ports to
  untrusted clients. `make dev-remotetap` does not start the viewer.

## Reporting a vulnerability

Do **not** include credentials, captured telemetry, private hostnames, customer
data, or exploit details in a public issue or pull request.

This repository does not currently use [GitHub private vulnerability
reporting](https://docs.github.com/code-security/security-advisories/working-with-repository-security-advisories/configuring-private-vulnerability-reporting-for-a-repository).
Ask the maintainer for a private reporting channel without posting sensitive
details in a public issue or pull request.

If a credential is discovered in Git, revoke/rotate it first. Removing it from
the working tree is not sufficient: assess every branch/tag, historical object,
GitHub release, Actions artifact/log, fork and cache. History rewrites
and remote changes require maintainer coordination; do not push an incidental fix
that leaves a credential valid in history.

## Local checks

Run `./scripts/check-secrets.sh` with Gitleaks v8.24.3 installed. It scans current
tracked files and all locally reachable Git history, with fully redacted reports.
It does not upload repository content or scan ignored files, remote-only refs,
GitHub attachments, binary/image contents or secrets that lack recognizable forms.
No automated scanner proves absence of secrets. Review local changes and
fetched remote refs separately.
