# Unraid

Map `/mnt/user/appdata/aperture` to `/config`, expose container port `8099`, then open
`http://<unraid-address>:8099/setup`. Browser setup works on Unraid; no Aperture environment variables are required.

Use the media-server container name when both containers share a Docker network. Behind a reverse proxy, set
`APERTURE_TRUSTED_PROXY_CIDRS` only to the network that directly connects the proxy to Aperture.

Use `PUID` and `PGID` only when the defaults do not match the appdata owner. See [`.env.example`](../.env.example) for
optional Aperture settings. Back up `/mnt/user/appdata/aperture` and treat it as sensitive.
