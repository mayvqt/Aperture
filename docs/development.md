# Development

Run locally:

```bash
go run ./cmd/aperture serve
```

Open `http://localhost:8099/setup`. Go does not load `.env` files itself; the example file is for Docker Compose or for
values explicitly exported by your shell.

Required checks:

```bash
gofmt -w <touched-go-files>
go test ./...
go vet ./...
go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Package responsibilities:

- `cmd/aperture`: process lifecycle.
- `internal/config`: strict configuration parsing.
- `internal/db`: SQLite schema and persistence.
- `internal/httpserver`: HTTP workflows, sessions, views, and workers.
- `internal/mediaserver`: provider-neutral contract and URL validation.
- `internal/mediaserver/protocol`: shared Jellyfin/Emby protocol mechanics.
- `internal/mediaserver/jellyfin` and `emby`: isolated provider adapters and tests.
- `internal/security`: tokens, hashing, encryption, and redaction.

Keep provider-specific protocol behavior out of handlers and persistence code. Schema changes update the canonical
schema revision and require fresh-init tests.
