# Codebase map

| Path | Ownership |
| --- | --- |
| `cmd/aperture` | CLI dispatch, configuration startup, logging, process lifecycle, and version output. |
| `internal/config` | Environment/flag parsing, defaults, URL validation, and generated encryption-key bootstrap. |
| `internal/connection` | Effective configuration, immutable operation snapshots, server identity verification and atomic connection publication. |
| `internal/db` | SQLite schema, migrations, settings, invites, templates, sessions, registrations, managed users, and audit records. |
| `internal/httpserver` | Routes, middleware, browser workflows, embedded templates/assets, shared account recovery, webhooks, and maintenance coordination. |
| `internal/mediaserver` | Provider-neutral contracts and URL rules. |
| `internal/mediaserver/jellyfin`, `emby`, `protocol`, `router` | Provider adapters, HTTP protocol, and immutable adapter factory. |
| `internal/security` | Encryption, token helpers, and diagnostic redaction. |
| `scripts`, `docker-entrypoint.sh`, `Dockerfile`, `docker-compose.yml` | Container build, startup, ownership, and regression checks. |
| `templates/unraid` | Unraid application template. |
| `.github/workflows` | CI, image publication, dependency updates, and release announcements. |

The nearest tests live beside their packages. Presentation sources are
`internal/httpserver/templates` and `internal/httpserver/assets`; both are
embedded into the binary.

Update this map when a top-level domain is added or ownership moves.
