# AGENTS.md

Engineering guidance for contributors and coding agents working on Aperture.

## Product Boundary

Aperture is a focused Jellyfin and Emby invite-registration service. It connects to one configured media server,
authenticates administrators, creates and manages invitations, registers users, applies non-admin policy templates,
records outcomes, and disables users when configured expiry dates arrive.

Do not turn it into an identity provider, notification platform, plugin system, media-request service, or frontend
framework. New behavior must directly support secure invitation, registration, policy application, expiry enforcement,
or operational follow-up.

## Priorities

Security and durable state come first, followed by correctness, maintainability, and operator clarity. Prefer boring Go,
explicit SQL, server-rendered HTML, and small cohesive packages. A compact codebase is valuable only while
responsibilities remain obvious.

## Before Changing Code

1. Read the complete workflow, its persistence methods, tests, and documentation.
2. Check `git status` and preserve unrelated work.
3. Consider upgrade behavior for existing databases and containers. Assume users will pull a newer image while keeping
   the same `/config/aperture.db`.
4. Identify effects on invite secrecy, admin authorization, CSRF, rate limits, proxy trust, and external media-server
   state.
5. Make the smallest cohesive change and test the failure paths.

## Architecture

- `cmd/aperture`: CLI dispatch, logging, process signals, and server lifecycle.
- `internal/config`: strict environment/flag parsing and deployment validation.
- `internal/db`: SQLite connection policy, schema initialization, encrypted persistence, and atomic transitions.
- `internal/httpserver`: routes, middleware, handlers, sessions, CSRF, rate limiting, background expiry orchestration,
  views, templates, and embedded assets.
- `internal/mediaserver`: provider-neutral contract and URL validation.
- `internal/mediaserver/protocol`: shared Jellyfin/Emby protocol mechanics.
- `internal/mediaserver/jellyfin` and `internal/mediaserver/emby`: provider-specific adapters.
- `internal/security`: token, hashing, comparison, redaction, and encryption primitives.

Do not put SQL or media-server request details in handlers. Do not put business decisions in templates, JavaScript,
startup code, or generic helpers.

## Code Organization

- One file should have one discoverable purpose. Split mixed workflows before they become catch-alls; 300–400 lines is a
  signal to review cohesion.
- Group handlers by user workflow, database code by aggregate/schema concern, and media-server calls by API area.
- Prefer clear typed data, early returns, small functions, and explicit transaction boundaries.
- Define narrow interfaces at the consuming package only when they provide a real test seam.
- Avoid global mutable state, framework layers, generic utility packages, speculative abstractions, and clever
  reflection.
- Preserve error causes with wrapping, but show public users only generic non-enumerating messages.
- Use `context.Context` throughout request, database, worker, and media-server paths.

## Security Invariants

- Never store or log passwords, raw session tokens, raw CSRF tokens, API keys, access tokens, or unencrypted retained
  invite tokens.
- Hash lookup tokens, encrypt recoverable secrets, use constant-time comparisons, and use cryptographically secure
  randomness.
- Invited users must never receive administrator policy fields.
- Admin access requires current administrator status on the configured media server.
- All state-changing forms require CSRF validation. Cookies remain `HttpOnly`, `SameSite`, and `Secure` by default.
- Rate-limit login and public registration using a client IP derived only from a directly connected trusted proxy.
- Configure trusted proxy CIDRs explicitly. Never use a global “trust all forwarded headers” switch.
- Validate URLs and user input, bound request/response sizes, and redact secret-like error content before persistence or
  logging.
- Add focused tests whenever a security invariant changes.

## Persistence And Upgrades

- SQLite is the local source of truth. Enable foreign keys, WAL mode, and a busy timeout on every connection.
- Maintain one clean canonical schema and increment its `PRAGMA user_version` when persisted expectations change.
- Unsupported schema revisions must be rejected clearly before serving traffic.
- Startup must initialize or validate the database before accepting traffic and must fail clearly on unsupported schema
  revisions.
- Test fresh initialization, repeated startup, schema revision handling, constraints, indexes, seed data, and durable
  invariants.
- Use constraints for durable invariants and transactions or atomic statements for invite reservations and multi-step
  changes.
- Preserve audit and registration history. Soft-deleted invites must not be resurrected.

## Media Server And Expiry Work

- The configured media server owns users, credentials, policies, libraries, and administrator identity.
- All clients require context cancellation, explicit timeouts, bounded responses, checked statuses, and redacted errors.
- Partial user-creation or policy failures must be recorded for admin follow-up without leaking secrets.
- User expiry is operationally time-sensitive. A lifecycle-bound background worker must process due disables without
  requiring an admin request or restart, retry safely, and stop on context cancellation.
- Aperture disables expired media-server users; it does not delete them.

## HTTP And UI

- Use bounded request bodies and server read-header, read, write, and idle timeouts.
- HTML forms must work without JavaScript. JavaScript may progressively enhance an existing workflow.
- Keep public errors calm and generic. Admin diagnostics may be precise only after redaction.
- Prefer direct forms and tables over wizards, dashboards full of chrome, theme systems, or frontend frameworks.

## Configuration And Containers

- Parse configuration strictly and reject invalid booleans, URLs, CIDRs, log levels, and undersized secrets.
- Keep `.env.example`, Dockerfile, Compose, README, and configuration docs synchronized.
- The container entrypoint may prepare `/config` as root, then must run Aperture unprivileged. Compose settings must not
  bypass that setup accidentally.
- Persist only application state under `/config`; never bake runtime secrets into an image.
- Treat image updates as the normal upgrade path. Do not require users to delete databases, clear `/config`, or manually
  run SQL to recover from an ordinary release upgrade.

## Tests And Tooling

Tests should emphasize token handling, encryption, sessions, CSRF, proxy trust, rate limits, invite state transitions,
schema initialization, registration compensation, policy sanitization, provider contracts, expiry retries, and public
error behavior. Use fake transports and stores rather than live services.

Before finishing, run:

```bash
gofmt -w <touched-go-files>
go test ./...
go vet ./...
go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Staticcheck findings are required fixes, including style checks such as lowercase, punctuation-free internal error
strings (`ST1005`). Keep user-facing validation copy separate from internal errors when it needs sentence-style
capitalization or punctuation. Vulnerability findings must be resolved by updating the affected toolchain or dependency;
do not suppress reachable findings. Do not weaken tests or checks to make a change pass.

## Documentation And Repository Hygiene

- Document configuration, schema lifecycle behavior, security assumptions, and operator recovery steps.
- Keep licensing, contribution, and vulnerability-reporting information current for a public repository.
- Do not commit secrets, local databases, generated config, build output, or tool caches.
- Keep dependency and automation updates reviewable and covered by CI.

## Definition Of Done

A change is complete only when code, schema policy, tests, documentation, and deployment files agree; required checks
pass; fresh installs and supported database revisions are safe; public behavior does not leak internals; and the diff
contains no unrelated churn.
