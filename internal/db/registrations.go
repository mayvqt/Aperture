package db

import (
	"context"
	"database/sql"
)

const registrationColumns = `id, invite_id, external_user_id, username, status, error_message, user_disable_at, user_disabled_at, disable_attempts, next_disable_attempt_at, template_attempts, next_template_attempt_at, created_at, updated_at`

func (s *Store) FailUserCreation(ctx context.Context, registrationID int64, message string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, error_message = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, RegistrationFailedCreateUser, message, registrationID, RegistrationCreatingUser)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) RecordCreatedUser(ctx context.Context, registrationID int64, externalUserID string) error {
	if externalUserID == "" {
		return ErrRegistrationTransition
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET external_user_id = ?, status = ?, error_message = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, externalUserID, RegistrationApplyingTemplate, registrationID, RegistrationCreatingUser)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) CompleteRegistration(ctx context.Context, registrationID int64, status, message string, disableAt sql.NullTime) error {
	if status != RegistrationComplete && status != RegistrationNeedsAttention {
		return ErrRegistrationTransition
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, error_message = NULLIF(?, ''),
		    user_disable_at = ?, next_disable_attempt_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, status, message, disableAt, disableAt, registrationID, RegistrationApplyingTemplate)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) DueUserDisables(ctx context.Context, limit int) ([]Registration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+registrationColumns+`
		FROM registrations
		WHERE external_user_id IS NOT NULL
		  AND user_disable_at IS NOT NULL
		  AND user_disable_at <= CURRENT_TIMESTAMP
		  AND user_disabled_at IS NULL
		  AND (next_disable_attempt_at IS NULL OR next_disable_attempt_at <= CURRENT_TIMESTAMP)
		ORDER BY next_disable_attempt_at ASC, user_disable_at ASC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regs []Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}
	return regs, rows.Err()
}
func (s *Store) MarkUserDisabled(ctx context.Context, registrationID int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET user_disabled_at = CURRENT_TIMESTAMP, status = ?,
		    error_message = NULL, next_disable_attempt_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND user_disable_at IS NOT NULL
		  AND user_disabled_at IS NULL
	`, RegistrationDisabledExpired, registrationID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}
func (s *Store) MarkUserDisableFailed(ctx context.Context, registrationID int64, message string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?,
		    error_message = NULLIF(?, ''),
		    disable_attempts = disable_attempts + 1,
		    next_disable_attempt_at = datetime(
		        CURRENT_TIMESTAMP,
		        '+' || CASE
		            WHEN disable_attempts >= 9 THEN 360
		            ELSE (1 << disable_attempts)
		        END || ' minutes'
		    ),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND user_disable_at IS NOT NULL
		  AND user_disabled_at IS NULL
	`, RegistrationDisableFailed, message, registrationID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}
func (s *Store) Audit(ctx context.Context, actor, action, targetType, targetID, ip, ua, metadata string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_user_id, action, target_type, target_id, ip_address, user_agent, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	`, actor, action, targetType, targetID, ip, ua, metadata)
	return err
}
func (s *Store) RecentRegistrations(ctx context.Context, limit int) ([]Registration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+registrationColumns+`
		FROM registrations
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var regs []Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}
	return regs, rows.Err()
}

func (s *Store) DashboardCounts(ctx context.Context) (DashboardCounts, error) {
	var counts DashboardCounts
	err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*)
			 FROM invites
			 WHERE enabled = 1
			   AND deleted_at IS NULL
			   AND uses < max_uses
			   AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)),
			(SELECT COUNT(*) FROM templates),
			(SELECT COUNT(*)
			 FROM registrations
			 WHERE status IN (?, ?, ?, ?)),
			(SELECT COUNT(*)
			 FROM registrations
			 WHERE external_user_id IS NOT NULL
			   AND user_disable_at IS NOT NULL
			   AND user_disabled_at IS NULL)
	`,
		RegistrationNeedsAttention,
		RegistrationFailedCreateUser,
		RegistrationFailedApplyTemplate,
		RegistrationDisableFailed,
	).Scan(&counts.ActiveInvites, &counts.Templates, &counts.NeedsAttention, &counts.ScheduledUserDisables)
	return counts, err
}

func (s *Store) LatestInviteActivity(ctx context.Context) (map[int64]InviteActivity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.invite_id, r.username, r.status, r.created_at
		FROM registrations r
		JOIN (
			SELECT invite_id, MAX(id) AS id
			FROM registrations
			GROUP BY invite_id
		) latest ON latest.id = r.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	activity := map[int64]InviteActivity{}
	for rows.Next() {
		var item InviteActivity
		if err := rows.Scan(&item.InviteID, &item.Username, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		activity[item.InviteID] = item
	}
	return activity, rows.Err()
}

func scanRegistration(scanner rowScanner) (Registration, error) {
	var reg Registration
	err := scanner.Scan(registrationScanDestinations(&reg)...)
	return reg, err
}

func registrationScanDestinations(reg *Registration) []any {
	return []any{
		&reg.ID,
		&reg.InviteID,
		&reg.ExternalUserID,
		&reg.Username,
		&reg.Status,
		&reg.ErrorMessage,
		&reg.UserDisableAt,
		&reg.UserDisabledAt,
		&reg.DisableAttempts,
		&reg.NextDisableAttemptAt,
		&reg.TemplateAttempts,
		&reg.NextTemplateAttemptAt,
		&reg.CreatedAt,
		&reg.UpdatedAt,
	}
}

func requireSingleTransition(result sql.Result) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrRegistrationTransition
	}
	return nil
}
