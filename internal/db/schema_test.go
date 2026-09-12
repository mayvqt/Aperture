package db

import (
	"context"
	"fmt"
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
	if err == nil || !strings.Contains(err.Error(), "preserve the database") {
		t.Fatalf("legacy database error = %v", err)
	}
}

func TestInitSchemaMigratesRevisionThreeWithManagedUsers(t *testing.T) {
	store := legacyCleanupStore(t, 3)
	ctx := t.Context()
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	seedTestBinding(t, ctx, store)
	if err := store.SaveManagedUser(ctx, ManagedUser{ExternalUserID: "user-1", Username: "Alice", BindingID: 1}); err != nil {
		t.Fatalf("managed_users was not created during migration: %v", err)
	}
}

func TestInitSchemaMigratesRevisionOneThroughCurrent(t *testing.T) {
	ctx, store := openLegacySchemaStore(t, 1, false)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}

	assertSchemaRevisionAndColumns(t, ctx, store, map[string][]string{
		"registrations": {"template_attempts", "next_template_attempt_at"},
		"webhooks":      {"kind", "role_ids"},
	})
}

func TestInitSchemaMigratesRevisionTwoWebhookColumns(t *testing.T) {
	ctx, store := openLegacySchemaStore(t, 2, true)
	if err := store.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}

	assertSchemaRevisionAndColumns(t, ctx, store, map[string][]string{
		"webhooks": {"kind", "role_ids"},
	})
}

func openLegacySchemaStore(t *testing.T, revision int, registrationRetry bool) (context.Context, *Store) {
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

	registrationColumns := `
	    disable_attempts INTEGER NOT NULL DEFAULT 0,
	    next_disable_attempt_at DATETIME,`
	if registrationRetry {
		registrationColumns += `
	    template_attempts INTEGER NOT NULL DEFAULT 0,
	    next_template_attempt_at DATETIME,`
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE registrations (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    invite_id INTEGER NOT NULL,
	    external_user_id TEXT,
	    username TEXT NOT NULL,
	    status TEXT NOT NULL,
	    template_name TEXT,
	    template_policy_json TEXT,
	    error_message TEXT,
	    user_disable_at DATETIME,
	    user_disabled_at DATETIME,`+registrationColumns+`
	    ip_address TEXT,
	    user_agent TEXT,
	    created_at DATETIME NOT NULL,
	    updated_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE webhooks (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    name TEXT NOT NULL,
	    url_encrypted TEXT NOT NULL,
	    events TEXT NOT NULL,
	    enabled INTEGER NOT NULL DEFAULT 1,
	    created_at DATETIME NOT NULL,
	    updated_at DATETIME NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, revision)); err != nil {
		t.Fatal(err)
	}
	return ctx, store
}

func assertSchemaRevisionAndColumns(t *testing.T, ctx context.Context, store *Store, expected map[string][]string) {
	t.Helper()
	var revision int
	if err := store.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != schemaRevision {
		t.Fatalf("schema revision = %d, want %d", revision, schemaRevision)
	}
	for table, columns := range expected {
		for _, column := range columns {
			exists, err := schemaColumnExists(ctx, store.db, table, column)
			if err != nil {
				t.Fatalf("inspect %s.%s: %v", table, column, err)
			}
			if !exists {
				t.Fatalf("schema is missing %s.%s", table, column)
			}
		}
	}
}
