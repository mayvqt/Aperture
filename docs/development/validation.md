# Validation

Use the smallest focused check that covers the changed contract:

| Change | Focused check |
| --- | --- |
| CLI behavior | `go test ./cmd/aperture` |
| SQLite/schema | `go test ./internal/db` |
| HTTP, templates, CSS, or JavaScript | `go test ./internal/httpserver` plus browser checks below |
| Jellyfin/Emby protocol | `go test ./internal/mediaserver/...` |
| Configuration/security | `go test ./internal/config ./internal/security` |
| Entrypoint | `sh scripts/test-entrypoint.sh` |
| Release binary | `CGO_ENABLED=0 go build -trimpath -o /tmp/aperture ./cmd/aperture` |

Format touched Go files with `gofmt -w`. Performance or refactor claims require a
representative before/after benchmark or trace and a regression threshold; a
clean test run alone is not performance evidence.

Media-server policy fixtures cover complete target defaults, case-insensitive
overrides, duplicate access flags, and disabling an account after an ambiguous
failure. Keep these checks when changing template import or application.

Account lifecycle regressions cover interrupted password setup, lost policy and
completion responses, exhausted template retries with pending cleanup, fixed
expiry, operation collisions, and fresh/legacy/rollback migration behavior with
encrypted-value preservation.

## UI evidence

Rebuild before visual checks because templates, CSS, and JavaScript are embedded.
Use a disposable database and synthetic users/invites. For every affected flow,
check desktop and narrow mobile widths, keyboard-only navigation, visible focus,
labels and error association, and loading, empty, validation-error, upstream-error,
and permission-denied states. Cover setup, login, invite registration, and the
affected admin page. Do not use real credentials or production data.

## Prerequisites and final gate

Use the Go version in `go.mod`. Do not install missing tools or download
dependencies without approval. Reuse local dependencies only when `go.sum`, the
Go toolchain, OS/architecture, and installed module tree match the revision being
validated; otherwise fail closed and use a clean locked environment.

The one comprehensive gate is the complete GitHub Actions `CI` workflow in
`.github/workflows/ci.yml` for the exact revision. It checks formatting,
whitespace, `go test ./...`, the entrypoint, vet, race detection, pinned
Staticcheck and govulncheck versions, a release-style build, and the Docker image.
Do not claim release readiness until both jobs pass on that revision.

Origin regressions cover upgrades from revisions 1–5, encrypted invite preservation,
rollback, unknown ownership, cloned IDs at different URLs, replacement servers,
API-key rotation, session revocation, lost settings acknowledgements, optional-key
login, scoped work queues and paginated review of old records.
