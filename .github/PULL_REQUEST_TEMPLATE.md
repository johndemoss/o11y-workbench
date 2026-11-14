# Pull requests

## What changed

Describe the behavior that changed and why. Keep it about the product and the
shipped change.

## Verification

List the commands you ran and their outcome (for example
`GOTOOLCHAIN=go1.26.11 ./scripts/check-go.sh`, `cd pipeline-ui && npm test`,
`npm run test:browser`). Say which checks you could not run and why.

## Checklist

- [ ] `./scripts/check-publication.sh` passes.
- [ ] Go checks pass for all three modules.
- [ ] Pipeline UI tests and build pass when UI code changed.
- [ ] Documentation and help text match the new behavior.
- [ ] No real telemetry, credentials, or captured data is included.
