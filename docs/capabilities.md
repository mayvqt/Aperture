# Capabilities

Aperture provides:

- browser-based first-run configuration for Jellyfin or Emby;
- administrator authentication through the configured media server;
- reusable non-administrator policy templates that preserve target authentication defaults, including import from an existing user;
- bounded, expiring invite links with usage limits and optional account expiry;
- account creation, policy application, retry, disable, and recovery workflows;
- managed-user and registration history views;
- Discord and generic JSON webhooks with selected events; and
- administrative audit history with bounded retention.

The media server remains the source of truth for users, passwords, and effective
permissions. Aperture stores the state needed to coordinate invitations and
recovery. Configuration details are in [Configuration](configuration.md).
