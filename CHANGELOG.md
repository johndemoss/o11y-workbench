# Changelog

Notable changes to the o11y workbench. The project is developed on `main`; notes
here describe user-visible behavior rather than internal work.

## Unreleased

### Security

- The web server rejects requests whose `Host` header is not `localhost`, a
  loopback address, the address it was started with, or an explicit
  `--allowed-hosts` entry, so a page on another origin cannot reach the local
  API through DNS rebinding.
- Every response now carries a restricting `Content-Security-Policy` plus
  `X-Content-Type-Options`, `Referrer-Policy`, and `X-Frame-Options`. The
  dashboard no longer relies on inline event handlers or inline scripts.
- Dashboard fields that come from scenario names, run failures, and telemetry
  (log severity, log and metric attributes) are rendered as text instead of
  markup.
- Local launchers keep their built binaries, PID records, and locks in a private
  per-checkout `.run/` directory instead of predictable paths under the system
  temporary directory, verify process identity before stopping a recorded
  process, and refuse to signal a process they cannot verify.

### Added

- `--allowed-hosts` on the web binary and `./scripts/serve.sh` for additional
  exact hostnames or IP literals.
- `./scripts/serve.sh --stop`, which safely stops the daemon started from this
  checkout.
- `./scripts/demo-sender.sh start|stop` for the demo telemetry sender, with
  output kept in the private runtime directory.
- Weekly Dependabot updates for the Go modules, both npm manifests, and GitHub
  Actions, plus issue and pull-request templates, `CODEOWNERS`, and a code of
  conduct.
