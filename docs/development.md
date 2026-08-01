# Development

```bash
go run ./cmd/aperture serve
```

Open `http://localhost:8099/setup`. Go does not load `.env` automatically.

```bash
gofmt -w <touched-go-files>
go test ./...
go vet ./...
go test -race ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

Keep SQL in `internal/db`, HTTP workflows in `internal/httpserver`, media protocol code in `internal/mediaserver`, and
secret primitives in `internal/security`. Schema changes require a revision update and upgrade/fresh-install tests.
