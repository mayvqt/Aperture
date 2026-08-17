package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"

	"github.com/mayvqt/aperture/internal/security"

	_ "modernc.org/sqlite"
)

type Store struct {
	db             *sql.DB
	path           string
	encryptor      *security.Encryptor
	settingsMu     sync.RWMutex
	settingsCache  Settings
	settingsCached bool
}

func Open(path string, encryptionKey string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve database path: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create database directory %s: %w", dir, err)
	}
	if err := ensureWritableDir(dir); err != nil {
		return nil, err
	}
	encryptor, err := security.NewEncryptor(encryptionKey)
	if err != nil {
		return nil, err
	}
	dsn := sqliteDSN(path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	store := &Store{db: db, path: path, encryptor: encryptor}
	if err := store.configure(); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.restrictDatabaseFiles(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func sqliteDSN(path string) string {
	urlPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" && urlPath[0] != '/' {
		urlPath = "/" + urlPath
	}
	u := &url.URL{Scheme: "file", Path: urlPath}
	query := u.Query()
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	u.RawQuery = query.Encode()
	return u.String()
}

func ensureWritableDir(dir string) error {
	f, err := os.CreateTemp(dir, ".aperture-write-test-*")
	if err != nil {
		return fmt.Errorf("database directory %s is not writable: %w", dir, err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("database directory %s write test failed: %w", dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("database directory %s cleanup failed: %w", dir, err)
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) configure() error {
	var foreignKeys, busyTimeout int
	var journalMode string
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read SQLite foreign_keys setting: %w", err)
	}
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		return fmt.Errorf("read SQLite busy_timeout setting: %w", err)
	}
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		return fmt.Errorf("read SQLite journal_mode setting: %w", err)
	}
	if foreignKeys != 1 || busyTimeout != 5000 || journalMode != "wal" {
		return fmt.Errorf("SQLite connection settings were not applied")
	}
	return nil
}

func (s *Store) restrictDatabaseFiles() error {
	for _, path := range []string{s.path, s.path + "-wal", s.path + "-shm"} {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("restrict database file %s: %w", path, err)
		}
	}
	return nil
}
