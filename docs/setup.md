# Setup

```bash
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8099` from a trusted network. Choose Jellyfin or Emby, enter the public URL, media-server URL,
and API key, then sign in as a media-server administrator. Create or import a non-admin template before creating an
invite.

The default template uses the media server's account defaults with administrator
access disabled. Imported policies keep the new account's authentication and
password-reset providers. JSON property names must be unique regardless of casing.

Standalone installs use `aperture serve`. Persist and back up the config directory. Do not expose Aperture publicly
until the unauthenticated first-run setup is complete.

Fresh installs using the example environment store state in `./config`. Existing
installs must retain their current host data directory when adopting `.env`;
set `APERTURE_DATA_DIR` to that directory. Without this override Compose preserves
its original `/mnt/cache/appdata/aperture` mapping.

Before upgrades, follow the [backup, restore, and rollback
procedure](development/operations.md). Schema upgrades are forward-only.
