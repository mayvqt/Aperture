package db

import (
	"context"
	"fmt"
)

const schemaRevision = 1

const schema = `
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    secret INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS templates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    policy_json TEXT NOT NULL DEFAULT '{}',
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS invites (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash TEXT NOT NULL UNIQUE,
    token_prefix TEXT,
    token_encrypted TEXT,
    label TEXT,
    template_id INTEGER NOT NULL,
    expires_at DATETIME,
    max_uses INTEGER NOT NULL DEFAULT 1,
    uses INTEGER NOT NULL DEFAULT 0,
    enabled INTEGER NOT NULL DEFAULT 1,
    user_expiry_days INTEGER NOT NULL DEFAULT 0,
    created_by_user_id TEXT,
    last_used_at DATETIME,
    deleted_at DATETIME,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE RESTRICT,
    CHECK (max_uses > 0),
    CHECK (user_expiry_days >= 0),
    CHECK (uses >= 0),
    CHECK (uses <= max_uses)
);

CREATE TABLE IF NOT EXISTS registrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    invite_id INTEGER NOT NULL,
    external_user_id TEXT,
    username TEXT NOT NULL,
    status TEXT NOT NULL,
    template_name TEXT,
    template_policy_json TEXT,
    error_message TEXT,
    user_disable_at DATETIME,
    user_disabled_at DATETIME,
    disable_attempts INTEGER NOT NULL DEFAULT 0,
    next_disable_attempt_at DATETIME,
    ip_address TEXT,
    user_agent TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (invite_id) REFERENCES invites(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    actor_user_id TEXT,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    ip_address TEXT,
    user_agent TEXT,
    metadata_json TEXT NOT NULL DEFAULT '{}',
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    external_user_id TEXT NOT NULL,
    external_username TEXT NOT NULL,
    access_token TEXT NOT NULL,
    device_id TEXT NOT NULL,
    csrf_secret TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_invites_status ON invites(enabled, deleted_at, expires_at, uses, max_uses);
CREATE UNIQUE INDEX IF NOT EXISTS idx_templates_one_default ON templates(is_default) WHERE is_default = 1;
CREATE INDEX IF NOT EXISTS idx_registrations_invite_id ON registrations(invite_id);
CREATE INDEX IF NOT EXISTS idx_registrations_created_at ON registrations(created_at);
CREATE INDEX IF NOT EXISTS idx_registrations_due_disable ON registrations(next_disable_attempt_at) WHERE external_user_id IS NOT NULL AND user_disabled_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_registrations_reconcile ON registrations(status, updated_at);
CREATE INDEX IF NOT EXISTS idx_audit_log_created_at ON audit_log(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`

func (s *Store) InitSchema(ctx context.Context) error {
	var revision int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&revision); err != nil {
		return fmt.Errorf("read schema revision: %w", err)
	}
	if revision > schemaRevision {
		return fmt.Errorf("database schema revision %d is newer than supported revision %d", revision, schemaRevision)
	}
	if revision == 0 {
		var tables int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM sqlite_master
			WHERE type = 'table' AND name NOT LIKE 'sqlite_%'
		`).Scan(&tables); err != nil {
			return fmt.Errorf("inspect database schema: %w", err)
		}
		if tables != 0 {
			return fmt.Errorf("unsupported database schema; remove the database and restart Aperture")
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin schema initialization: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO templates (name, description, policy_json, is_default, created_at, updated_at)
		SELECT 'Default', 'Restricted default template', '{"IsAdministrator":false}', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		WHERE NOT EXISTS (SELECT 1 FROM templates)
	`); err != nil {
		return fmt.Errorf("seed default template: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("record schema revision: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema initialization: %w", err)
	}
	return s.restrictDatabaseFiles()
}
