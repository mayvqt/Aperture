# Aperture

[![CI](https://github.com/mayvqt/Aperture/actions/workflows/ci.yml/badge.svg)](https://github.com/mayvqt/Aperture/actions/workflows/ci.yml)
[![Docker Image](https://github.com/mayvqt/Aperture/actions/workflows/docker-image.yml/badge.svg)](https://github.com/mayvqt/Aperture/actions/workflows/docker-image.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26.5-00ADD8)](go.mod)
[![Container](https://img.shields.io/badge/ghcr.io-mayvqt%2Faperture-0f766e)](https://github.com/mayvqt/Aperture/pkgs/container/aperture)

Aperture is a small Jellyfin and Emby invite-registration app.

Admins sign in with their media-server credentials, create invite links, and choose a user-policy template. Invite
recipients open a link, create an account on the configured server, and Aperture applies the selected template.

## Docs

- [Setup](docs/setup.md): first run, media-server connection, Docker Compose, and standalone binaries.
- [Configuration](docs/configuration.md): environment variables, secrets, reverse proxies, and persistence.
- [Unraid](docs/unraid.md): container setup notes for Unraid.
- [Development](docs/development.md): local dev commands, tests, schema policy, and project style.
- [Contributing](docs/CONTRIBUTING.md): contribution requirements and checks.
- [Security](docs/SECURITY.md): vulnerability reporting and sensitive-data guidance.

## Quick Start

```bash
docker compose up --build
```

Open `http://localhost:8099`, complete browser setup, then sign in with a media-server administrator account.

The codebase intentionally uses server-rendered HTML, SQLite, and a small provider-aware media-server client.
