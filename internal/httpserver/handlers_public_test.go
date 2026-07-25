package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/security"
)

func TestPublicRegisterSchedulesUserDisable(t *testing.T) {
	store := newFakeStore()
	store.invite.UserExpiryDays = 7
	media := &fakeMediaServer{}
	handler := New(testConfig(), store, media)
	token := "public-invite-token"
	csrf := "csrf-value"
	form := url.Values{
		"csrf":             {csrf},
		"username":         {"new_user"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}
	req := httptest.NewRequest(http.MethodPost, "/i/"+token+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.0.2.1:1234"
	req.AddCookie(&http.Cookie{
		Name:  publicCSRFCookie,
		Value: security.HashToken(store.settings.InviteSecret, token+":"+csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
	}
	if location := rr.Header().Get("Location"); location != "/guide" {
		t.Fatalf("Location = %q, want /guide", location)
	}
	if !media.createdUser || !media.appliedTemplate {
		t.Fatalf("media-server create/apply = %v/%v, want both true", media.createdUser, media.appliedTemplate)
	}
	if !store.beganUserCreation {
		t.Fatal("registration did not enter creating-user state before media-server call")
	}
	if store.recordedUserID != "new-media-user" {
		t.Fatalf("recorded media-server user ID = %q", store.recordedUserID)
	}
	if !store.completedDisableAt.Valid {
		t.Fatal("expected registration to receive a disable timestamp")
	}
	if until := time.Until(store.completedDisableAt.Time); until < 6*24*time.Hour || until > 8*24*time.Hour {
		t.Fatalf("disable timestamp is %s away, want about 7 days", until)
	}
}

func TestPublicRegisterChecksTemplateBeforeReservingInvite(t *testing.T) {
	store := newFakeStore()
	store.templateErr = errors.New("template read failed")
	handler := New(testConfig(), store, &fakeMediaServer{})
	token := "public-invite-token"
	csrf := "csrf-value"
	form := url.Values{
		"csrf":             {csrf},
		"username":         {"new_user"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}
	req := httptest.NewRequest(http.MethodPost, "/i/"+token+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  publicCSRFCookie,
		Value: security.HashToken(store.settings.InviteSecret, token+":"+csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rr.Code, rr.Body.String())
	}
	if store.reservedInviteUse {
		t.Fatal("invite use was reserved before template prerequisites succeeded")
	}
}

func TestPublicRegisterRetainsInviteUseAfterUncertainUserCreationFailure(t *testing.T) {
	store := newFakeStore()
	media := &fakeMediaServer{createErr: errors.New("create failed")}
	handler := New(testConfig(), store, media)
	token := "public-invite-token"
	csrf := "csrf-value"
	form := url.Values{
		"csrf":             {csrf},
		"username":         {"new_user"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}
	req := httptest.NewRequest(http.MethodPost, "/i/"+token+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{
		Name:  publicCSRFCookie,
		Value: security.HashToken(store.settings.InviteSecret, token+":"+csrf),
	})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body %s", rr.Code, rr.Body.String())
	}
	if !store.beganUserCreation || store.completedStatus != db.RegistrationFailedCreateUser {
		t.Fatalf("creation phase/status = %t/%q", store.beganUserCreation, store.completedStatus)
	}
	if store.recordedUserID != "" {
		t.Fatalf("failed creation recorded media-server user ID %q", store.recordedUserID)
	}
}

func TestPublicRegisterWithoutAPIKeyDoesNotConsumeInvite(t *testing.T) {
	store := newFakeStore()
	store.settings.APIKey = ""
	cfg := testConfig()
	cfg.APIKey = ""
	cfg.APIKeyManaged = false
	handler := New(cfg, store, &fakeMediaServer{})
	token := "public-invite-token"
	csrf := "csrf-value"
	form := url.Values{
		"csrf":             {csrf},
		"username":         {"new_user"},
		"password":         {"correct horse"},
		"confirm_password": {"correct horse"},
	}
	req := httptest.NewRequest(http.MethodPost, "/i/"+token+"/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: publicCSRFCookie, Value: security.HashToken(store.settings.InviteSecret, token+":"+csrf)})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body %s", rr.Code, rr.Body.String())
	}
	if store.reservedInviteUse {
		t.Fatal("invite use was reserved without an API key")
	}
}

func TestPublicInviteShowsOnlyAccountCreationDetails(t *testing.T) {
	store := newFakeStore()
	store.invite.Label = "Admin label"
	store.invite.Template = "Internal template"
	store.invite.UserExpiryDays = 14
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/i/public-token", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Create your account", "Choose the username and password.", "Username", "Password"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q:\n%s", want, body)
		}
	}
	for _, unwanted := range []string{"Admin label", "Internal template", "scheduled account expiry", "Expires after"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body leaked admin-only invite detail %q:\n%s", unwanted, body)
		}
	}
}

func TestPublicInviteFailsEarlyWithoutAPIKey(t *testing.T) {
	store := newFakeStore()
	store.settings.APIKey = ""
	cfg := testConfig()
	cfg.APIKey = ""
	cfg.APIKeyManaged = false
	handler := New(cfg, store, &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/i/public-token", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "Registration unavailable") {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	if len(rr.Result().Cookies()) != 0 {
		t.Fatal("unusable invite issued a CSRF cookie")
	}
}

func TestPublicInviteUnavailableDoesNotRevealDetails(t *testing.T) {
	store := newFakeStore()
	store.inviteErr = db.ErrNotFound
	handler := New(testConfig(), store, &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/i/missing-token", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Invite unavailable") || strings.Contains(body, "not found") {
		t.Fatalf("public error leaked details or missed friendly copy:\n%s", body)
	}
}

func TestGuideRendersAccountCreatedMessage(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), &fakeMediaServer{})
	req := httptest.NewRequest(http.MethodGet, "/guide", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{"Account created", "Sign in to the media server"} {
		if !strings.Contains(body, want) {
			t.Fatalf("guide missing %q:\n%s", want, body)
		}
	}
}
