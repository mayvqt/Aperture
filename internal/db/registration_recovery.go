package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *Store) ReconcileStaleRegistrations(ctx context.Context, staleBefore time.Time, limit int) (ReconciliationResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationResult{}, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, invite_id, status
		FROM registrations
		WHERE status IN (?, ?, ?, ?, ?)
		  AND updated_at <= ?
		ORDER BY updated_at ASC
		LIMIT ?
	`, RegistrationReserved, RegistrationCreatingUser, RegistrationApplyingTemplate, RegistrationRetryingTemplate, RegistrationLegacyPending, staleBefore.UTC(), limit)
	if err != nil {
		return ReconciliationResult{}, err
	}
	type staleRegistration struct {
		id       int64
		inviteID int64
		status   string
	}
	var stale []staleRegistration
	for rows.Next() {
		var reg staleRegistration
		if err := rows.Scan(&reg.id, &reg.inviteID, &reg.status); err != nil {
			rows.Close()
			return ReconciliationResult{}, err
		}
		stale = append(stale, reg)
	}
	if err := rows.Close(); err != nil {
		return ReconciliationResult{}, err
	}
	if err := rows.Err(); err != nil {
		return ReconciliationResult{}, err
	}

	var result ReconciliationResult
	for _, reg := range stale {
		if reg.status == RegistrationReserved {
			changed, err := tx.ExecContext(ctx, `
				UPDATE registrations
				SET status = ?,
				    error_message = 'Registration stopped before media-server user creation began.',
				    updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND status = ?
			`, RegistrationAbandonedBeforeUser, reg.id, RegistrationReserved)
			if err != nil {
				return ReconciliationResult{}, err
			}
			if err := requireSingleTransition(changed); err != nil {
				return ReconciliationResult{}, err
			}
			inviteUpdate, err := tx.ExecContext(ctx, `
				UPDATE invites
				SET uses = uses - 1, updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND uses > 0
			`, reg.inviteID)
			if err != nil {
				return ReconciliationResult{}, err
			}
			if err := requireSingleTransition(inviteUpdate); err != nil {
				return ReconciliationResult{}, err
			}
			result.ReleasedReservations++
			continue
		}

		message := "Registration was interrupted after media-server user creation may have begun."
		if reg.status == RegistrationApplyingTemplate || reg.status == RegistrationRetryingTemplate {
			message = "Registration was interrupted after media-server user creation and before template application completed."
		}
		changed, err := tx.ExecContext(ctx, `
			UPDATE registrations
			SET status = ?,
			    error_message = ?,
			    updated_at = CURRENT_TIMESTAMP
			WHERE id = ? AND status = ?
		`, RegistrationNeedsAttention, message, reg.id, reg.status)
		if err != nil {
			return ReconciliationResult{}, err
		}
		if err := requireSingleTransition(changed); err != nil {
			return ReconciliationResult{}, err
		}
		result.FlaggedAmbiguous++
	}
	return result, tx.Commit()
}

func (s *Store) ClaimTemplateRecovery(ctx context.Context, registrationID int64) (RegistrationRecovery, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RegistrationRecovery{}, err
	}
	defer tx.Rollback()

	var recovery RegistrationRecovery
	destinations := append(
		registrationScanDestinations(&recovery.Registration),
		&recovery.Template.Name,
		&recovery.Template.PolicyJSON,
		&recovery.UserExpiryDays,
	)
	err = tx.QueryRowContext(ctx, `
		SELECT r.id, r.invite_id, r.external_user_id, r.username, r.status, r.error_message,
		       r.user_disable_at, r.user_disabled_at,
		       r.disable_attempts, r.next_disable_attempt_at, r.created_at, r.updated_at,
		       COALESCE(r.template_name, ''),
		       COALESCE(r.template_policy_json, ''),
		       i.user_expiry_days
		FROM registrations r
		JOIN invites i ON i.id = r.invite_id
		WHERE r.id = ?
	`, registrationID).Scan(destinations...)
	if errors.Is(err, sql.ErrNoRows) {
		return RegistrationRecovery{}, ErrNotFound
	}
	if err != nil {
		return RegistrationRecovery{}, err
	}
	if !recovery.Registration.ExternalUserID.Valid ||
		!CanRetryRegistrationTemplate(recovery.Registration.Status) {
		return RegistrationRecovery{}, ErrRegistrationTransition
	}
	claimed, err := tx.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND external_user_id IS NOT NULL AND status IN (?, ?)
	`, RegistrationRetryingTemplate, registrationID, RegistrationNeedsAttention, RegistrationFailedApplyTemplate)
	if err != nil {
		return RegistrationRecovery{}, err
	}
	if err := requireSingleTransition(claimed); err != nil {
		return RegistrationRecovery{}, err
	}
	recovery.Registration.Status = RegistrationRetryingTemplate
	if err := tx.Commit(); err != nil {
		return RegistrationRecovery{}, err
	}
	return recovery, nil
}

func (s *Store) RecordTemplateRetryFailure(ctx context.Context, registrationID int64, message string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, error_message = NULLIF(?, ''), updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND status = ?
	`, RegistrationNeedsAttention, message, registrationID, RegistrationRetryingTemplate)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) CompleteTemplateRecovery(ctx context.Context, registrationID int64, disableAt sql.NullTime) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, error_message = NULL,
		    user_disable_at = ?, next_disable_attempt_at = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
		  AND external_user_id IS NOT NULL
		  AND status = ?
	`, RegistrationComplete, disableAt, disableAt, registrationID, RegistrationRetryingTemplate)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}
