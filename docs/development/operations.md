# Operations

Run one Aperture process per state directory. The container runs without added
privileges and should receive only a writable `/config`, its listening port, and
network access to the configured media server and webhook destinations. Restrict
proxy trust to the directly connected proxy. Keep `.env`, `/config`, cookies,
invite URLs, logs, and backups private; diagnostics must be redacted before
sharing.

## Backup and restore

Before upgrades, stop Aperture and copy the entire host state directory,
including `encryption.key`, the database, and any matching WAL/SHM files. Store
the backup off-host with restricted access. Restore into a separate directory
using the matching application version; never combine a database with stale
WAL/SHM files. Verify `/healthz`, setup/login, templates, invites, registration
history, and a synthetic media-server connection before treating the backup as
usable. Record the last successful restore test.

## Release and rollback

Builds are published by `.github/workflows/docker-image.yml` after the reusable
CI workflow succeeds. Deploy an immutable version tag or digest and record it
with the backup used for the change; `latest` is not a rollback reference.
After deployment, verify process health, `/healthz`, authentication, an
administrator read, and configured outbound connectivity without exposing
secrets.

Schema upgrades are forward-only. Application rollback therefore means restoring
the pre-upgrade state with the previous immutable image. Stop on failed health or
data verification; preserve failed state for diagnosis. Maintenance or repair
actions that mutate users, registrations, or SQLite state are explicit,
operator-approved procedures, never automatic troubleshooting steps.

Revision 5 resumes incomplete-account disables from durable state. Review older
password-setup failures before upgrading as described in [Security](../SECURITY.md).
Allow four minutes for accepted account operations to finish before SQLite
closes. The supplied Compose and Unraid configurations allow 270 seconds,
including notification delivery, before forcing the process to stop.
