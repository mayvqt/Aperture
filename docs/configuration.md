# Configuration

Aperture can be configured in `/setup`. Environment values from [`.env.example`](../.env.example) override browser
settings and make the corresponding fields read-only.

The media provider must be `jellyfin` or `emby`. Registrations and expiry processing require an API key. Changing the
provider or server signs out the current administrator; existing registration history remains unchanged.

Cookie security follows the public HTTPS URL unless explicitly overridden. Trust forwarded client-IP headers only from
the directly connected proxy CIDRs.

## Storage

`APERTURE_DATA_DIR` selects the Compose host directory mounted at `/config`. Keep
it unchanged for existing installs; see [Setup](setup.md). Docker state lives under `/config`; standalone state defaults to `./data`. Aperture generates the encryption, session,
and invite secrets when omitted. Back up the entire state directory. If a schema is unsupported, preserve the directory
and use a compatible Aperture version; never delete the database as a normal upgrade step.

## Webhooks

Admins can configure Discord or generic JSON webhooks, events, and optional Discord role IDs in the web UI. URLs are
encrypted. Failed template application is retried automatically up to six times.

## Account recovery

Registrations show access retries and pending disables separately. After six
automatic access retries, review the account and use **Retry access** when ready.
Failed disables keep retrying with backoff until the account is disabled or no
longer exists. Password-incomplete accounts require administrator review and
cannot be enabled by retrying a template.

An account's expiry is fixed when its invite use is reserved. Recovery never
extends it, and an expired account can only be disabled. Accounts undergoing
creation or recovery cannot be removed until that operation finishes.

## Changing servers and reviewing history

Ownership includes the provider, normalized server URL and authenticated server ID.
Changing any of these signs out all administrators. Existing invites and accounts
remain associated with their original server; automatic work pauses for records
that belong elsewhere. A cloned server ID at a different URL is a separate server.

After an upgrade, older records show **Unverified server**. Use **Review server** on
an invite or registration to inspect the destination and confirm its assignment.
Review tracked-only users under **Users → Tracked user history**. Assignment keeps
invite links, usage counts and account deadlines. It never assigns all accounts
from an invite together. The account ID must exist on the destination server with
a verified non-administrator policy; Aperture does not guess ownership from names.

Use **Needs review** to find older records requiring attention. Invite and
registration history pages show 50 records at a time, with links to older pages.
