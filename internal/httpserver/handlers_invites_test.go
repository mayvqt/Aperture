package httpserver

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestInvitesListShowsSavedCopyButtons(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodGet, "/admin/invites", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`data-copy="invite-url-1"`, "https://aperture.example/i/saved-token", "account access", "alice", "Duplicate"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, ">Enable</button>") {
		t.Fatalf("active invite row should not show Enable action:\n%s", body)
	}
}

func TestInvitesNewCanReuseExistingInviteAsPreset(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodGet, "/admin/invites/new?preset_id=1", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{`value="Family"`, `value="3"`, `selected>Default</option>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing preset value %q:\n%s", want, body)
		}
	}
}

func TestInvitesNewIgnoresMissingPreset(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := adminRequest(t, http.MethodGet, "/admin/invites/new?preset_id=404", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	if !htmlElementHasAttributes(rr.Body.String(), "input", map[string]string{
		"name":     "max_uses",
		"type":     "number",
		"min":      "1",
		"max":      "500",
		"value":    "1",
		"required": "",
	}) {
		t.Fatalf("body missing default max uses:\n%s", rr.Body.String())
	}
}

func TestInvitesCreateStoresUserExpiryDaysWithoutPuttingTokenInRedirect(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	form := url.Values{
		"csrf":             {store.session.CSRFSecret},
		"label":            {"Family"},
		"template_id":      {"1"},
		"max_uses":         {"2"},
		"expires_at":       {"2030-01-02"},
		"user_expiry_days": {"14"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/invites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if store.createdInvite.UserExpiryDays != 14 {
		t.Fatalf("UserExpiryDays = %d, want 14", store.createdInvite.UserExpiryDays)
	}
	if store.createdInvite.Token == "" {
		t.Fatal("expected raw invite token to be retained for encrypted copy links")
	}
	location := rr.Header().Get("Location")
	if location != "/admin/invites" {
		t.Fatalf("Location = %q, want token-free invite list URL", location)
	}
	if strings.Contains(location, store.createdInvite.Token) || strings.Contains(location, "%2Fi%2F") {
		t.Fatalf("redirect exposed invite token material: %q", location)
	}
}

func TestInvitesCreateRequiresAPIKey(t *testing.T) {
	store := newFakeStore()
	store.settings.APIKey = ""
	cfg := testConfig()
	cfg.APIKey = ""
	cfg.APIKeyManaged = false
	handler := New(cfg, store, &fakeMediaServer{})
	form := url.Values{
		"csrf":        {store.session.CSRFSecret},
		"label":       {"Family"},
		"template_id": {"1"},
		"max_uses":    {"1"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/invites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict || !strings.Contains(rr.Body.String(), "API key required") {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	if store.createdInvite.ID != 0 {
		t.Fatal("invite was created without an API key")
	}
}

func TestInvitesCreateSupportsQuickExpiryChoices(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	form := url.Values{
		"csrf":               {store.session.CSRFSecret},
		"label":              {"Family"},
		"template_id":        {"1"},
		"max_uses":           {"2"},
		"expires_after_days": {"7"},
		"user_expiry_days":   {"0"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/invites", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if !store.createdInvite.ExpiresAt.Valid {
		t.Fatal("expected quick expiry to set invite expiry")
	}
}
