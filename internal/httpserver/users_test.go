package httpserver

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestUsersPageMergesLiveImportedAndInviteUsers(t *testing.T) {
	store := newFakeStore()
	store.managedUsers = []db.ManagedUser{{ExternalUserID: "external-1", Username: "External"}}
	store.registrations = []db.Registration{{ID: 7, ExternalUserID: sql.NullString{String: "invite-1", Valid: true}, Username: "Invite User"}}
	media := &fakeMediaServer{users: []mediaserver.User{
		{ID: "external-1", Name: "External", Policy: []byte(`{"IsDisabled":false}`)},
		{ID: "invite-1", Name: "Invite User", Policy: []byte(`{}`)},
		{ID: "untracked-1", Name: "Untracked", Policy: []byte(`{}`)},
	}}
	handler := New(testConfig(), store, media)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin/users", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{"Imported", "Invite", "External", "Track", "Delete"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("body missing %q:\n%s", want, rr.Body.String())
		}
	}
}

func TestUsersTrackRejectsAdministrators(t *testing.T) {
	store := newFakeStore()
	media := &fakeMediaServer{users: []mediaserver.User{{ID: "admin-1", Name: "Admin", Policy: []byte(`{"IsAdministrator":true}`)}}}
	handler := New(testConfig(), store, media)
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/users/admin-1/track", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict || len(store.managedUsers) != 0 {
		t.Fatalf("status/users = %d/%#v", rr.Code, store.managedUsers)
	}
}

func TestUsersDeleteRemovesUpstreamAndTrackedRecord(t *testing.T) {
	store := newFakeStore()
	store.managedUsers = []db.ManagedUser{{ExternalUserID: "user-1", Username: "Alice"}}
	media := &fakeMediaServer{users: []mediaserver.User{{ID: "user-1", Name: "Alice", Policy: []byte(`{}`)}}}
	handler := New(testConfig(), store, media)
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/users/user-1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther || media.deletedUserID != "user-1" || len(store.managedUsers) != 0 {
		t.Fatalf("status/deleted/managed = %d/%q/%#v", rr.Code, media.deletedUserID, store.managedUsers)
	}
}

func TestUsersDeleteProtectsAdministrators(t *testing.T) {
	store := newFakeStore()
	store.managedUsers = []db.ManagedUser{{ExternalUserID: "admin-1", Username: "Admin"}}
	media := &fakeMediaServer{users: []mediaserver.User{{ID: "admin-1", Name: "Admin", Policy: []byte(`{"IsAdministrator":true}`)}}}
	handler := New(testConfig(), store, media)
	form := url.Values{"csrf": {store.session.CSRFSecret}}
	req := adminRequest(t, http.MethodPost, "/admin/users/admin-1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict || media.deletedUserID != "" || len(store.managedUsers) != 1 {
		t.Fatalf("status/deleted/managed = %d/%q/%#v", rr.Code, media.deletedUserID, store.managedUsers)
	}
}
