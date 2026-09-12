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
	BindingID   int64
	Generation  int64
}

type SessionInput struct {
	UserID, Username, AccessToken, DeviceID string
	TTL                                     time.Duration
	BindingID, Generation                   int64
}

func (s *Store) CreateSession(ctx context.Context, input SessionInput) (string, string, error) {
	if input.BindingID <= 0 {
		return "", "", ErrConnectionChanged
	}
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
	encryptedAccessToken, err := s.encryptor.EncryptString(input.AccessToken)
	if err != nil {
		return "", "", err
	}
	encryptedCSRF, err := s.encryptor.EncryptString(csrf)
	if err != nil {
		return "", "", err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, external_user_id, external_username, access_token, device_id, csrf_secret, expires_at, binding_id, connection_generation, created_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP
		FROM media_connection WHERE id=1 AND binding_id=? AND generation=?
	`, sessionHash, input.UserID, input.Username, encryptedAccessToken, input.DeviceID, encryptedCSRF, now.Add(input.TTL), input.BindingID, input.Generation, input.BindingID, input.Generation)
	if err != nil {
		return "", "", err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return "", "", err
	} else if changed != 1 {
		return "", "", ErrConnectionChanged
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
		SELECT id, external_user_id, external_username, access_token, device_id, csrf_secret, expires_at, COALESCE(binding_id,0),connection_generation
		FROM sessions
		WHERE id = ? AND expires_at > CURRENT_TIMESTAMP
	`, sessionHash).Scan(&session.ID, &session.UserID, &session.Username, &session.AccessToken, &session.DeviceID, &session.CSRFSecret, &session.ExpiresAt, &session.BindingID, &session.Generation)
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
