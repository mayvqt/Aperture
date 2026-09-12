package httpserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/security"
)

type acknowledgementFailureStore struct{ Store }

func (s acknowledgementFailureStore) CompleteTemplateRecovery(ctx context.Context, id int64) error {
	if err := s.Store.CompleteTemplateRecovery(ctx, id); err != nil {
		return err
	}
	return errors.New("completion acknowledgement lost")
}

type pausedReservationStore struct {
	*fakeStore
	entered, release chan struct{}
}

func (s *pausedReservationStore) ReserveInviteUse(ctx context.Context, id, bindingID int64, ip, ua, username string) (int64, db.Template, error) {
	reg, tmpl, err := s.fakeStore.ReserveInviteUse(ctx, id, bindingID, ip, ua, username)
	close(s.entered)
	<-s.release
	return reg, tmpl, err
}

func TestShutdownWaitsForReservationAndRejectsLateProvisioning(t *testing.T) {
	store := &pausedReservationStore{fakeStore: newFakeStore(), entered: make(chan struct{}), release: make(chan struct{})}
	media := &fakeMediaServer{}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	form := url.Values{"csrf": {"csrf"}, "username": {"new_user"}, "password": {"correct horse"}, "confirm_password": {"correct horse"}}
	req := httptest.NewRequest(http.MethodPost, "/i/token/register", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: publicCSRFCookie, Value: security.HashToken(store.settings.InviteSecret, "token:csrf")})
	finished := make(chan struct{})
	go func() { defer close(finished); s.ServeHTTP(httptest.NewRecorder(), req) }()
	<-store.entered
	s.CloseAdmission()
	drained := make(chan error, 1)
	go func() { drained <- s.Drain(t.Context()) }()
	select {
	case <-drained:
		t.Fatal("drain returned while a reservation handler still used the store")
	case <-time.After(20 * time.Millisecond):
	}
	close(store.release)
	<-finished
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if media.createdUser || store.beganUserCreation {
		t.Fatal("provisioning started after shutdown admission closed")
	}
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/guide", nil))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("new request accepted during shutdown: %d", rr.Code)
	}
}

type recoveryRoundTrip func(*http.Request) (*http.Response, error)

func (f recoveryRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRecoveryPreservesFailureAlertsAndCleanupMeaning(t *testing.T) {
	notices := make(chan webhookNotice, 4)
	previous := http.DefaultTransport
	http.DefaultTransport = recoveryRoundTrip(func(r *http.Request) (*http.Response, error) {
		var n struct{ Event, Title, Description string }
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			return nil, err
		}
		notices <- webhookNotice{Event: n.Event, Title: n.Title, Description: n.Description}
		return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	defer func() { http.DefaultTransport = previous }()
	store := newFakeStore()
	store.webhooks = []db.Webhook{{URL: "http://webhook.test/events", Kind: "generic", Enabled: true, Events: "template.failed,user.disable_failed,user.disabled"}}
	store.recovery.Registration.TemplateAttempts = 5
	media := &fakeMediaServer{applyErr: errors.New("response lost"), disableErr: errors.New("offline")}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	if err := s.recoverAccount(verifiedTestContext(t, s), 1, true); err == nil {
		t.Fatal("expected template failure")
	}
	if err := s.waitForWebhooks(t.Context()); err != nil {
		t.Fatal(err)
	}
	seen := map[string]webhookNotice{}
	for len(notices) > 0 {
		n := <-notices
		seen[n.Event] = n
	}
	if _, ok := seen["user.disable_failed"]; !ok {
		t.Fatal("disable failure alert dropped")
	}
	if n, ok := seen["template.failed"]; !ok || !strings.Contains(n.Description, "finished") {
		t.Fatalf("final retry alert=%+v", n)
	}
	media.disableErr = nil
	r := store.recovery.Registration
	r.UserDisableAt = sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}
	if err := s.disableAccount(verifiedTestContext(t, s), r); err != nil {
		t.Fatal(err)
	}
	if err := s.waitForWebhooks(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := <-notices; n.Event != "user.disabled" || n.Title != "Incomplete account disabled" {
		t.Fatalf("early cleanup misreported: %+v", n)
	}
}

func TestCommittedRecoveryIsNotDisabledWhenAcknowledgementFails(t *testing.T) {
	store, _, id := recoveryFixture(t)
	media := &fakeMediaServer{}
	s := NewServer(testConfig(), acknowledgementFailureStore{store}, testMediaFactory(media))
	if err := s.recoverAccount(verifiedTestContext(t, s), id, false); err != nil {
		t.Fatalf("confirmed completion reported failure: %v", err)
	}
	r, err := store.Registration(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != db.RegistrationComplete || r.CleanupPending || media.disabledUserID != "" {
		t.Fatalf("committed recovery was disabled: %+v disabled=%q", r, media.disabledUserID)
	}
}

func TestFailedPolicyResponseLeavesDurableCleanupAfterRetryLimit(t *testing.T) {
	store, _, id := recoveryFixture(t)
	media := &fakeMediaServer{applyErr: errors.New("policy response lost"), disableErr: errors.New("offline")}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	for i := 0; i < 6; i++ {
		if err := s.recoverAccount(verifiedTestContext(t, s), id, false); err == nil {
			t.Fatal("expected policy failure")
		}
	}
	r, err := store.Registration(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.TemplateAttempts != 6 || !r.CleanupPending || r.DisableAttempts != 6 || !r.NextDisableAttemptAt.Valid {
		t.Fatalf("cleanup lost: %+v", r)
	}
	if ids, err := store.ListDueTemplateRecoveryIDs(t.Context(), 1, 10); err != nil || len(ids) > 0 {
		t.Fatalf("exhausted retries=%v %v", ids, err)
	}
	// Retry only the security cleanup using the same persisted registration.
	media.disableErr = nil
	if err := s.disableAccount(verifiedTestContext(t, s), r); err != nil {
		t.Fatal(err)
	}
	r, err = store.Registration(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.CleanupPending || r.Status != db.RegistrationNeedsAttention {
		t.Fatalf("cleanup result=%+v", r)
	}
}

func TestExpiredRecoveryOnlyAttemptsDisable(t *testing.T) {
	store := newFakeStore()
	store.recovery.Registration.UserDisableAt = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
	media := &fakeMediaServer{disableErr: errors.New("offline")}
	s := NewServer(testConfig(), store, testMediaFactory(media))
	if err := s.recoverAccount(verifiedTestContext(t, s), 1, false); err == nil {
		t.Fatal("expected disable failure")
	}
	if media.appliedTemplate || store.recoveryFailure != "" || store.failedDisableID != 1 || store.recordedUserID != "" {
		t.Fatal("expired recovery ran template failure/compensation path")
	}
}

func TestAccountOperationGuardsAreScopedAndReleased(t *testing.T) {
	s := NewServer(testConfig(), newFakeStore(), testMediaFactory(&fakeMediaServer{}))
	release, err := s.claimAccountOperations(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.claimAccountOperations(2, 3); !errors.Is(err, db.ErrRegistrationTransition) {
		t.Fatalf("overlapping operation accepted: %v", err)
	}
	other, err := s.claimAccountOperations(3)
	if err != nil {
		t.Fatalf("independent account blocked: %v", err)
	}
	other()
	release()
	release, err = s.claimAccountOperations(1)
	if err != nil {
		t.Fatal(err)
	}
	release()
	if err := s.Drain(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func recoveryFixture(t *testing.T) (*db.Store, db.Settings, int64) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "aperture.db"), "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ctx := t.Context()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"media_provider": "jellyfin", "public_url": "https://join.test", "server_url": "http://media.test", "api_key": "synthetic-key"} {
		if err := store.SetSetting(ctx, key, value, key == "api_key"); err != nil {
			t.Fatal(err)
		}
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishMediaConnection(ctx, db.ConnectionUpdate{Origin: db.MediaBinding{Provider: "jellyfin", BaseURL: "http://media:8096", ServerID: "synthetic-server"}}); err != nil {
		t.Fatal(err)
	}
	inviteID, err := store.CreateInvite(ctx, db.Invite{TokenHash: "retry-fixture", TemplateID: 1, MaxUses: 1, UserExpiryDays: 7, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, id, "alice-id"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, id, db.RegistrationNeedsAttention, "response lost"); err != nil {
		t.Fatal(err)
	}
	return store, settings, id
}
