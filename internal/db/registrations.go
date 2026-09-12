package db

import (
	"context"
	"database/sql"
	"errors"
)

const registrationColumns = `id, invite_id, external_user_id, username, status, error_message, user_disable_at, user_disabled_at, disable_attempts, next_disable_attempt_at, template_attempts, next_template_attempt_at, created_at, updated_at, cleanup_pending, cleanup_error`

// RecordProvisioningUser is called before a second provider request can begin.
// Password setup is still incomplete and template recovery remains forbidden.
func (s *Store) RecordProvisioningUser(ctx context.Context, id int64, userID string) error {
	if userID == "" {
		return ErrRegistrationTransition
	}
	result, err := s.db.ExecContext(ctx, `UPDATE registrations SET external_user_id = ?, cleanup_pending = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = ?`, userID, id, RegistrationCreatingUser)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

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

// RecordFailedUserCreation preserves a partial account without making it
// eligible for a template-only retry before password setup is complete.
func (s *Store) RecordFailedUserCreation(ctx context.Context, registrationID int64, externalUserID, message string) error {
	if externalUserID == "" {
		return ErrRegistrationTransition
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET external_user_id = ?, status = ?, error_message = NULLIF(?, ''), cleanup_pending = 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, externalUserID, RegistrationFailedCreateUser, message, registrationID, RegistrationCreatingUser)
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
		SET external_user_id = ?, status = ?, error_message = NULL, cleanup_pending = 1, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, externalUserID, RegistrationApplyingTemplate, registrationID, RegistrationCreatingUser)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) CompleteRegistration(ctx context.Context, registrationID int64, status, message string) error {
	if status != RegistrationComplete && status != RegistrationNeedsAttention {
		return ErrRegistrationTransition
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, error_message = NULLIF(?, ''),
		    cleanup_pending = CASE WHEN ? = 'complete' THEN 0 ELSE 1 END,
		    cleanup_error = NULL, next_disable_attempt_at = CASE WHEN ? = 'complete' THEN user_disable_at END, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, status, message, status, status, registrationID, RegistrationApplyingTemplate)
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
		  AND (cleanup_pending = 1 OR (user_disable_at <= CURRENT_TIMESTAMP AND user_disabled_at IS NULL))
		  AND status NOT IN ('reserved','creating_user','applying_template','retrying_template','pending')
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
		SET user_disabled_at = CASE WHEN user_disable_at <= CURRENT_TIMESTAMP THEN CURRENT_TIMESTAMP ELSE user_disabled_at END,
		    status = CASE WHEN user_disable_at <= CURRENT_TIMESTAMP THEN ? ELSE status END,
		    cleanup_pending = 0, cleanup_error = NULL,
		    next_disable_attempt_at = user_disable_at, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND (cleanup_pending = 1 OR (user_disable_at <= CURRENT_TIMESTAMP AND user_disabled_at IS NULL))
	`, RegistrationDisabledExpired, registrationID)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}
func (s *Store) MarkUserDisableFailed(ctx context.Context, registrationID int64, message string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = CASE WHEN user_disable_at <= CURRENT_TIMESTAMP THEN ? ELSE status END,
		    cleanup_error = NULLIF(?, ''),
		    disable_attempts = disable_attempts + 1,
		    next_disable_attempt_at = MIN(datetime(
		        CURRENT_TIMESTAMP,
		        '+' || CASE
		            WHEN disable_attempts >= 9 THEN 360
		            ELSE (1 << disable_attempts)
		        END || ' minutes'
		    ), COALESCE(CASE WHEN user_disable_at > CURRENT_TIMESTAMP THEN user_disable_at END, '9999-12-31 23:59:59')),
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND (cleanup_pending = 1 OR (user_disable_at <= CURRENT_TIMESTAMP AND user_disabled_at IS NULL))
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

func (s *Store) RegistrationUsers(ctx context.Context) ([]Registration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+registrationColumns+`
		FROM registrations
		WHERE external_user_id IS NOT NULL
		  AND id IN (SELECT MAX(id) FROM registrations WHERE external_user_id IS NOT NULL GROUP BY external_user_id)
		ORDER BY username COLLATE NOCASE
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var registrations []Registration
	for rows.Next() {
		registration, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registration)
	}
	return registrations, rows.Err()
}

func (s *Store) Registration(ctx context.Context, id int64) (Registration, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+registrationColumns+` FROM registrations WHERE id = ?`, id)
	registration, err := scanRegistration(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Registration{}, ErrNotFound
	}
	return registration, err
}

func (s *Store) DeleteRegistration(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM registrations WHERE id = ? AND external_user_id IS NULL AND status NOT IN ('reserved','creating_user','applying_template','retrying_template','pending')`, id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrRegistrationTransition
	}
	return nil
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
		&reg.CleanupPending,
		&reg.CleanupError,
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
