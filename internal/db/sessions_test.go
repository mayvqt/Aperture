package db

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSessionLifecycleEncryptsTokenAndExpires(t *testing.T) {
	ctx, store := testStore(t)
	sessionID, csrf, err := store.CreateSession(ctx, SessionInput{UserID: "jf-user", Username: "admin", AccessToken: "access-token", DeviceID: "device-id", TTL: time.Hour, BindingID: 1, Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if csrf == "" {
		t.Fatal("expected CSRF secret")
	}
	var storedID string
	if err := store.db.QueryRowContext(ctx, `SELECT id FROM sessions`).Scan(&storedID); err != nil {
		t.Fatal(err)
	}
	if storedID == sessionID {
		t.Fatal("session cookie token stored in plaintext")
	}
	var rawToken string
	if err := store.db.QueryRowContext(ctx, `SELECT access_token FROM sessions WHERE id = ?`, storedID).Scan(&rawToken); err != nil {
		t.Fatal(err)
	}
	if rawToken == "access-token" {
		t.Fatal("access token stored in plaintext")
	}
	session, err := store.Session(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.AccessToken != "access-token" || session.DeviceID != "device-id" || session.CSRFSecret != csrf {
		t.Fatalf("session = %#v, csrf %q; want decrypted token and csrf", session, csrf)
	}
	if err := store.DeleteSession(ctx, sessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Session(ctx, sessionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Session after delete error = %v, want ErrNotFound", err)
	}
}

func TestCreateSessionRemovesExpiredSessions(t *testing.T) {
	ctx, store := testStore(t)
	expiredID, _, err := store.CreateSession(ctx, SessionInput{UserID: "old-user", Username: "old-admin", AccessToken: "old-token", DeviceID: "old-device", TTL: time.Hour, BindingID: 1, Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE sessions SET expires_at = ? WHERE id = ?`, time.Now().Add(-time.Hour).UTC(), hashSessionID(expiredID)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Session(ctx, expiredID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired Session error = %v, want ErrNotFound", err)
	}

	if _, _, err := store.CreateSession(ctx, SessionInput{UserID: "new-user", Username: "new-admin", AccessToken: "new-token", DeviceID: "new-device", TTL: time.Hour, BindingID: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE external_user_id = 'old-user'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired session count = %d, want 0", count)
	}
}

func TestSessionSchemaOmitsUnusedLastSeen(t *testing.T) {
	ctx, store := testStore(t)
	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(sessions)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "last_seen_at" {
			t.Fatal("sessions schema still contains unused last_seen_at column")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredSessionCleanupIsBoundedAndContinues(t *testing.T) {
	ctx, store := testStore(t)
	expiredAt := time.Now().Add(-time.Hour).UTC()
	for i := range 101 {
		_, err := store.db.ExecContext(ctx, `
			INSERT INTO sessions (id, external_user_id, external_username, access_token, device_id, csrf_secret, expires_at, created_at)
			VALUES (?, 'old-user', 'old-admin', '', 'old-device', 'unused', ?, CURRENT_TIMESTAMP)
		`, fmt.Sprintf("expired-%d", i), expiredAt)
		if err != nil {
			t.Fatal(err)
		}
	}

	remaining := func() int {
		t.Helper()
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE expires_at <= ?`, time.Now().UTC()).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	if _, _, err := store.CreateSession(ctx, SessionInput{UserID: "new-user", Username: "new-admin", AccessToken: "new-token", DeviceID: "new-device-1", TTL: time.Hour, BindingID: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if got := remaining(); got != 1 {
		t.Fatalf("expired sessions after first bounded cleanup = %d, want 1", got)
	}
	if _, _, err := store.CreateSession(ctx, SessionInput{UserID: "new-user", Username: "new-admin", AccessToken: "new-token", DeviceID: "new-device-2", TTL: time.Hour, BindingID: 1, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if got := remaining(); got != 0 {
		t.Fatalf("expired sessions after second cleanup = %d, want 0", got)
	}
}
