package httpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/mayvqt/aperture/internal/config"
)

var csrfInputPattern = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

func hiddenCSRF(body string) string {
	match := csrfInputPattern.FindStringSubmatch(body)
	if len(match) != 2 {
		return ""
	}
	return match[1]
}

func adminRequest(t *testing.T, method, target string, body io.Reader) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session-id"})
	req.RemoteAddr = "192.0.2.1:1234"
	return req
}

func testConfig() config.Config {
	return config.Config{
		PublicURL:        "https://aperture.example",
		CookieSecure:     false,
		SessionSecret:    "session-secret-with-at-least-32-characters",
		InviteSecret:     "invite-secret-with-at-least-32-characters",
		MediaProvider:    "jellyfin",
		ServerURL:        "http://media:8096",
		APIKey:           "api-key",
		PublicURLManaged: true,
		ProviderManaged:  true,
		ServerURLManaged: true,
		APIKeyManaged:    true,
	}
}
