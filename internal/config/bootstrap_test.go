package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureEncryptionKeyCreatesAndReusesProtectedKey(t *testing.T) {
	dir := t.TempDir()
	first := Config{ConfigDir: dir}
	if err := EnsureEncryptionKey(&first); err != nil {
		t.Fatal(err)
	}
	if len(first.EncryptionKey) < 32 {
		t.Fatalf("generated key length = %d", len(first.EncryptionKey))
	}
	info, err := os.Stat(filepath.Join(dir, encryptionKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
	}

	second := Config{ConfigDir: dir}
	if err := EnsureEncryptionKey(&second); err != nil {
		t.Fatal(err)
	}
	if second.EncryptionKey != first.EncryptionKey {
		t.Fatal("encryption key changed between startups")
	}
}

func TestEnsureEncryptionKeyPreservesEnvironmentValue(t *testing.T) {
	cfg := Config{ConfigDir: t.TempDir(), EncryptionKey: testSecret}
	if err := EnsureEncryptionKey(&cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg.ConfigDir, encryptionKeyFilename)); !os.IsNotExist(err) {
		t.Fatalf("key file created for managed key: %v", err)
	}
}
