package httpserver

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestRegistrationTemplateRetryUsesSavedSnapshot(t *testing.T) {
	store := newFakeStore()
	store.recovery.Template.PolicyJSON = `{"EnableAllFolders":false}`
	media := &fakeMediaServer{}
	handler := New(testConfig(), store, testMediaFactory(media))
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/registrations/1/retry-template", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/admin/registrations" {
		t.Fatalf("status/location = %d/%q; body %s", rr.Code, rr.Header().Get("Location"), rr.Body.String())
	}
	if media.appliedUserID != "media-alice" || media.appliedPolicy != store.recovery.Template.PolicyJSON {
		t.Fatalf("applied user/policy = %q/%q", media.appliedUserID, media.appliedPolicy)
	}
	if !store.recoveryCompleted {
		t.Fatal("successful template retry was not completed")
	}
}

func TestRegistrationsPageShowsTemplateRecoveryAction(t *testing.T) {
	store := newFakeStore()
	store.registrations = []db.Registration{{
		BindingID:      1,
		ID:             7,
		Status:         db.RegistrationNeedsAttention,
		ExternalUserID: sql.NullString{String: "media-alice", Valid: true},
		ErrorMessage:   sql.NullString{String: "policy failed", Valid: true},
	}}
	handler := New(testConfig(), store, testMediaFactory(&fakeMediaServer{}))
	req := adminRequest(t, http.MethodGet, "/admin/registrations", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !htmlFormHasElement(
		body,
		map[string]string{
			"action": "/admin/registrations/7/retry-template",
			"method": "post",
		},
		"input",
		map[string]string{
			"name":  "csrf",
			"type":  "hidden",
			"value": "csrf-secret",
		},
	) {
		t.Fatalf("registration retry form missing CSRF input:\n%s", body)
	}
	if !strings.Contains(body, "Retry access") {
		t.Fatalf("body missing recovery action:\n%s", body)
	}
	if strings.Contains(body, "policy failed") {
		t.Fatalf("page exposed persisted diagnostic text:\n%s", body)
	}
}

func TestRegistrationTemplateRetryRecordsFailure(t *testing.T) {
	store := newFakeStore()
	media := &fakeMediaServer{applyErr: errors.New("apply failed")}
	handler := New(testConfig(), store, testMediaFactory(media))
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/registrations/1/retry-template", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body %s", rr.Code, rr.Body.String())
	}
	if store.recoveryFailure == "" || store.recoveryCompleted {
		t.Fatalf("failure/completed = %q/%t", store.recoveryFailure, store.recoveryCompleted)
	}
}

func TestRegistrationDeleteRemovesUpstreamUserWhenPresent(t *testing.T) {
	for _, test := range []struct {
		name               string
		liveUsers          []mediaserver.User
		wantUpstreamDelete bool
	}{
		{name: "user already deleted"},
		{name: "user still exists", liveUsers: []mediaserver.User{{ID: "media-alice", Name: "alice", Policy: []byte(`{"IsAdministrator":false}`)}}, wantUpstreamDelete: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newFakeStore()
			media := &fakeMediaServer{users: test.liveUsers}
			handler := New(testConfig(), store, testMediaFactory(media))
			form := url.Values{"csrf": {store.session.CSRFSecret}}
			req := adminRequest(t, http.MethodPost, "/admin/registrations/1/delete", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want 303; body %s", rr.Code, rr.Body.String())
			}
			if got := media.deletedUserID != ""; got != test.wantUpstreamDelete {
				t.Fatalf("upstream deleted = %t, want %t", got, test.wantUpstreamDelete)
			}
			if len(store.registrations) != 0 {
				t.Fatalf("local registrations remain: %#v", store.registrations)
			}
		})
	}
}
