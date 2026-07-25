# Security

Aperture handles media-server credentials, API keys, sessions, invitation tokens, and encrypted local state.

## Reporting vulnerabilities

Use GitHub's private vulnerability reporting for this repository. Do not open a public issue until a fix is available. Include affected versions, impact, reproduction steps, and mitigations, while removing real credentials, tokens, logs, and private service URLs.

## Supported versions

Until stable release branches exist, security fixes target the current `main` branch.

## Sensitive data

Treat `/config`, SQLite backups, `.env`, session cookies, invite links, media-server tokens, API keys, and diagnostic logs as sensitive. Use placeholders in examples and reports.
