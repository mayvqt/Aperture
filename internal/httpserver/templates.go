package httpserver

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

var pages = template.Must(template.New("pages").Funcs(templateFuncs).ParseFS(templateFiles, "templates/*.html"))

func render(w http.ResponseWriter, name string, data any) {
	renderStatus(w, name, data, http.StatusOK)
}

func renderStatus(w http.ResponseWriter, name string, data any, status int) {
	var body bytes.Buffer
	if err := pages.ExecuteTemplate(&body, name, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = body.WriteTo(w)
}

//go:embed templates/*.html
var templateFiles embed.FS
