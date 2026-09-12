package db

import (
	"context"
	"database/sql"
	"errors"
)

const inviteColumns = `i.id, i.token_hash, COALESCE(i.token_prefix, ''), COALESCE(i.token_encrypted, ''), COALESCE(i.label, ''), i.template_id, t.name,
		       i.expires_at, i.max_uses, i.uses, i.enabled, i.user_expiry_days, i.last_used_at, i.deleted_at, i.created_at, i.updated_at, COALESCE(i.binding_id,0)`

func (s *Store) CreateInvite(ctx context.Context, invite Invite) (int64, error) {
	if invite.BindingID <= 0 {
		return 0, ErrConnectionChanged
	}
	encryptedToken := ""
	if invite.Token != "" {
		var err error
		encryptedToken, err = s.encryptor.EncryptString(invite.Token)
		if err != nil {
			return 0, err
		}
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO invites (token_hash, token_prefix, token_encrypted, label, template_id, expires_at, max_uses, uses, enabled, user_expiry_days, created_by_user_id, binding_id, created_at, updated_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, 0, 1, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, invite.TokenHash, invite.TokenPrefix, encryptedToken, invite.Label, invite.TemplateID, invite.ExpiresAt, invite.MaxUses, invite.UserExpiryDays, invite.CreatedByUserID, invite.BindingID)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
func (s *Store) InvitePreview(ctx context.Context, limit int) ([]Invite, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(label, ''), expires_at, max_uses, uses, enabled, user_expiry_days, deleted_at, COALESCE(binding_id,0)
		FROM invites
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var invites []Invite
	for rows.Next() {
		var invite Invite
		if err := rows.Scan(&invite.ID, &invite.Label, &invite.ExpiresAt, &invite.MaxUses, &invite.Uses, &invite.Enabled, &invite.UserExpiryDays, &invite.DeletedAt, &invite.BindingID); err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}
	return invites, rows.Err()
}

func (s *Store) InvitePreset(ctx context.Context, id int64) (Invite, error) {
	var invite Invite
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(i.label, ''), i.template_id, t.name, i.expires_at, i.max_uses, i.user_expiry_days
		FROM invites i
		JOIN templates t ON t.id = i.template_id
		WHERE i.id = ? AND i.deleted_at IS NULL
	`, id).Scan(&invite.Label, &invite.TemplateID, &invite.Template, &invite.ExpiresAt, &invite.MaxUses, &invite.UserExpiryDays)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	return invite, nil
}

func (s *Store) InviteByHash(ctx context.Context, hash string, bindingID int64) (Invite, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+inviteColumns+`
		FROM invites i
		JOIN templates t ON t.id = i.template_id
		WHERE i.token_hash = ? AND i.binding_id = ?
	`, hash, bindingID)
	invite, err := s.scanInvite(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	return invite, nil
}
func (s *Store) ReserveInviteUse(ctx context.Context, inviteID, bindingID int64, ip, ua, username string) (int64, Template, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, Template{}, err
	}
	defer tx.Rollback()

	var template Template
	err = tx.QueryRowContext(ctx, `
		SELECT t.id, t.name, COALESCE(t.description, ''), t.policy_json, t.is_default, t.created_at, t.updated_at
		FROM invites i
		JOIN templates t ON t.id = i.template_id
		WHERE i.id = ? AND i.binding_id = ?
	`, inviteID, bindingID).Scan(&template.ID, &template.Name, &template.Description, &template.PolicyJSON, &template.IsDefault, &template.CreatedAt, &template.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Template{}, ErrInviteUnavailable
	}
	if err != nil {
		return 0, Template{}, err
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE invites
		SET uses = uses + 1, last_used_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND binding_id = ?
		  AND enabled = 1
		  AND deleted_at IS NULL
		  AND uses < max_uses
		  AND (expires_at IS NULL OR expires_at > CURRENT_TIMESTAMP)
	`, inviteID, bindingID)
	if err != nil {
		return 0, Template{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return 0, Template{}, err
	}
	if changed != 1 {
		return 0, Template{}, ErrInviteUnavailable
	}
	result, err = tx.ExecContext(ctx, `
		INSERT INTO registrations (
			invite_id, username, status, template_name, template_policy_json,
			ip_address, user_agent, user_disable_at, binding_id, created_at, updated_at
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, CASE WHEN user_expiry_days > 0 THEN datetime(CURRENT_TIMESTAMP, '+' || user_expiry_days || ' days') END, binding_id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		FROM invites WHERE id = ?
	`, inviteID, username, RegistrationReserved, template.Name, template.PolicyJSON, ip, ua, inviteID)
	if err != nil {
		return 0, Template{}, err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, Template{}, err
	}
	if inserted != 1 {
		return 0, Template{}, ErrInviteUnavailable
	}
	regID, err := result.LastInsertId()
	if err != nil {
		return 0, Template{}, err
	}
	if err := tx.Commit(); err != nil {
		return 0, Template{}, err
	}
	return regID, template, nil
}

func (s *Store) BeginUserCreation(ctx context.Context, registrationID int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE registrations
		SET status = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status = ?
	`, RegistrationCreatingUser, registrationID, RegistrationReserved)
	if err != nil {
		return err
	}
	return requireSingleTransition(result)
}

func (s *Store) SetInviteEnabled(ctx context.Context, id, bindingID int64, enabled bool) error {
	value := 0
	if enabled {
		value = 1
	}
	result, err := s.db.ExecContext(ctx, `UPDATE invites SET enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL AND (? = 0 OR binding_id = ?)`, value, id, value, bindingID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) DeleteInvite(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE invites SET enabled = 0, deleted_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleted_at IS NULL`, id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) scanInvite(scanner rowScanner) (Invite, error) {
	var invite Invite
	var encryptedToken string
	err := scanner.Scan(&invite.ID, &invite.TokenHash, &invite.TokenPrefix, &encryptedToken, &invite.Label, &invite.TemplateID, &invite.Template, &invite.ExpiresAt, &invite.MaxUses, &invite.Uses, &invite.Enabled, &invite.UserExpiryDays, &invite.LastUsedAt, &invite.DeletedAt, &invite.CreatedAt, &invite.UpdatedAt, &invite.BindingID)
	if err != nil {
		return Invite{}, err
	}
	if encryptedToken != "" {
		token, err := s.encryptor.DecryptString(encryptedToken)
		if err != nil {
			return Invite{}, err
		}
		invite.Token = token
	}
	return invite, nil
}
