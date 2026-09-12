package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var ErrConnectionChanged = errors.New("media-server connection changed")

type MediaBinding struct {
	ID                                int64
	Provider, BaseURL, ServerID, Name string
}

type MediaConnection struct {
	Binding           MediaBinding
	Provider, BaseURL string
	Generation        int64
}

type ConnectionUpdate struct {
	Origin                                 MediaBinding
	ExpectedGeneration                     int64
	Provider, PublicURL, ServerURL, APIKey *string
}

const bindingSchema = `
CREATE TABLE IF NOT EXISTS media_bindings (
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 provider TEXT NOT NULL, base_url TEXT NOT NULL, server_id TEXT NOT NULL,
 name TEXT NOT NULL DEFAULT '',
 UNIQUE(provider, base_url, server_id)
);
CREATE TABLE IF NOT EXISTS media_connection (
 id INTEGER PRIMARY KEY CHECK(id = 1),
 provider TEXT NOT NULL DEFAULT '', base_url TEXT NOT NULL DEFAULT '',
 binding_id INTEGER REFERENCES media_bindings(id),
 generation INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO media_connection(id) VALUES(1);
`

func migrateMediaBindings(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, bindingSchema); err != nil {
		return err
	}
	for _, table := range []string{"invites", "registrations", "sessions"} {
		if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN binding_id INTEGER REFERENCES media_bindings(id)"); err != nil {
			return fmt.Errorf("bind %s to media origin: %w", table, err)
		}
	}
	_, err := tx.ExecContext(ctx, `
	 ALTER TABLE sessions ADD COLUMN connection_generation INTEGER NOT NULL DEFAULT 0;
	 CREATE TABLE managed_users_bound (
	  id INTEGER PRIMARY KEY AUTOINCREMENT,
	  binding_id INTEGER REFERENCES media_bindings(id),
	  external_user_id TEXT NOT NULL, username TEXT NOT NULL,
	  created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
	  UNIQUE(binding_id,external_user_id)
	 );
	 INSERT INTO managed_users_bound(external_user_id,username,created_at,updated_at)
	 SELECT external_user_id,username,created_at,updated_at FROM managed_users;
	 DROP TABLE managed_users;
	 ALTER TABLE managed_users_bound RENAME TO managed_users;
	 CREATE INDEX idx_registrations_binding_user ON registrations(binding_id,external_user_id,id);
	 CREATE INDEX idx_registrations_binding_disable ON registrations(binding_id,next_disable_attempt_at);
	 CREATE INDEX idx_registrations_binding_retry ON registrations(binding_id,status,next_template_attempt_at);
	 CREATE INDEX idx_invites_binding ON invites(binding_id,id);
	`)
	return err
}

type connectionQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readMediaConnection(ctx context.Context, q connectionQuerier) (MediaConnection, error) {
	var c MediaConnection
	err := q.QueryRowContext(ctx, `SELECT c.provider,c.base_url,c.generation,COALESCE(b.id,0),COALESCE(b.provider,''),COALESCE(b.base_url,''),COALESCE(b.server_id,''),COALESCE(b.name,'') FROM media_connection c LEFT JOIN media_bindings b ON b.id=c.binding_id WHERE c.id=1`).Scan(&c.Provider, &c.BaseURL, &c.Generation, &c.Binding.ID, &c.Binding.Provider, &c.Binding.BaseURL, &c.Binding.ServerID, &c.Binding.Name)
	return c, err
}

func (s *Store) MediaConnection(ctx context.Context) (MediaConnection, error) {
	return readMediaConnection(ctx, s.db)
}

func (s *Store) MediaBindings(ctx context.Context) ([]MediaBinding, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,provider,base_url,server_id,name FROM media_bindings ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bindings []MediaBinding
	for rows.Next() {
		var b MediaBinding
		if err := rows.Scan(&b.ID, &b.Provider, &b.BaseURL, &b.ServerID, &b.Name); err != nil {
			return nil, err
		}
		bindings = append(bindings, b)
	}
	return bindings, rows.Err()
}

// Adoption is an explicit administrator action after reviewing the destination.
// Invites and registrations are independent: one old invite can span servers.
func (s *Store) AdoptInvite(ctx context.Context, id, bindingID int64) error {
	if bindingID <= 0 {
		return ErrConnectionChanged
	}
	result, err := s.db.ExecContext(ctx, `UPDATE invites SET binding_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=? AND deleted_at IS NULL AND COALESCE(binding_id,0)<>?`, bindingID, id, bindingID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) AdoptRegistration(ctx context.Context, id, bindingID int64) error {
	if bindingID <= 0 {
		return ErrConnectionChanged
	}
	result, err := s.db.ExecContext(ctx, `UPDATE registrations SET binding_id=?, user_disabled_at=NULL, cleanup_pending=CASE WHEN external_user_id IS NOT NULL AND (status<>'complete' OR user_disable_at<=CURRENT_TIMESTAMP) THEN 1 ELSE 0 END, cleanup_error=NULL, next_disable_attempt_at=NULL, updated_at=CURRENT_TIMESTAMP WHERE id=? AND COALESCE(binding_id,0)<>? AND status NOT IN ('reserved','creating_user','applying_template','retrying_template','pending')`, bindingID, id, bindingID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

// PublishMediaConnection commits browser-managed settings, verified origin and
// session generation together. Legacy ownership is deliberately left unknown.
func (s *Store) PublishMediaConnection(ctx context.Context, u ConnectionUpdate) (MediaConnection, error) {
	var encryptedKey string
	if u.APIKey != nil {
		var err error
		encryptedKey, _, err = s.settingValue(*u.APIKey, true)
		if err != nil {
			return MediaConnection{}, err
		}
	}
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	defer func() { s.settingsCached = false }()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MediaConnection{}, err
	}
	defer tx.Rollback()
	current, err := readMediaConnection(ctx, tx)
	if err != nil {
		return MediaConnection{}, err
	}
	if current.Generation != u.ExpectedGeneration {
		return MediaConnection{}, ErrConnectionChanged
	}
	for _, setting := range []struct {
		key    string
		value  *string
		secret int
	}{
		{"media_provider", u.Provider, 0}, {"public_url", u.PublicURL, 0}, {"server_url", u.ServerURL, 0},
	} {
		if setting.value != nil {
			if err := upsertSetting(ctx, tx, setting.key, *setting.value, setting.secret); err != nil {
				return MediaConnection{}, err
			}
		}
	}
	if u.APIKey != nil {
		if err := upsertSetting(ctx, tx, "api_key", encryptedKey, 1); err != nil {
			return MediaConnection{}, err
		}
	}
	binding := u.Origin
	binding.ID = 0
	if binding.ServerID != "" {
		if binding.Provider == "" || binding.BaseURL == "" {
			return MediaConnection{}, errors.New("verified media origin is incomplete")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO media_bindings(provider,base_url,server_id,name) VALUES(?,?,?,?) ON CONFLICT(provider,base_url,server_id) DO UPDATE SET name=excluded.name`, binding.Provider, binding.BaseURL, binding.ServerID, binding.Name); err != nil {
			return MediaConnection{}, err
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM media_bindings WHERE provider=? AND base_url=? AND server_id=?`, binding.Provider, binding.BaseURL, binding.ServerID).Scan(&binding.ID); err != nil {
			return MediaConnection{}, err
		}
	}
	generation := current.Generation
	if current.Provider != binding.Provider || current.BaseURL != binding.BaseURL || current.Binding.ID != binding.ID {
		generation++
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions`); err != nil {
			return MediaConnection{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_connection SET provider=?,base_url=?,binding_id=NULLIF(?,0),generation=? WHERE id=1`, binding.Provider, binding.BaseURL, binding.ID, generation); err != nil {
		return MediaConnection{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaConnection{}, err
	}
	s.settingsCached = false
	return MediaConnection{Binding: binding, Provider: binding.Provider, BaseURL: binding.BaseURL, Generation: generation}, nil
}
