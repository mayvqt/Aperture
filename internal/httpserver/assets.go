package httpserver

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

//go:embed assets/*
var embeddedAssets embed.FS

var (
	stylesheet     = bundleStylesheets()
	stylesheetHash = fmt.Sprintf("%x", sha256.Sum256(stylesheet))[:12]
)

var stylesheetFiles = []string{
	"assets/base.css",
	"assets/layout.css",
	"assets/forms.css",
	"assets/components.css",
	"assets/pages.css",
	"assets/responsive.css",
}

func bundleStylesheets() []byte {
	var bundled bytes.Buffer
	for _, name := range stylesheetFiles {
		contents, err := embeddedAssets.ReadFile(name)
		if err != nil {
			panic(err)
		}
		bundled.Write(contents)
		bundled.WriteByte('\n')
	}
	return bundled.Bytes()
}

func (s *Server) assets() http.Handler {
	assets, _ := fs.Sub(embeddedAssets, "assets")
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/assets/app.css" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			http.ServeContent(w, r, "app.css", time.Time{}, bytes.NewReader(stylesheet))
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.StripPrefix("/assets/", fileServer).ServeHTTP(w, r)
	})
}
