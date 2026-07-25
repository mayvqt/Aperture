package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAppliesSQLiteConnectionPolicyAndRestrictsDatabaseFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aperture.db")
	store, err := Open(path, "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.InitSchema(t.Context()); err != nil {
		t.Fatal(err)
	}

	var foreignKeys, busyTimeout int
	var journalMode string
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 || busyTimeout != 5000 || journalMode != "wal" {
		t.Fatalf("SQLite policy = foreign_keys %d, busy_timeout %d, journal_mode %q", foreignKeys, busyTimeout, journalMode)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("database mode = %o, want 600", got)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err := os.Stat(path + suffix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode = %o, want 600", suffix, got)
		}
	}
}
