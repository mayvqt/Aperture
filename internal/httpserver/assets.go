package httpserver

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"time"
)

//go:embed assets/*
var embeddedAssets embed.FS

var (
	stylesheet     = bundleStylesheets()
	stylesheetHash = assetVersion(stylesheet)
	scriptHash     = assetVersion(mustReadAsset("assets/app.js"))
)

var stylesheetFiles = []string{
	"assets/base.css",
	"assets/layout.css",
	"assets/forms.css",
	"assets/components.css",
	"assets/pages.css",
	"assets/responsive.css",
	"assets/aperture.css",
}

func bundleStylesheets() []byte {
	var bundled bytes.Buffer
	for _, name := range stylesheetFiles {
		bundled.Write(mustReadAsset(name))
		bundled.WriteByte('\n')
	}
	return bundled.Bytes()
}

func mustReadAsset(name string) []byte {
	contents, err := embeddedAssets.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return contents
}

func assetVersion(contents []byte) string {
	digest := sha256.Sum256(contents)
	return hex.EncodeToString(digest[:6])
}

func (s *Server) assets() http.Handler {
	assets, _ := fs.Sub(embeddedAssets, "assets")
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets/app.css" || r.URL.Path == "/assets/app.js" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if r.URL.Path == "/assets/app.css" {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeContent(w, r, "app.css", time.Time{}, bytes.NewReader(stylesheet))
			return
		}
		if w.Header().Get("Cache-Control") == "" {
			w.Header().Set("Cache-Control", "no-cache")
		}
		http.StripPrefix("/assets/", fileServer).ServeHTTP(w, r)
	})
}
