# Security

## Reporting vulnerabilities

Use GitHub private vulnerability reporting. Do not open a public issue before a fix is available. Remove real secrets,
tokens, logs, and private URLs from reports. Security fixes target the current `main` branch.

API keys, media-server access tokens, session and CSRF secrets, and retained invite tokens are encrypted before database
storage. The web UI does not return stored API keys or upstream diagnostic text. Application logs redact known secret
forms and configured secrets.

Treat `/config`, backups, `.env`, cookies, logs, and invite links as sensitive. Disable proxy request-body logging.
Complete the unauthenticated first-run `/setup` flow on a trusted network before public exposure.

Administrative audit events are retained for 90 days. Maintenance removes expired events in bounded batches to keep
the application database from growing indefinitely.

## Provisioning failures and upgrades

Initial setup is serialized within the application process so a delayed request
cannot overwrite completed setup. Run one Aperture process per state directory.
After an invite use is reserved, account provisioning continues for a bounded
period even if the browser disconnects. Incomplete accounts are disabled when
the media server is reachable; failed cleanup requires administrator intervention.
Accounts with incomplete password setup retain their external ID for review and
cannot be enabled through automatic or manual template-only retries.

Template policies always disable administrator access, regardless of property
casing. Conflicting property names are rejected. Applying a template preserves
the target account's authentication and password-reset providers and merges its
remaining defaults, so imported authentication settings do not cross accounts.

Before upgrading an existing installation, review older `needs_attention`
registrations in the media server, especially password-setup failures. Earlier
records are not reclassified by this release. Keep any incomplete accounts
disabled and finish password setup before allowing a template retry. Do this
before restarting Aperture, since maintenance can retry eligible records.
