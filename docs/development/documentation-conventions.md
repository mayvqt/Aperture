# Documentation conventions

Docs keep what the code can't say: intent, decisions, contracts, and traps.
Anything derivable is generated or linked to its source of truth instead of
copied. Pages describe current, shipped behavior; history lives in git.

Write for one audience and one task per page, and keep commands executable.
Unfinished work belongs in a dedicated roadmap only when it has an accepted
scope; confirmed defects belong in a known-issues page; release notes describe
one shipped audience-facing revision. Move facts between those pages rather
than duplicating them.

For large, high-risk, or external-integration work, keep the accepted plan
immutable and record execution/evidence separately. Inventory each operation,
verify upstream contracts against canonical sources, use deterministic fakes,
and make real-service smoke tests explicit and opt-in. Record only reproduced
defects.

## Keeping docs current

- Update docs once per feature, when committing it, not after every step.
- Run `scripts/doc_owners [<base>]` to list the pages and sections that own the
  changed paths (committed since `<base>`, default `origin/main`, plus
  uncommitted and untracked files). Open only those.
- Never hand-edit a `<!-- generated: ... -->` block; rerun its generator
  (`scripts/gen-codebase-map.sh` for the code map).
- Run `scripts/check-docs.sh` before pushing; CI runs it too. It fails on a
  stale generated block, a package without a doc comment, a broken relative
  link or anchor, a page no other page links to, and an impact-map path that
  matches no tracked file.

## Documentation impact map

`scripts/doc_owners` reads this table: it matches changed files against the
`Paths` patterns (shell `case` patterns, so `*` also matches `/`) and prints
the links in `Pages`. Text outside the links is guidance for the reviewer.
Add a row when a new documentation domain is introduced.

| Change | Paths | Pages |
| --- | --- | --- |
| User-visible behavior | `internal/httpserver/*`, `internal/mediaserver/*` | [Capabilities](../capabilities.md), [Configuration](../configuration.md), and any other user page describing the flow |
| Configuration or setup | `internal/config/*`, `.env.example`, `docker-compose.yml`, `docker-entrypoint.sh`, `templates/unraid/*` | [Configuration](../configuration.md), [Setup](../setup.md), [Unraid](../unraid.md) |
| Package ownership or architecture | `cmd/*`, `go.mod`, `internal/connection/*`, `internal/httpserver/routes.go`, `internal/httpserver/server.go`, `internal/mediaserver/mediaserver.go`, `internal/mediaserver/router/*` | [Architecture](architecture.md), [Codebase map](codebase-map.md#other-paths) (the package table is generated) |
| Schema, retention, encryption, or stored data | `internal/db/*`, `internal/security/crypto.go` | [Data](data.md), and [Operations](operations.md) when operator impact changes |
| Tests, tools, UI states, or CI | `*_test.go`, `scripts/*`, `.github/workflows/ci.yml`, `internal/httpserver/templates/*`, `internal/httpserver/assets/*` | [Validation](validation.md) |
| Security boundary or disclosure process | `internal/security/*`, `internal/httpserver/middleware.go`, `internal/httpserver/session.go`, `internal/httpserver/rate.go` | [Security](../SECURITY.md), [Architecture](architecture.md#security-and-concurrency-boundaries), and the Data or Operations section it affects |
| Deployment, backup, health, rollback, or release | `Dockerfile`, `docker-compose.yml`, `docker-entrypoint.sh`, `templates/unraid/*`, `internal/httpserver/health.go`, `.github/workflows/docker-image.yml`, `.github/workflows/discord-update.yml` | [Operations](operations.md), [Setup](../setup.md), [Unraid](../unraid.md) |

Indexes (the repository [README](../../README.md) and the
[development index](README.md)) change only when a page is added, removed, or
renamed.
