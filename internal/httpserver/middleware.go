package httpserver

import (
	"net/http"
	"net/url"
	"strings"
)

func securePublicHost(rawURL string) string {
	publicURL, err := url.Parse(rawURL)
	if err != nil || publicURL.Scheme != "https" {
		return ""
	}
	return publicURL.Host
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		_, publicURL, _ := s.runtimeSettings()
		hstsHost := securePublicHost(publicURL)
		if hstsHost != "" && strings.EqualFold(hstsHost, r.Host) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if !strings.HasPrefix(r.URL.Path, "/assets/") && r.URL.Path != "/healthz" {
			w.Header().Set("Cache-Control", "no-store")
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		next.ServeHTTP(w, r)
	})
}
