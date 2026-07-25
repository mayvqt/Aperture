# Configuration

Aperture can be configured entirely through the first-run browser page.
Environment variables are optional deployment-managed overrides.

## Automatic settings

The encryption key is generated at `/config/encryption.key` in Docker or
`./data/encryption.key` for a standalone install. Session and invite secrets are
generated and encrypted in SQLite. Back up the entire config directory.

## Media Server

```txt
APERTURE_MEDIA_PROVIDER=jellyfin
APERTURE_SERVER_URL=http://jellyfin:8096
APERTURE_API_KEY=
```

These values can instead be entered in the browser. The provider must be
`jellyfin` or `emby`. For Emby, a server-root URL is
automatically normalized to its `/emby` API path. The API key may be saved later
in Settings, but registrations and expiry processing require it. Browser-managed
provider, public URL, server URL, and API key values can be changed in Settings.
Changing the provider or server signs out the current administrator because the
saved session belongs to the previous server. Existing registration history is
not transferred to a new server.

## HTTP and Proxy

```txt
APERTURE_HTTP_ADDR=:8099
APERTURE_COOKIE_SECURE=true
APERTURE_TRUSTED_PROXY_CIDRS=127.0.0.1/32
```

Cookie security is inferred from the configured public URL. Environment values
override browser-managed settings. Trust only the CIDRs of proxies that directly
connect to Aperture.

## Storage

SQLite state lives in the config directory: `/config/aperture.db` in Docker or
`./data/aperture.db` standalone. If startup reports an unsupported database
schema, remove the database and restart.
