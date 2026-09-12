package db

import (
	"context"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "aperture.db"), "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}

	seedTestBinding(t, ctx, store)
	// Ordinary lifecycle fixtures represent one already verified server.
	if _, err := store.db.ExecContext(ctx, `CREATE TRIGGER fixture_registration_binding AFTER INSERT ON registrations WHEN NEW.binding_id IS NULL BEGIN UPDATE registrations SET binding_id=1 WHERE id=NEW.id; END`); err != nil {
		t.Fatal(err)
	}
	return ctx, store
}

func seedTestBinding(t *testing.T, ctx context.Context, store *Store) {
	t.Helper()
	c, err := store.MediaConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishMediaConnection(ctx, ConnectionUpdate{ExpectedGeneration: c.Generation, Origin: MediaBinding{Provider: "jellyfin", BaseURL: "http://media:8096", ServerID: "synthetic-server"}}); err != nil {
		t.Fatal(err)
	}
}
