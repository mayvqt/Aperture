# Security

Aperture handles media-server credentials, API keys, sessions, invitation tokens, and encrypted local state.

## Reporting vulnerabilities

Use GitHub's private vulnerability reporting for this repository. Do not open a public issue until a fix is available.
Include affected versions, impact, reproduction steps, and mitigations, while removing real credentials, tokens, logs,
and private service URLs.

## Supported versions

Until stable release branches exist, security fixes target the current `main` branch.

## Sensitive data

Treat `/config`, SQLite backups, `.env`, session cookies, invite links, media-server tokens, API keys, and diagnostic
logs as sensitive. Use placeholders in examples and reports.

API keys, media-server access tokens, session and CSRF secrets, and retained invite tokens are encrypted before database
storage. Secret-bearing persistence models are not passed to HTML templates, and persisted upstream diagnostic text is
not rendered in the web UI. API-key fields are write-only: saving an empty field retains the current key, and no
endpoint returns its value.

Aperture redacts common credential forms and configured runtime secrets from application logs and stored operational
errors. It also keeps raw invite tokens out of post-create redirect URLs so reverse-proxy access logs do not capture
them. Operators should still disable request-body logging, restrict access to logs and `/config`, and treat copied invite
links as bearer credentials.

The first-run `/setup` route is intentionally available before an administrator session exists. Complete setup from a
trusted network before exposing Aperture through a public reverse proxy.
