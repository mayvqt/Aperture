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
	return ctx, store
}
