package httpserver

import (
	"context"
	"database/sql"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type observedMedia struct {
	*fakeMediaServer
	inspections, adminChecks int
	serverID                 string
}

func (m *observedMedia) Inspect(context.Context, string, string, string) (mediaserver.ServerInfo, error) {
	m.inspections++
	return mediaserver.ServerInfo{ID: m.serverID}, nil
}
func (m *observedMedia) IsAdmin(context.Context, string, string, string, string) (bool, error) {
	m.adminChecks++
	return true, nil
}

func TestAdminSessionIsRejectedBeforeTokenReachesDifferentOrigin(t *testing.T) {
	store := newFakeStore()
	cfg := testConfig()
	cfg.ServerURL = "http://different-media:8096"
	media := &observedMedia{fakeMediaServer: &fakeMediaServer{}, serverID: "synthetic-server"}
	s := NewServer(cfg, store, testMediaFactory(media))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin", nil))
	if rr.Code != http.StatusSeeOther || media.inspections != 0 || media.adminChecks != 0 {
		t.Fatalf("old token forwarded: status=%d identity=%d admin=%d", rr.Code, media.inspections, media.adminChecks)
	}
}
func TestReplacementAtSameURLDoesNotRunOldSessionHandler(t *testing.T) {
	store := newFakeStore()
	media := &observedMedia{fakeMediaServer: &fakeMediaServer{}, serverID: "replacement"}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin", nil))
	if rr.Code != http.StatusSeeOther || media.inspections != 1 || media.adminChecks != 0 {
		t.Fatal("replacement server accepted prior session")
	}
}
func TestUnverifiedRegistrationCannotDeleteCurrentAccount(t *testing.T) {
	store := newFakeStore()
	store.registrations[0].BindingID = 0
	media := &fakeMediaServer{users: []mediaserver.User{{ID: "media-alice", Policy: []byte(`{"IsAdministrator":false}`)}}}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	form := url.Values{"csrf": {"csrf-secret"}}
	req := adminRequest(t, http.MethodPost, "/admin/registrations/1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict || media.deletedUserID != "" || len(store.registrations) != 1 {
		t.Fatal("unverified registration deleted a current-server account")
	}
}
func TestServerReviewRequiresCurrentAccountAndConfirmation(t *testing.T) {
	store := newFakeStore()
	store.registrations[0].BindingID = 0
	media := &fakeMediaServer{users: []mediaserver.User{{ID: "media-alice", Name: "Current Alice", Policy: []byte(`{"IsAdministrator":false}`)}}}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin/server-review/registration/1", nil))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "Current Alice") || !strings.Contains(rr.Body.String(), "Unverified") {
		t.Fatalf("review omitted account identity: %d", rr.Code)
	}
	req := adminRequest(t, http.MethodPost, "/admin/server-review/registration/1", strings.NewReader("csrf=csrf-secret&binding_id=1&source_id=0"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatal("assignment accepted without confirmation")
	}
	release, err := s.claimAccountOperations(1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin/server-review/registration/1", nil))
	if rr.Code != http.StatusConflict {
		t.Fatal("review bypassed ongoing account operation")
	}
}
func TestUnsafeUserPoliciesCannotPermitDestructiveActions(t *testing.T) {
	for _, raw := range []string{"", `null`, `{}`, `{"IsAdministrator":null}`, `{"IsAdministrator":"false"}`, `{"IsAdministrator":false,"isadministrator":true}`} {
		if !userIsAdministrator(mediaserver.User{Policy: []byte(raw)}) {
			t.Fatalf("unsafe policy allowed account management: %s", raw)
		}
	}
	if userIsAdministrator(mediaserver.User{Policy: []byte(`{"IsAdministrator":false}`)}) {
		t.Fatal("verified regular user rejected")
	}
}
func TestInviteStatusIncludesExpiredAndUnverifiedLinks(t *testing.T) {
	invite := db.Invite{BindingID: 1, Enabled: true, MaxUses: 1, ExpiresAt: sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}}
	if inviteState(invite, 1) != "Expired" {
		t.Fatal("expired link shown active")
	}
	invite.BindingID = 0
	if inviteState(invite, 1) != "Unverified server" {
		t.Fatal("unknown link shown active")
	}
}

func TestInvalidInviteDoesNotContactMediaServer(t *testing.T) {
	store := newFakeStore()
	store.inviteErr = db.ErrNotFound
	media := &observedMedia{fakeMediaServer: &fakeMediaServer{}, serverID: "synthetic-server"}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/i/invalid", nil))
	if rr.Code != http.StatusNotFound || media.inspections != 0 {
		t.Fatal("invalid public token triggered an upstream request")
	}
}

type replacingMaintenanceMedia struct {
	*fakeMediaServer
	inspections int
	disabled    []string
}

func (m *replacingMaintenanceMedia) Inspect(context.Context, string, string, string) (mediaserver.ServerInfo, error) {
	m.inspections++
	id := "synthetic-server"
	if m.inspections >= 3 {
		id = "replacement"
	}
	return mediaserver.ServerInfo{ID: id}, nil
}
func (m *replacingMaintenanceMedia) DisableUser(_ context.Context, _, _, id string) error {
	m.disabled = append(m.disabled, id)
	return nil
}
func TestMaintenanceVerifiesIdentityBeforeEachAccount(t *testing.T) {
	store := newFakeStore()
	store.dueDisables = []db.Registration{{ID: 41, BindingID: 1, Status: db.RegistrationNeedsAttention, CleanupPending: true, ExternalUserID: sql.NullString{String: "first", Valid: true}}, {ID: 42, BindingID: 1, Status: db.RegistrationNeedsAttention, CleanupPending: true, ExternalUserID: sql.NullString{String: "second", Valid: true}}}
	media := &replacingMaintenanceMedia{fakeMediaServer: &fakeMediaServer{}}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	s.runMaintenance(t.Context())
	if len(media.disabled) != 1 || media.disabled[0] != "first" {
		t.Fatalf("batch mutated replacement server: %v", media.disabled)
	}
}
