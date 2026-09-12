package httpserver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/security"
)

func TestPublicRegisterSchedulesUserDisable(t *testing.T) {
	store := newFakeStore()
	store.invite.UserExpiryDays = 7
	media := &fakeMediaServer{}
	handler := New(testConfig(), store, testMediaFactory(media))
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
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
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
	handler := New(testConfig(), store, testMediaFactory(media))
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
	handler := New(cfg, store, testMediaFactory(&fakeMediaServer{}))
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
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
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
	handler := New(cfg, store, testMediaFactory(&fakeMediaServer{}))
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
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
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
	handler := New(testConfig(), newFakeStore(), testMediaFactory(&fakeMediaServer{}))
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

type provisioningStore struct {
	*fakeStore
	recordError   error
	completeError error
}

func (s *provisioningStore) RecordCreatedUser(ctx context.Context, id int64, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.recordError != nil {
		return s.recordError
	}
	return s.fakeStore.RecordCreatedUser(ctx, id, userID)
}

func (s *provisioningStore) CompleteRegistration(ctx context.Context, id int64, status, message string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.completeError != nil {
		return s.completeError
	}
	return s.fakeStore.CompleteRegistration(ctx, id, status, message)
}

type provisioningMedia struct {
	*fakeMediaServer
	cancel       context.CancelFunc
	partialError error
}

func (m *provisioningMedia) CreateUser(ctx context.Context, baseURL, key, username, password string, created func(mediaserver.User) error) (mediaserver.User, error) {
	user, err := m.fakeMediaServer.CreateUser(ctx, baseURL, key, username, password, created)
	if m.cancel != nil {
		m.cancel()
	}
	if m.partialError != nil {
		return user, m.partialError
	}
	return user, err
}

func (m *provisioningMedia) ApplyTemplate(ctx context.Context, baseURL, key, userID string, template db.Template) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.fakeMediaServer.ApplyTemplate(ctx, baseURL, key, userID, template)
}

func (m *provisioningMedia) DisableUser(ctx context.Context, baseURL, key, userID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.fakeMediaServer.DisableUser(ctx, baseURL, key, userID)
}

func TestPublicRegisterProtectsIncompleteProvisioning(t *testing.T) {
	for _, scenario := range []string{"disconnect after create", "password failure", "template failure", "record failure", "completion failure"} {
		t.Run(scenario, func(t *testing.T) {
			store := &provisioningStore{fakeStore: newFakeStore()}
			store.invite.UserExpiryDays = 7
			media := &provisioningMedia{fakeMediaServer: &fakeMediaServer{}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch scenario {
			case "disconnect after create":
				media.cancel = cancel
			case "password failure":
				media.partialError = errors.New("password setup failed")
			case "template failure":
				media.applyErr = errors.New("policy update failed")
			case "record failure":
				store.recordError = errors.New("database write failed")
			case "completion failure":
				store.completeError = errors.New("database write failed")
			}
			handler := New(testConfig(), store, testMediaFactory(media))
			token, csrf := "public-invite-token", "csrf-value"
			form := url.Values{"csrf": {csrf}, "username": {"new_user"}, "password": {"correct horse"}, "confirm_password": {"correct horse"}}
			req := httptest.NewRequest(http.MethodPost, "/i/"+token+"/register", strings.NewReader(form.Encode())).WithContext(ctx)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: publicCSRFCookie, Value: security.HashToken(store.settings.InviteSecret, token+":"+csrf)})
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if scenario == "disconnect after create" {
				if rr.Code != http.StatusSeeOther || !media.appliedTemplate || store.recordedUserID != "new-media-user" || !store.completedDisableAt.Valid || media.disabledUserID != "" {
					t.Fatalf("disconnect interrupted provisioning: status=%d applied=%t persisted=%q expiry=%t disabled=%q", rr.Code, media.appliedTemplate, store.recordedUserID, store.completedDisableAt.Valid, media.disabledUserID)
				}
			} else if media.disabledUserID != "new-media-user" {
				t.Fatalf("incomplete account was not disabled: %q", media.disabledUserID)
			}
			if scenario == "password failure" && (store.completedStatus != db.RegistrationFailedCreateUser || store.recordedUserID != "new-media-user" || media.appliedTemplate) {
				t.Fatalf("partial password setup became eligible for template retry: status=%q user=%q applied=%t", store.completedStatus, store.recordedUserID, media.appliedTemplate)
			}
		})
	}
}
