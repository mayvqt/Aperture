# Aperture

## Overview

Aperture creates Jellyfin or Emby accounts from controlled invite links and applies non-admin policy templates.

## Quick start

```bash
cp .env.example .env
docker compose up -d --build
```

Open `http://localhost:8099`, complete browser setup, then sign in with a media-server administrator account.

Complete setup on a trusted network before exposing Aperture publicly. Fresh installs keep state in `./config`;
existing installs should retain their current data path. See [setup and backups](docs/setup.md).

## Documentation

- [Setup](docs/setup.md)
- [Configuration](docs/configuration.md)
- [Unraid](docs/unraid.md)
- [Security](docs/SECURITY.md)
- [Development](docs/development.md)
- [Contributing](docs/CONTRIBUTING.md)
- [License](LICENSE)
- [Issues](https://github.com/mayvqt/Aperture/issues)

## Related projects

These are separate deployments in the same media-server and Seerr ecosystem:

- [Veyra](https://github.com/mayvqt/Veyra) — a self-hosted Jellyfin or Emby portal with Seerr requests and Arr data.
- [Augur](https://github.com/mayvqt/Augur) — a Discord bot for requesting movies and TV shows through Seerr.
