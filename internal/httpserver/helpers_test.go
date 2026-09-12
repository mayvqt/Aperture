package httpserver

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

func TestClientIPHonorsProxyOnlyWhenEnabled(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 198.51.100.10")
	if got := clientIP(req, nil); got != "198.51.100.10" {
		t.Fatalf("clientIP without trust proxy = %q", got)
	}
	if got := clientIP(req, parseTrustedProxies([]string{"198.51.100.0/24"})); got != "203.0.113.9" {
		t.Fatalf("clientIP with trust proxy = %q", got)
	}
}

func TestClientIPIgnoresInvalidProxyHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Forwarded-For", "not-an-ip")
	req.Header.Set("X-Real-IP", "also-not-an-ip")
	if got := clientIP(req, parseTrustedProxies([]string{"198.51.100.0/24"})); got != "198.51.100.10" {
		t.Fatalf("clientIP with invalid proxy headers = %q", got)
	}
}

func TestClientIPRejectsSpoofedForwardedPrefix(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "198.51.100.10:1234"
	req.Header.Set("X-Forwarded-For", "192.0.2.99, 203.0.113.9, 198.51.100.20")
	trusted := parseTrustedProxies([]string{"198.51.100.0/24"})

	if got := clientIP(req, trusted); got != "203.0.113.9" {
		t.Fatalf("clientIP with spoofed prefix = %q, want nearest untrusted client", got)
	}
}

func TestValidateBaseURL(t *testing.T) {
	cfg := testConfig()
	s := &Server{cfg: cfg}
	for _, value := range []string{"http://jellyfin:8096", "https://jellyfin.example"} {
		if _, err := s.validateServerURL(value); err != nil {
			t.Fatalf("validateBaseURL(%q) unexpected error: %v", value, err)
		}
	}
	for _, value := range []string{"ftp://jellyfin", "not a url", "", "https://user:pass@jellyfin.example", "https://jellyfin.example/?api_key=secret", "https://jellyfin.example/#fragment"} {
		if _, err := s.validateServerURL(value); err == nil {
			t.Fatalf("validateBaseURL(%q) expected error", value)
		}
	}
}

func TestMessageResponseUsesHTMLContentType(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()

	s.message(rr, "Invalid request", "Try again.", http.StatusBadRequest)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(rr.Body.String(), "Invalid request") {
		t.Fatalf("body = %q", rr.Body.String())
	}
}

func TestRequestUserAgentIsBounded(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", strings.Repeat("a", maxUserAgentLength+20))
	if got := requestUserAgent(req); len([]rune(got)) != maxUserAgentLength {
		t.Fatalf("bounded User-Agent length = %d", len([]rune(got)))
	}
}

func TestSafeErrorRedactsSecretLikeValues(t *testing.T) {
	got := safeError(assertErr("jellyfin failed: api_key=abc123 access_token=def456 password=hunter2"))
	for _, leaked := range []string{"abc123", "def456", "hunter2"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("safeError leaked %q in %q", leaked, got)
		}
	}
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

func TestDashboardStatsFrom(t *testing.T) {
	now := time.Now()
	stats := dashboardStatsFrom([]db.Invite{
		{Enabled: true, Uses: 0, MaxUses: 1},
		{Enabled: false, Uses: 0, MaxUses: 1},
		{Enabled: true, Uses: 1, MaxUses: 1},
		{Enabled: true, Uses: 0, MaxUses: 1, ExpiresAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true}},
	}, []db.Registration{
		{Status: "complete", UserDisableAt: sql.NullTime{Time: now.Add(time.Hour), Valid: true}},
		{Status: db.RegistrationNeedsAttention},
		{Status: db.RegistrationDisableFailed},
	})
	if stats.ActiveInvites != 1 || stats.Registrations != 3 || stats.NeedsAttention != 2 || stats.ScheduledUserDisables != 1 {
		t.Fatalf("stats = %#v", stats)
	}
}
