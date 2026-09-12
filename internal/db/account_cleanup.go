package db

import (
	"context"
	"database/sql"
	"fmt"
)

func migrateAccountCleanup(ctx context.Context, tx *sql.Tx) error {
	for _, column := range []struct{ name, definition string }{
		{"cleanup_pending", "INTEGER NOT NULL DEFAULT 0"}, {"cleanup_error", "TEXT"},
	} {
		exists, err := schemaColumnExists(ctx, tx, "registrations", column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.ExecContext(ctx, "ALTER TABLE registrations ADD COLUMN "+column.name+" "+column.definition); err != nil {
				return fmt.Errorf("add account cleanup state: %w", err)
			}
		}
	}
	// Existing successful deadlines are immutable. Failed legacy registrations
	// lacked one; use the original creation time and the retained invite policy.
	_, err := tx.ExecContext(ctx, `
		UPDATE registrations SET user_disable_at = datetime(created_at, '+' ||
		 (SELECT user_expiry_days FROM invites WHERE id = registrations.invite_id) || ' days')
		WHERE user_disable_at IS NULL AND EXISTS
		 (SELECT 1 FROM invites WHERE id = registrations.invite_id AND user_expiry_days > 0);
		UPDATE registrations SET cleanup_pending = 1, next_disable_attempt_at = NULL
		WHERE external_user_id IS NOT NULL AND status IN
		 ('creating_user','applying_template','retrying_template','needs_attention','failed_create_user','failed_apply_template','pending');
		CREATE INDEX IF NOT EXISTS idx_registrations_cleanup ON registrations(next_disable_attempt_at) WHERE cleanup_pending = 1;
	`)
	return err
}

// RequireAccountCleanup preserves the known account ID even when the preceding
// result write failed or timed out. Password-incomplete creation cannot recover
// through a template-only retry.
func (s *Store) RequireAccountCleanup(ctx context.Context, id int64, userID string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations SET external_user_id = COALESCE(external_user_id, NULLIF(?, '')),
		 cleanup_pending = 1, next_disable_attempt_at = NULL,
		 status = CASE WHEN status = 'creating_user' THEN 'failed_create_user'
		               WHEN status IN ('applying_template','retrying_template') THEN 'needs_attention' ELSE status END,
		 updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status NOT IN ('complete','disabled_expired')
		 AND (external_user_id IS NULL OR external_user_id = ?)
	`, userID, id, userID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}
