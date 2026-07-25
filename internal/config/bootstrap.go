package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mayvqt/aperture/internal/security"
)

const encryptionKeyFilename = "encryption.key"

func EnsureEncryptionKey(cfg *Config) error {
	if cfg.EncryptionKey != "" {
		return nil
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	path := filepath.Join(cfg.ConfigDir, encryptionKeyFilename)
	value, err := os.ReadFile(path)
	if err == nil {
		cfg.EncryptionKey = strings.TrimSpace(string(value))
		if len(cfg.EncryptionKey) < 32 {
			return fmt.Errorf("%s contains an invalid encryption key", path)
		}
		return os.Chmod(path, 0o600)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read encryption key: %w", err)
	}
	key, err := security.RandomToken(32)
	if err != nil {
		return fmt.Errorf("generate encryption key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create encryption key: %w", err)
	}
	if _, err := file.WriteString(key + "\n"); err != nil {
		file.Close()
		return fmt.Errorf("write encryption key: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close encryption key: %w", err)
	}
	cfg.EncryptionKey = key
	return nil
}
