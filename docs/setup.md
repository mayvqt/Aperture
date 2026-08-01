# Setup

1. Start Aperture:

   ```bash
   docker compose up --build
   ```

2. Open `http://localhost:8099`.
3. Choose Jellyfin or Emby, then enter the Aperture URL, media-server URL, and API key.
4. Sign in with a media-server administrator account.
5. Import or create a non-administrator template, then create an invite.

Standalone installs can run `aperture serve` and use the same browser setup. Secrets are generated automatically and
stored under the config directory.

Complete the first-run setup from a trusted network before publishing Aperture through a reverse proxy. Until setup is
saved, `/setup` is intentionally unauthenticated because no administrator session can exist yet.
