# Contributing

Aperture is a focused Go service for Jellyfin and Emby invitations. Keep changes small, secure, and within that product
boundary.

## Local development

```bash
cp .env.example .env
go run ./cmd/aperture serve
```

See [Development](development.md) for the complete workflow.

## Pull requests

- Explain the user or operator problem being solved.
- Add tests for behavior, persistence, security, and failure-path changes.
- Keep schema changes explicit, versioned, and covered by fresh-initialization tests.
- Update deployment and configuration documentation when behavior changes.
- Run `gofmt`, `go test ./...`, `go vet ./...`, and `go test -race ./...`.
- Never commit `.env`, databases, credentials, tokens, logs, or generated artifacts.

Repository-specific design and security rules are in [AGENTS.md](../AGENTS.md).
