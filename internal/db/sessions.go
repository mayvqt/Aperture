package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/mayvqt/aperture/internal/security"
)

type Session struct {
	ID          string
	UserID      string
	Username    string
	AccessToken string
	DeviceID    string
	CSRFSecret  string
	ExpiresAt   time.Time
}

func (s *Store) CreateSession(ctx context.Context, userID, username, accessToken, deviceID string, ttl time.Duration) (string, string, error) {
	now := time.Now().UTC()
	sessionID, err := security.RandomToken(32)
	if err != nil {
		return "", "", err
	}
	csrf, err := security.RandomToken(32)
	if err != nil {
		return "", "", err
	}
	sessionHash := hashSessionID(sessionID)
	encryptedAccessToken, err := s.encryptor.EncryptString(accessToken)
	if err != nil {
		return "", "", err
	}
	encryptedCSRF, err := s.encryptor.EncryptString(csrf)
	if err != nil {
		return "", "", err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, external_user_id, external_username, access_token, device_id, csrf_secret, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, sessionHash, userID, username, encryptedAccessToken, deviceID, encryptedCSRF, now.Add(ttl))
	if err != nil {
		return "", "", err
	}
	_, _ = s.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE id IN (
			SELECT id FROM sessions
			WHERE expires_at <= ?
			LIMIT 100
		)
	`, now)
	return sessionID, csrf, nil
}

func (s *Store) Session(ctx context.Context, id string) (Session, error) {
	sessionHash := hashSessionID(id)
	var session Session
	err := s.db.QueryRowContext(ctx, `
		SELECT id, external_user_id, external_username, access_token, device_id, csrf_secret, expires_at
		FROM sessions
		WHERE id = ? AND expires_at > CURRENT_TIMESTAMP
	`, sessionHash).Scan(&session.ID, &session.UserID, &session.Username, &session.AccessToken, &session.DeviceID, &session.CSRFSecret, &session.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	session.AccessToken, err = s.encryptor.DecryptString(session.AccessToken)
	if err != nil {
		return Session{}, err
	}
	session.CSRFSecret, err = s.encryptor.DecryptString(session.CSRFSecret)
	if err != nil {
		return Session{}, err
	}
	session.ID = id
	return session, nil
}
func (s *Store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, hashSessionID(id))
	return err
}

func hashSessionID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
