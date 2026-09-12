package db

import (
	"context"
	"database/sql"
	"errors"

	"github.com/mayvqt/aperture/internal/security"
)

func (s *Store) EnsureRuntimeSecrets(ctx context.Context, sessionSecret, inviteSecret string) error {
	if err := s.ensureRuntimeSecret(ctx, "session_secret", sessionSecret); err != nil {
		return err
	}
	return s.ensureRuntimeSecret(ctx, "invite_secret", inviteSecret)
}

func (s *Store) ensureRuntimeSecret(ctx context.Context, key, configured string) error {
	if configured != "" {
		return s.SetSetting(ctx, key, configured, true)
	}
	if _, err := s.Setting(ctx, key); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	generated, err := security.RandomToken(32)
	if err != nil {
		return err
	}
	return s.SetSetting(ctx, key, generated, true)
}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	s.settingsMu.RLock()
	if s.settingsCached {
		settings := s.settingsCache
		s.settingsMu.RUnlock()
		return settings, nil
	}
	s.settingsMu.RUnlock()

	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	if s.settingsCached {
		return s.settingsCache, nil
	}
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	s.settingsCache = settings
	s.settingsCached = true
	return settings, nil
}

func (s *Store) loadSettings(ctx context.Context) (Settings, error) {
	values := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `
		SELECT key, value, secret
		FROM settings
		WHERE key IN ('media_provider', 'public_url', 'server_url', 'api_key', 'session_secret', 'invite_secret')
	`)
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		var secret bool
		if err := rows.Scan(&key, &value, &secret); err != nil {
			return Settings{}, err
		}
		if secret {
			value, err = s.encryptor.DecryptString(value)
			if err != nil {
				return Settings{}, err
			}
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return Settings{}, err
	}
	return Settings{
		Provider:      values["media_provider"],
		PublicURL:     values["public_url"],
		ServerURL:     values["server_url"],
		APIKey:        values["api_key"],
		SessionSecret: values["session_secret"],
		InviteSecret:  values["invite_secret"],
	}, nil
}
func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var value string
	var secret bool
	err := s.db.QueryRowContext(ctx, `SELECT value, secret FROM settings WHERE key = ?`, key).Scan(&value, &secret)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if secret {
		return s.encryptor.DecryptString(value)
	}
	return value, nil
}
func (s *Store) SetSetting(ctx context.Context, key, value string, secret bool) error {
	value, secretInt, err := s.settingValue(value, secret)
	if err != nil {
		return err
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	err = upsertSetting(ctx, s.db, key, value, secretInt)
	if err == nil {
		s.settingsCached = false
	}
	return err
}

func (s *Store) settingValue(value string, secret bool) (string, int, error) {
	if !secret {
		return value, 0, nil
	}
	encrypted, err := s.encryptor.EncryptString(value)
	if err != nil {
		return "", 0, err
	}
	return encrypted, 1, nil
}

type settingExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func upsertSetting(ctx context.Context, execer settingExecer, key, value string, secret int) error {
	_, err := execer.ExecContext(ctx, `
		INSERT INTO settings (key, value, secret, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, secret = excluded.secret, updated_at = CURRENT_TIMESTAMP
	`, key, value, secret)
	return err
}
