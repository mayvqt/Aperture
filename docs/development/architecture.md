# Architecture

`cmd/aperture` loads configuration, opens the store, initializes schema and
runtime secrets, selects a media-server adapter, and starts one HTTP server that
owns maintenance and notification delivery. Handlers depend on narrow store and media-server
interfaces; persistence stays in `internal/db`, while outbound Jellyfin/Emby
protocol details stay in `internal/mediaserver`.

## Sources of truth and contracts

- Jellyfin or Emby owns user identity, passwords, administrator status, and
  effective user policy.
- SQLite owns Aperture settings, templates, invite state, registration recovery,
  managed-user tracking, sessions, webhooks, and audit records.
- Environment variables and flags override browser-managed settings. The database
  owns settings that are not deployment-managed.
- Public HTTP routes are registered in `internal/httpserver/routes.go`.
  `/i/{token}` and registration are unauthenticated; `/setup` is available only
  before setup completes; `/admin/*` requires a verified media-server admin.
- Jellyfin and Emby request shapes and authorization are provider contracts.
  Verify changes against the supported upstream documentation and cover them
  with deterministic HTTP fixtures before any opt-in live smoke test.
- Policy application reads the target user's complete policy before merging
  template overrides. Shared normalization rejects ambiguous case aliases,
  preserves target authentication providers, and forces non-administrator access.

## Security and concurrency boundaries

Invite tokens, session material, API keys, access tokens, webhook URLs, cookies,
and the entire state directory are sensitive. Preserve CSRF checks, bounded
request bodies, rate limits, trusted-proxy parsing, security headers, redaction,
and no-redirect outbound clients.

SQLite is deliberately limited to one connection and uses WAL, foreign keys, and
a busy timeout. Setup is serialized in-process. Registration reserves invite
capacity and an immutable expiry before provisioning; ambiguous external failures
retain evidence rather than silently releasing capacity. Shared account recovery
coordinates manual and automatic retries with per-registration operation guards,
transactional claims, and durable cleanup. The maintenance worker reconciles
stale work, disables incomplete and expired accounts before retrying templates,
and prunes audit events. Graceful shutdown drains accepted HTTP and maintenance
work before notifications and before closing SQLite. Changes to
these flows must preserve idempotency, bounded work, and cancellation behavior.

`internal/connection` resolves deployment overrides and publishes immutable
connection snapshots through a media adapter factory. Each accepted operation
retains its adapter, URL, credentials and verified ownership. Administrative
sessions must match both binding and generation before their tokens are sent;
authenticated system information detects a replacement at the same URL.
Background account queries filter ownership before limits and grouping. History
remains readable across origins; explicit assignment uses account operation guards
and verifies an existing non-administrator account at the destination.
