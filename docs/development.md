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
staticcheck ./...
govulncheck ./...
```

Keep SQL in `internal/db`, HTTP workflows in `internal/httpserver`, media protocol code in `internal/mediaserver`, and
secret primitives in `internal/security`. Schema changes require a revision update and upgrade/fresh-install tests.

Tests use temporary SQLite databases and fake media-server clients. No live
Jellyfin/Emby accounts or external database containers are needed. Use existing
installed tools; ask before installing missing tools or downloading dependencies.
CI pins tool versions and action commits, and image publishing reuses its full
checks. Run `sh scripts/test-entrypoint.sh` for entrypoint changes and
`CGO_ENABLED=0 go build -trimpath -o ./build/aperture ./cmd/aperture` for release
build validation. Published images report their tag or commit through
`aperture version`; local builds report `dev`.

No browser wrapper is checked in. Use available shared browser tooling with a
disposable config directory and synthetic users/invites; check desktop/mobile,
validation errors, permission denial, and the setup/login/invite flows. Templates,
CSS and JavaScript are embedded, so rebuild before checking rendered changes.
