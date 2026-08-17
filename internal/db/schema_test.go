package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitSchemaCreatesCurrentSchemaAndIsRepeatable(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "aperture.db"), "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.InitSchema(ctx); err != nil {
		t.Fatalf("second initialization failed: %v", err)
	}
	var revision, templates int
	if err := store.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM templates`).Scan(&templates); err != nil {
		t.Fatal(err)
	}
	if revision != schemaRevision || templates != 1 {
		t.Fatalf("schema revision/templates = %d/%d, want %d/1", revision, templates, schemaRevision)
	}
}

func TestInitSchemaRejectsPreV1Database(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "aperture.db"), "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	err = store.InitSchema(context.Background())
	if err == nil || !strings.Contains(err.Error(), "remove the database") {
		t.Fatalf("legacy database error = %v", err)
	}
}

func TestInitSchemaMigratesRevisionThreeWithManagedUsers(t *testing.T) {
	ctx, store := testStore(t)
	if _, err := store.db.ExecContext(ctx, `DROP TABLE managed_users`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `PRAGMA user_version = 3`); err != nil {
		t.Fatal(err)
	}
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveManagedUser(ctx, ManagedUser{ExternalUserID: "user-1", Username: "Alice"}); err != nil {
		t.Fatalf("managed_users was not created during migration: %v", err)
	}
}
