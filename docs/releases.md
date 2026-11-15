# Releases

Source-only releases built from sanitized `main`. No build artifacts or container
images are published.

## Notes

`CHANGELOG.md` is the single source of release notes. Every user-visible change
lands under `Unreleased` in `Added`, `Changed`, `Fixed` or `Security` while it is
developed, and that section becomes the published notes verbatim.

## Cutting a release

1. Confirm `main` is the intended commit and `git status` is clean.
2. Require the four `CI checks` jobs to be green (`secrets`, `go`,
   `ui (pipeline-ui)`, `embedded-ui-and-browser`) plus the browser and worker
   checks described in `CONTRIBUTING.md`.
3. Move the `Unreleased` entries into a new `## [X.Y.Z] - YYYY-MM-DD` section and
   leave an empty `Unreleased` heading above it.
4. Tag the sanitized `main` commit: `git tag -a vX.Y.Z -m "o11y workbench vX.Y.Z"`.
5. Create a GitHub Release from that tag whose body is the matching changelog
   section. GitHub adds the source archives automatically.

Versions follow SemVer. Below 1.0, a minor release may contain documented
breaking changes; patch releases contain fixes only. Tags are immutable; fixes
become the next patch release.

## Tag pushing

The local `pre-push` hook rejects every tag push for
`johndemoss/o11y-workbench`, including `vX.Y.Z`. Relaxing that policy is a
separate, explicit decision: update the hook to allow annotated SemVer tags whose
commit descends from the sanitized baseline, review the change, and only then
push the tag. Never publish tags from the retained private history.
