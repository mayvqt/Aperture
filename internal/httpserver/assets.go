package httpserver

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embeddedAssets embed.FS

func (s *Server) assets() http.Handler {
	assets, _ := fs.Sub(embeddedAssets, "assets")
	fileServer := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		http.StripPrefix("/assets/", fileServer).ServeHTTP(w, r)
	})
}
