# Unraid

Map `/mnt/user/appdata/aperture` to `/config`, expose container port `8099`, then open
`http://<unraid-address>:8099/setup`. Browser setup works on Unraid; no Aperture environment variables are required.

## Optional variables

```txt
PUID=99
PGID=100
UMASK=022
APERTURE_HTTP_ADDR=:8099
APERTURE_CONFIG_DIR=/config
APERTURE_DB_PATH=/config/aperture.db
APERTURE_LOG_LEVEL=info
APERTURE_TRUSTED_PROXY_CIDRS=127.0.0.1/32
APERTURE_PUBLIC_URL=https://invites.example.com
APERTURE_COOKIE_SECURE=true
APERTURE_MEDIA_PROVIDER=jellyfin
APERTURE_SERVER_URL=http://jellyfin:8096
APERTURE_API_KEY=
APERTURE_ENCRYPTION_KEY=
APERTURE_SESSION_SECRET=
APERTURE_INVITE_SECRET=
```

`PUID`, `PGID`, and `UMASK` default to the values shown. Leave the application settings blank to manage them in the
browser. Setting one by environment makes that field deployment-managed. Use `emby` instead of `jellyfin` when needed.

Use the media-server container name when both containers share a Docker network. Behind a reverse proxy, set
`APERTURE_TRUSTED_PROXY_CIDRS` only to the network that directly connects the proxy to Aperture.

Back up `/mnt/user/appdata/aperture` and treat it as sensitive.
