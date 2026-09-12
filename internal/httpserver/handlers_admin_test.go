package httpserver

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestAdminDashboardLeavesDueUserDisablesToBackgroundWorker(t *testing.T) {
	store := newFakeStore()
	store.dueDisables = []db.Registration{{
		ID:             42,
		ExternalUserID: sql.NullString{String: "media-expired", Valid: true},
	}}
	media := &fakeMediaServer{}
	handler := New(testConfig(), store, testMediaFactory(media))
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	if media.disabledUserID != "" || store.markedDisabledID != 0 {
		t.Fatal("dashboard processed user expiry instead of leaving it to the background worker")
	}
}

func TestSettingsEnvironmentValuesAreManagedAndNotWritable(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))

	getReq := adminRequest(t, http.MethodGet, "/admin/settings", nil)
	getRR := httptest.NewRecorder()
	handler.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body %s", getRR.Code, getRR.Body.String())
	}
	body := getRR.Body.String()
	for _, want := range []string{"Managed by APERTURE_MEDIA_PROVIDER", "Managed by APERTURE_PUBLIC_URL", "Managed by APERTURE_SERVER_URL", "Managed by APERTURE_API_KEY"} {
		if !strings.Contains(getRR.Body.String(), want) {
			t.Fatalf("settings body missing %q:\n%s", want, getRR.Body.String())
		}
	}
	if !htmlElementHasAttributes(body, "input", map[string]string{
		"name":     "server_url",
		"value":    "http://media:8096",
		"required": "",
		"disabled": "",
	}) {
		t.Fatalf("settings body missing managed server URL input:\n%s", body)
	}
	if !htmlElementHasAttributes(body, "input", map[string]string{
		"name":         "api_key",
		"type":         "password",
		"autocomplete": "off",
		"placeholder":  "leave blank to keep saved key",
		"disabled":     "",
	}) {
		t.Fatalf("settings body missing managed API key input:\n%s", body)
	}
	if strings.Contains(body, ">Save settings</button>") {
		t.Fatalf("fully managed settings still show save action:\n%s", getRR.Body.String())
	}

	form := url.Values{
		"csrf":       {store.session.CSRFSecret},
		"server_url": {"http://attacker.invalid"},
		"api_key":    {"forged-key"},
	}
	postReq := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusSeeOther {
		t.Fatalf("POST status = %d, want redirect; body %s", postRR.Code, postRR.Body.String())
	}
	if len(store.settingWrites) != 0 {
		t.Fatalf("managed settings writes = %#v, want none", store.settingWrites)
	}
}

func TestSettingsUpdatesAllBrowserManagedApplicationSettings(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.PublicURL = "https://old.example.com"
	cfg.MediaProvider = "jellyfin"
	cfg.ServerURL = ""
	cfg.APIKey = ""
	cfg.PublicURLManaged = false
	cfg.ProviderManaged = false
	cfg.ServerURLManaged = false
	cfg.APIKeyManaged = false
	media := &fakeMediaServer{provider: mediaserver.ProviderJellyfin}
	handler := New(cfg, store, testMediaFactory(media))

	form := url.Values{
		"csrf":       {store.session.CSRFSecret},
		"provider":   {"emby"},
		"public_url": {"https://join.example.com"},
		"server_url": {"http://emby:8096"},
		"api_key":    {"new-api-key"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want redirect; body %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Location") != "/login" || store.deletedSessionID != store.session.ID {
		t.Fatalf("provider switch did not clear the old media-server session")
	}
	if handler.(*Server).connectionsStateProvider() != "emby" {
		t.Fatalf("active provider = %q, want Emby", media.provider)
	}
	if store.settings.Provider != "emby" || store.settings.PublicURL != "https://join.example.com" ||
		store.settings.ServerURL != "http://emby:8096/emby" || store.settings.APIKey != "new-api-key" {
		t.Fatalf("saved settings = %#v", store.settings)
	}
	if len(store.settingWrites) != 4 {
		t.Fatalf("setting writes = %#v, want four", store.settingWrites)
	}
}

func TestSettingsCanRemoveSavedAPIKey(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.APIKey = ""
	cfg.APIKeyManaged = false
	handler := New(cfg, store, testMediaFactory(&fakeMediaServer{}))
	form := url.Values{
		"csrf":           {store.session.CSRFSecret},
		"remove_api_key": {"on"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want redirect; body %s", rr.Code, rr.Body.String())
	}
	if store.settings.APIKey != "" {
		t.Fatalf("saved API key was not removed")
	}
}

func TestSettingsNeverRendersAPIKey(t *testing.T) {
	store := newFakeStore()
	store.settings.APIKey = "ui-stored-super-secret-key"
	store.settings.SessionSecret = "stored-session-secret-never-render"
	store.settings.InviteSecret = "stored-invite-secret-never-render"
	store.session.AccessToken = "admin-access-token-never-render"
	store.session.DeviceID = "admin-device-id-never-render"
	cfg := testConfig()
	cfg.APIKey = "environment-api-key-never-render"
	cfg.APIKeyManaged = false
	handler := New(cfg, store, testMediaFactory(&fakeMediaServer{}))

	req := adminRequest(t, http.MethodGet, "/admin/settings", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, secret := range []string{
		store.settings.APIKey,
		store.settings.SessionSecret,
		store.settings.InviteSecret,
		store.session.AccessToken,
		store.session.DeviceID,
		cfg.APIKey,
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("settings page exposed backend secret %q", secret)
		}
	}
	if strings.Contains(body, "********") {
		t.Fatalf("settings page rendered a secret placeholder:\n%s", body)
	}
	if strings.Contains(body, `name="api_key" value=`) {
		t.Fatalf("API-key input must always be empty:\n%s", body)
	}

	form := url.Values{
		"csrf":       {store.session.CSRFSecret},
		"server_url": {"not-a-url"},
		"api_key":    {"new-submitted-super-secret-key"},
	}
	postReq := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRR := httptest.NewRecorder()
	handler.ServeHTTP(postRR, postReq)
	if strings.Contains(postRR.Body.String(), "new-submitted-super-secret-key") || strings.Contains(postRR.Body.String(), "********") {
		t.Fatalf("validation response exposed API-key material:\n%s", postRR.Body.String())
	}
}

func TestSettingsPostWritesOnlyUIManagedValues(t *testing.T) {
	tests := []struct {
		name       string
		configure  func(*config.Config)
		wantKey    string
		wantValue  string
		wantSecret bool
	}{
		{
			name: "environment URL leaves API key editable",
			configure: func(cfg *config.Config) {
				cfg.APIKey = ""
				cfg.APIKeyManaged = false
			},
			wantKey: "api_key", wantValue: "new-api-key", wantSecret: true,
		},
		{
			name: "environment API key leaves URL editable",
			configure: func(cfg *config.Config) {
				cfg.ServerURL = ""
				cfg.ServerURLManaged = false
			},
			wantKey: "server_url", wantValue: "http://new-media:8096",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			cfg := testConfig()
			tt.configure(&cfg)
			handler := New(cfg, store, testMediaFactory(&fakeMediaServer{}))
			form := url.Values{
				"csrf":       {store.session.CSRFSecret},
				"server_url": {"http://new-media:8096"},
				"api_key":    {"new-api-key"},
			}
			req := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want redirect; body %s", rr.Code, rr.Body.String())
			}
			if len(store.settingWrites) != 1 {
				t.Fatalf("setting writes = %#v, want one", store.settingWrites)
			}
			got := store.settingWrites[0]
			if got.key != tt.wantKey || got.value != tt.wantValue || got.secret != tt.wantSecret {
				t.Fatalf("setting write = %#v, want key=%q value=%q secret=%t", got, tt.wantKey, tt.wantValue, tt.wantSecret)
			}
		})
	}
}

func TestSettingsPostRejectsUnreachableConnectionBeforeWriting(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.ServerURL = ""
	cfg.APIKey = ""
	cfg.ServerURLManaged = false
	cfg.APIKeyManaged = false
	handler := New(cfg, store, testMediaFactory(&fakeMediaServer{inspectErr: errFakePing}))
	form := url.Values{
		"csrf":       {store.session.CSRFSecret},
		"server_url": {"http://new-media:8096"},
		"api_key":    {"new-api-key"},
	}
	req := adminRequest(t, http.MethodPost, "/admin/settings/media-server", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "Could not verify or save this connection") {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	if len(store.settingWrites) != 0 {
		t.Fatalf("unreachable settings were written: %#v", store.settingWrites)
	}
}

func TestAdminDashboardShowsHealthChecks(t *testing.T) {
	store := newFakeStore()
	store.invites = nil
	media := &fakeMediaServer{pingErr: errFakePing}
	handler := New(testConfig(), store, testMediaFactory(media))
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Jellyfin check failed", "No active invites", "Check settings"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing health check %q:\n%s", want, body)
		}
	}
}

func TestAdminDashboardDoesNotRenderRetainedInviteTokens(t *testing.T) {
	store := newFakeStore()
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
	req := adminRequest(t, http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	for _, secret := range []string{store.invite.Token, store.invite.TokenHash, store.invite.TokenPrefix} {
		if secret != "" && strings.Contains(rr.Body.String(), secret) {
			t.Fatalf("dashboard rendered invite token material %q", secret)
		}
	}
}

func TestAdminDashboardCachesMediaServerHealthCheck(t *testing.T) {
	store := newFakeStore()
	media := &fakeMediaServer{}
	cfg := testConfig()
	cfg.ServerURL = ""
	cfg.APIKey = ""
	cfg.ServerURLManaged = false
	cfg.APIKeyManaged = false
	handler := New(cfg, store, testMediaFactory(media))

	for range 2 {
		req := adminRequest(t, http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
		}
	}

	if media.pingCalls.Load() != 1 {
		t.Fatalf("media-server ping calls = %d, want 1", media.pingCalls.Load())
	}
}

func TestAdminDashboardHealthCacheChangesWithCredentials(t *testing.T) {
	store := newFakeStore()
	media := &fakeMediaServer{}
	cfg := testConfig()
	cfg.ServerURL = ""
	cfg.APIKey = ""
	cfg.ServerURLManaged = false
	cfg.APIKeyManaged = false
	handler := New(cfg, store, testMediaFactory(media))

	request := func() {
		t.Helper()
		req := adminRequest(t, http.MethodGet, "/admin", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
		}
	}

	request()
	server := handler.(*Server)
	current, err := server.connections.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	target := current.Settings
	target.APIKey = "replacement-api-key"
	if _, err := server.connections.Publish(t.Context(), current, target, server.settingsUpdate(target, true)); err != nil {
		t.Fatal(err)
	}
	request()
	if media.pingCalls.Load() != 2 {
		t.Fatalf("media-server ping calls after credential change = %d, want 2", media.pingCalls.Load())
	}
}
