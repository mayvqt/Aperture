# Security

## Reporting vulnerabilities

Use GitHub private vulnerability reporting. Do not open a public issue before a fix is available. Remove real secrets,
tokens, logs, and private URLs from reports. Security fixes target the current `main` branch.

API keys, media-server access tokens, session and CSRF secrets, and retained invite tokens are encrypted before database
storage. The web UI does not return stored API keys or upstream diagnostic text. Application logs redact known secret
forms and configured secrets.

Treat `/config`, backups, `.env`, cookies, logs, and invite links as sensitive. Disable proxy request-body logging.
Complete the unauthenticated first-run `/setup` flow on a trusted network before public exposure.
