# Configuration

Aperture can be configured in `/setup`. Environment values from [`.env.example`](../.env.example) override browser
settings and make the corresponding fields read-only.

The media provider must be `jellyfin` or `emby`. Registrations and expiry processing require an API key. Changing the
provider or server signs out the current administrator; existing registration history remains unchanged.

Cookie security follows the public HTTPS URL unless explicitly overridden. Trust forwarded client-IP headers only from
the directly connected proxy CIDRs.

## Storage

Docker state lives under `/config`; standalone state defaults to `./data`. Aperture generates the encryption, session,
and invite secrets when omitted. Back up the entire state directory. If a schema is unsupported, preserve the directory
and use a compatible Aperture version; never delete the database as a normal upgrade step.
