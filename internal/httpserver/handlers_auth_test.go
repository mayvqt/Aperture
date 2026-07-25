package httpserver

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/security"
)

func TestAdminLoginBrowserFlow(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})

	getRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, getRequest)
	csrf := hiddenCSRF(getResponse.Body.String())
	if csrf == "" {
		t.Fatalf("login page missing CSRF token:\n%s", getResponse.Body.String())
	}

	form := url.Values{
		"csrf":     {csrf},
		"username": {"admin"},
		"password": {"password"},
	}
	postRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range getResponse.Result().Cookies() {
		postRequest.AddCookie(cookie)
	}
	postResponse := httptest.NewRecorder()
	handler.ServeHTTP(postResponse, postRequest)
	if postResponse.Code != http.StatusSeeOther || postResponse.Header().Get("Location") != "/admin" {
		t.Fatalf("login status = %d, location = %q; body %s", postResponse.Code, postResponse.Header().Get("Location"), postResponse.Body.String())
	}
	if store.createdDeviceID != "new-device-id" {
		t.Fatalf("stored session device ID = %q, want authenticated device ID", store.createdDeviceID)
	}

	adminRequest := httptest.NewRequest(http.MethodGet, "/admin", nil)
	for _, cookie := range postResponse.Result().Cookies() {
		adminRequest.AddCookie(cookie)
	}
	adminResponse := httptest.NewRecorder()
	handler.ServeHTTP(adminResponse, adminRequest)
	if adminResponse.Code != http.StatusOK {
		t.Fatalf("admin status = %d; body %s", adminResponse.Code, adminResponse.Body.String())
	}
	if adminResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("admin Cache-Control = %q, want no-store", adminResponse.Header().Get("Cache-Control"))
	}
}

func TestSuccessfulLoginsDoNotExhaustClientIPLimit(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})
	csrf := "csrf-value"

	for i := range 60 {
		form := url.Values{
			"csrf":     {csrf},
			"username": {fmt.Sprintf("admin-%d", i)},
			"password": {"password"},
		}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.0.2.10:1234"
		req.AddCookie(&http.Cookie{Name: anonymousCSRFCookie, Value: security.HashToken(store.settings.SessionSecret, csrf)})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther {
			t.Fatalf("successful attempt %d status = %d, want redirect", i+1, rr.Code)
		}
	}
}

func TestSetupPostRejectsInvalidMediaServerURL(t *testing.T) {
	store := newFakeStore()
	store.settings.Provider = ""
	store.settings.ServerURL = ""
	handler := New(config.Config{
		PublicURL:     "https://aperture.example",
		CookieSecure:  false,
		SessionSecret: store.settings.SessionSecret,
		InviteSecret:  store.settings.InviteSecret,
	}, store, &fakeMediaServer{})
	csrf := "csrf-value"
	form := url.Values{
		"csrf":       {csrf},
		"provider":   {"jellyfin"},
		"public_url": {"https://aperture.example"},
		"server_url": {"ftp://media"},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  anonymousCSRFCookie,
		Value: security.HashToken(store.settings.SessionSecret, csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want rendered setup form; body %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Enter a valid server URL without credentials, query strings, or fragments.") {
		t.Fatalf("body missing validation error:\n%s", rr.Body.String())
	}
}

func TestSetupRedirectsWhenMediaServerURLIsEnvironmentManaged(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/setup", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/login" {
			t.Fatalf("%s /setup = %d location %q, want redirect to login", method, rr.Code, rr.Header().Get("Location"))
		}
	}
	if len(store.settingWrites) != 0 {
		t.Fatalf("environment-managed setup writes = %#v, want none", store.settingWrites)
	}
}

func TestSetupUsesEnvironmentManagedAPIKeyWithoutStoringIt(t *testing.T) {
	store := newFakeStore()
	store.settings.Provider = ""
	store.settings.PublicURL = ""
	store.settings.ServerURL = ""
	cfg := testConfig()
	cfg.ServerURL = ""
	cfg.ServerURLManaged = false
	cfg.APIKeyManaged = true
	media := &fakeMediaServer{}
	handler := New(cfg, store, media)

	getReq := httptest.NewRequest(http.MethodGet, "/setup", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK || !strings.Contains(getRR.Body.String(), "Managed by APERTURE_API_KEY") || !strings.Contains(getRR.Body.String(), `name="api_key" autocomplete="off" disabled`) {
		t.Fatalf("managed API key setup form was editable:\n%s", getRR.Body.String())
	}

	csrf := "csrf-value"
	form := url.Values{
		"csrf":       {csrf},
		"provider":   {"jellyfin"},
		"public_url": {"https://aperture.example"},
		"server_url": {"http://new-media:8096"},
		"api_key":    {"forged-key"},
	}
	postReq := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: anonymousCSRFCookie, Value: security.HashToken(store.settings.SessionSecret, csrf)})
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("POST status = %d, want redirect; body %s", postRR.Code, postRR.Body.String())
	}
	if len(store.settingWrites) != 4 || store.settingWrites[0].key != "media_provider" || store.settingWrites[1].key != "public_url" {
		t.Fatalf("setup writes = %#v, want complete browser settings", store.settingWrites)
	}
	if got, _ := media.pingAPIKey.Load().(string); got != cfg.APIKey {
		t.Fatalf("setup ping API key = %q, want environment-managed key", got)
	}
}

func TestSetupConfiguresEmbyEntirelyFromBrowser(t *testing.T) {
	store := newFakeStore()
	store.settings.Provider = ""
	store.settings.PublicURL = ""
	store.settings.ServerURL = ""
	store.settings.APIKey = ""
	cfg := config.Config{
		CookieSecure: false,
	}
	media := &fakeMediaServer{}
	handler := New(cfg, store, media)

	getReq := httptest.NewRequest(http.MethodGet, "/setup", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	csrf := hiddenCSRF(getRR.Body.String())
	if csrf == "" || !strings.Contains(getRR.Body.String(), `<select name="provider"`) ||
		!strings.Contains(getRR.Body.String(), `<option value="jellyfin"`) ||
		!strings.Contains(getRR.Body.String(), `<option value="emby"`) ||
		strings.Contains(getRR.Body.String(), `<input name="provider"`) ||
		!strings.Contains(getRR.Body.String(), `name="public_url" value="http://example.com"`) {
		t.Fatalf("setup form is incomplete:\n%s", getRR.Body.String())
	}

	form := url.Values{
		"csrf":       {csrf},
		"provider":   {"emby"},
		"public_url": {"https://join.example.com"},
		"server_url": {"http://emby:8096"},
		"api_key":    {"api-key"},
	}
	postReq := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range getRR.Result().Cookies() {
		postReq.AddCookie(cookie)
	}
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusSeeOther || postRR.Header().Get("Location") != "/login" {
		t.Fatalf("status = %d, location = %q; body %s", postRR.Code, postRR.Header().Get("Location"), postRR.Body.String())
	}
	if media.provider != mediaserver.ProviderEmby {
		t.Fatalf("active provider = %q", media.provider)
	}
	if store.settings.Provider != "emby" || store.settings.PublicURL != "https://join.example.com" ||
		store.settings.ServerURL != "http://emby:8096/emby" || store.settings.APIKey != "api-key" {
		t.Fatalf("saved settings = %#v", store.settings)
	}
}

func TestSetupRestoresProviderWhenSettingsCannotBeSaved(t *testing.T) {
	store := newFakeStore()
	store.settings.Provider = ""
	store.settings.PublicURL = ""
	store.settings.ServerURL = ""
	store.settings.APIKey = ""
	store.setupSettingsErr = errors.New("save failed")
	cfg := config.Config{MediaProvider: "jellyfin"}
	media := &fakeMediaServer{provider: mediaserver.ProviderJellyfin}
	handler := New(cfg, store, media)

	csrf := "csrf-value"
	form := url.Values{
		"csrf":       {csrf},
		"provider":   {"emby"},
		"public_url": {"http://join.example.com"},
		"server_url": {"http://emby:8096"},
		"api_key":    {"api-key"},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  anonymousCSRFCookie,
		Value: security.HashToken(store.settings.SessionSecret, csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if media.provider != mediaserver.ProviderJellyfin {
		t.Fatalf("provider = %q, want restored Jellyfin provider", media.provider)
	}
}

func TestLoginInvalidCSRFDoesNotConsumeRateLimit(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{})

	for range 10 {
		form := url.Values{"username": {"admin"}, "password": {"password"}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid CSRF status = %d, want %d", rr.Code, http.StatusBadRequest)
		}
	}

	csrf := "csrf-value"
	form := url.Values{"csrf": {csrf}, "username": {"admin"}, "password": {"password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  anonymousCSRFCookie,
		Value: security.HashToken(store.settings.SessionSecret, csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("valid login status = %d, want %d; body %s", rr.Code, http.StatusSeeOther, rr.Body.String())
	}
}

func TestLoginRateLimitsRotatingUsernamesByClientIP(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, &fakeMediaServer{authErr: errors.New("bad credentials")})
	csrf := "csrf-value"

	for i := range 51 {
		form := url.Values{
			"csrf":     {csrf},
			"username": {fmt.Sprintf("user-%d", i)},
			"password": {"wrong"},
		}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.RemoteAddr = "192.0.2.10:1234"
		req.AddCookie(&http.Cookie{Name: anonymousCSRFCookie, Value: security.HashToken(store.settings.SessionSecret, csrf)})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if i < 50 && rr.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want login response", i+1, rr.Code)
		}
		if i == 50 && rr.Code != http.StatusTooManyRequests {
			t.Fatalf("attempt %d status = %d, want 429", i+1, rr.Code)
		}
	}
}
