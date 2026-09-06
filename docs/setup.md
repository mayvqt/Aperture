# Setup

```bash
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8099` from a trusted network. Choose Jellyfin or Emby, enter the public URL, media-server URL,
and API key, then sign in as a media-server administrator. Create or import a non-admin template before creating an
invite.

Standalone installs use `aperture serve`. Persist and back up the config directory. Do not expose Aperture publicly
until the unauthenticated first-run setup is complete.

Fresh installs using the example environment store state in `./config`. Existing
installs must retain their current host data directory when adopting `.env`;
set `APERTURE_DATA_DIR` to that directory. Without this override Compose preserves
its original `/mnt/cache/appdata/aperture` mapping.

For backup, stop Aperture and copy the entire host state directory, including
`encryption.key` and any SQLite WAL/SHM files, then restart it. Restore into a
separate directory with the matching application version and verify setup/login,
templates, invites, and registration history. Never combine a restored database
with stale WAL/SHM files. Keep a pre-upgrade backup: schema upgrades are forward-only.
