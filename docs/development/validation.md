# Validation

The one comprehensive gate is the complete GitHub Actions `CI` workflow,
[`.github/workflows/ci.yml`](../../.github/workflows/ci.yml), on the exact
revision; its steps are the list of checks, and the release-style build command
is there. Do not claim release readiness until both of its jobs pass on that
revision.

While working, run the smallest check that covers the changed contract:
`go test` on the touched packages (tests live beside each package; use
`./internal/mediaserver/...` for provider protocol changes),
`sh scripts/test-entrypoint.sh` for the container entrypoint, and
`scripts/check-docs.sh` for docs. HTTP, template, CSS, and JavaScript changes
also need the [UI evidence](#ui-evidence) below. CI rejects unformatted Go, so
run `gofmt -w` on touched files.

Performance or refactor claims require a representative before/after benchmark
or trace and a regression threshold; a clean test run alone is not performance
evidence.

## Regression coverage to keep

- Media-server policy fixtures: complete target defaults, case-insensitive
  overrides, duplicate access flags, and disabling an account after an
  ambiguous failure. Keep them when changing template import or application.
- Account lifecycle: interrupted password setup, lost policy and completion
  responses, exhausted template retries with pending cleanup, fixed expiry,
  operation collisions, and fresh/legacy/rollback migration behavior with
  encrypted-value preservation.
- Server ownership: upgrades from revisions 1–5, encrypted invite preservation,
  rollback, unknown ownership, cloned IDs at different URLs, replacement
  servers, API-key rotation, session revocation, lost settings
  acknowledgements, optional-key login, scoped work queues, and paginated
  review of old records.

## UI evidence

Rebuild before visual checks because templates, CSS, and JavaScript are embedded.
Use a disposable database and synthetic users/invites. For every affected flow,
check desktop and narrow mobile widths, keyboard-only navigation, visible focus,
labels and error association, and loading, empty, validation-error, upstream-error,
and permission-denied states. Cover setup, login, invite registration, and the
affected admin page. Do not use real credentials or production data.

## Prerequisites

Use the Go version in `go.mod`. Do not install missing tools or download
dependencies without approval. Reuse local dependencies only when `go.sum`, the
Go toolchain, OS/architecture, and installed module tree match the revision being
validated; otherwise fail closed and use a clean locked environment.
