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
  endpoints, process imported data, and run admitted local evaluation binaries.
  Do not grant access to untrusted clients or import untrusted collector configs.
- Several request bodies and generation parameters lack comprehensive budgets.
  Run the workbench with restricted network access and OS resource limits.
- Concurrent web jobs currently share mutable endpoint routing. Do not rely on
  destination isolation across overlapping runs.
- Use synthetic telemetry. Real traces, logs, attributes, headers, imported
  configurations, TAP captures, evaluation artifacts, screenshots and terminal
  output can contain credentials or personal/customer data. Do not commit them.
- Docker listens on all container interfaces intentionally. Publish its host
  port on loopback only, for example `-p 127.0.0.1:18080:8080`. Review all Compose
  port mappings before running; not every example is hardened.

## Reporting a vulnerability

Do **not** include credentials, captured telemetry, private hostnames, customer
data, or exploit details in a public issue or pull request.

Use [GitHub private vulnerability reporting](https://github.com/johndemoss/o11y-workbench/security/advisories/new)
when enabled. If unavailable, ask the maintainer for a private reporting channel
without posting sensitive details. 

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
