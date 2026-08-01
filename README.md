# Aperture

Aperture creates Jellyfin or Emby accounts from controlled invite links and applies non-admin policy templates.

## Quick Start

```bash
docker compose up --build
```

Open `http://localhost:8099`, complete browser setup, then sign in with a media-server administrator account.

Complete setup on a trusted network before exposing Aperture publicly. Persist and back up `/config`.

See [setup](docs/setup.md), [configuration](docs/configuration.md), [security](docs/SECURITY.md), or
[development](docs/development.md). See [contributing](docs/CONTRIBUTING.md) before opening a change. Unraid users can
use the [Unraid notes](docs/unraid.md).
