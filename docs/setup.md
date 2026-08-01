# Setup

```bash
docker compose up --build
```

Open `http://localhost:8099` from a trusted network. Choose Jellyfin or Emby, enter the public URL, media-server URL,
and API key, then sign in as a media-server administrator. Create or import a non-admin template before creating an
invite.

Standalone installs use `aperture serve`. Persist and back up the config directory. Do not expose Aperture publicly
until the unauthenticated first-run setup is complete.
