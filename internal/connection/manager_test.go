package connection

import (
	"context"
	"errors"
	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"path/filepath"
	"testing"
	"time"
)

type identityMedia struct {
	mediaserver.Server
	id    string
	calls int
}

func (m *identityMedia) Inspect(context.Context, string, string, string) (mediaserver.ServerInfo, error) {
	m.calls++
	return mediaserver.ServerInfo{ID: m.id, Name: "Synthetic"}, nil
}

type acknowledgementStore struct {
	*db.Store
	fail bool
}

func (s *acknowledgementStore) PublishMediaConnection(ctx context.Context, u db.ConnectionUpdate) (db.MediaConnection, error) {
	v, err := s.Store.PublishMediaConnection(ctx, u)
	if err == nil && s.fail {
		s.fail = false
		return db.MediaConnection{}, errors.New("acknowledgement lost")
	}
	return v, err
}
func fixture(t *testing.T) (*db.Store, *Manager, *identityMedia) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "aperture.db"), "synthetic-encryption-key-at-least-32")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.InitSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"media_provider": "jellyfin", "public_url": "https://join.test", "server_url": "http://media.test", "api_key": "synthetic-key"} {
		if err := store.SetSetting(t.Context(), key, value, key == "api_key"); err != nil {
			t.Fatal(err)
		}
	}
	media := &identityMedia{id: "server-A"}
	manager := New(config.Config{}, store, func(mediaserver.Provider) (mediaserver.Server, error) { return media, nil })
	return store, manager, media
}
func verified(t *testing.T, m *Manager) Snapshot {
	t.Helper()
	s, err := m.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	s, err = m.Verify(t.Context(), s, "synthetic-token", "device")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func session(t *testing.T, store *db.Store, s Snapshot) string {
	t.Helper()
	id, _, err := store.CreateSession(t.Context(), db.SessionInput{UserID: "admin", Username: "admin", AccessToken: "secret", DeviceID: "device", TTL: time.Hour, BindingID: s.Identity.Binding.ID, Generation: s.Identity.Generation})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOriginChangesRevokeSessionsButKeyRotationPreservesThem(t *testing.T) {
	store, m, media := fixture(t)
	old := verified(t, m)
	id := session(t, store, old)
	target := old.Settings
	target.APIKey = "rotated-key"
	rotated, err := m.Publish(t.Context(), old, target, db.ConnectionUpdate{APIKey: &target.APIKey})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Identity != old.Identity {
		t.Fatal("credential rotation changed ownership")
	}
	if _, err := store.Session(t.Context(), id); err != nil {
		t.Fatal("same-origin session revoked")
	}
	target.ServerURL = "http://clone.test"
	clone, err := m.Publish(t.Context(), rotated, target, db.ConnectionUpdate{ServerURL: &target.ServerURL})
	if err != nil {
		t.Fatal(err)
	}
	if clone.Identity.Binding.ID == old.Identity.Binding.ID || clone.Identity.Binding.ServerID != old.Identity.Binding.ServerID {
		t.Fatal("cloned server ID aliased another URL")
	}
	if _, err := store.Session(t.Context(), id); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("old session survived URL change")
	}
	if old.Settings.ServerURL != "http://media.test" || old.Settings.APIKey != "synthetic-key" {
		t.Fatal("accepted operation snapshot changed")
	}
	if _, err := m.Publish(t.Context(), old, old.Settings, db.ConnectionUpdate{}); !errors.Is(err, db.ErrConnectionChanged) {
		t.Fatal("stale settings overwrote new origin")
	}
	id = session(t, store, clone)
	media.id = "replacement-server"
	replacement := verified(t, m)
	if replacement.Identity.Binding.ID == clone.Identity.Binding.ID {
		t.Fatal("replacement reused ownership")
	}
	if _, err := store.Session(t.Context(), id); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("replacement kept old session")
	}
	if _, _, err := store.CreateSession(t.Context(), db.SessionInput{BindingID: clone.Identity.Binding.ID, Generation: clone.Identity.Generation, TTL: time.Hour}); !errors.Is(err, db.ErrConnectionChanged) {
		t.Fatal("in-flight login created old-origin session")
	}
}

func TestEnvironmentOriginIsInvalidatedBeforeNetworkUse(t *testing.T) {
	store, m, media := fixture(t)
	old := verified(t, m)
	id := session(t, store, old)
	calls := media.calls
	restarted := New(config.Config{ServerURLManaged: true, ServerURL: "http://new-media.test"}, store, func(mediaserver.Provider) (mediaserver.Server, error) { return media, nil })
	s, err := restarted.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if s.Identity.Binding.ID != 0 || s.Verified || media.calls != calls {
		t.Fatal("startup trusted or contacted an unverified origin")
	}
	if _, err := store.Session(t.Context(), id); !errors.Is(err, db.ErrNotFound) {
		t.Fatal("environment move did not revoke session")
	}
}

func TestPublicationReloadsAfterLostAcknowledgement(t *testing.T) {
	store, m, _ := fixture(t)
	old := verified(t, m)
	wrapper := &acknowledgementStore{Store: store, fail: true}
	m.store = wrapper
	target := old.Settings
	target.ServerURL = "http://new.test"
	if _, err := m.Publish(t.Context(), old, target, db.ConnectionUpdate{ServerURL: &target.ServerURL}); err == nil {
		t.Fatal("missing injected failure")
	}
	current, err := m.Current(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if current.Settings.ServerURL != target.ServerURL || current.Identity.Generation <= old.Identity.Generation {
		t.Fatal("cached old state survived ambiguous publication")
	}
	verified(t, m)
	if _, err := m.Publish(t.Context(), old, old.Settings, db.ConnectionUpdate{}); !errors.Is(err, db.ErrConnectionChanged) {
		t.Fatal("reload reused a stale snapshot version")
	}
}

func TestOptionalAPIKeyCanBindUsingLoginToken(t *testing.T) {
	store, m, media := fixture(t)
	old := verified(t, m)
	target := old.Settings
	target.ServerURL = "http://login-only.test"
	target.APIKey = ""
	calls := media.calls
	s, err := m.Publish(t.Context(), old, target, db.ConnectionUpdate{ServerURL: &target.ServerURL, APIKey: &target.APIKey})
	if err != nil {
		t.Fatal(err)
	}
	if s.Identity.Binding.ID != 0 || media.calls != calls {
		t.Fatal("keyless setup was rejected or guessed ownership")
	}
	s, err = m.Verify(t.Context(), s, "administrator-login-token", "login-device")
	if err != nil {
		t.Fatal(err)
	}
	session(t, store, s)
}
