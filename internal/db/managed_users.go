package db

import (
	"context"
)

func (s *Store) ListManagedUsers(ctx context.Context, bindingID int64) ([]ManagedUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,external_user_id, username, created_at, updated_at,COALESCE(binding_id,0) FROM managed_users WHERE binding_id = ? ORDER BY username COLLATE NOCASE`, bindingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []ManagedUser
	for rows.Next() {
		var user ManagedUser
		if err := rows.Scan(&user.ID, &user.ExternalUserID, &user.Username, &user.CreatedAt, &user.UpdatedAt, &user.BindingID); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SaveManagedUser(ctx context.Context, user ManagedUser) error {
	if user.BindingID <= 0 {
		return ErrConnectionChanged
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO managed_users (external_user_id, username, binding_id, created_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(binding_id,external_user_id) DO UPDATE SET username = excluded.username, updated_at = CURRENT_TIMESTAMP
	`, user.ExternalUserID, user.Username, user.BindingID)
	return err
}

func (s *Store) DeleteUserRecords(ctx context.Context, bindingID int64, externalUserID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM registrations WHERE external_user_id = ? AND binding_id = ?`, externalUserID, bindingID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_users WHERE external_user_id = ? AND binding_id = ?`, externalUserID, bindingID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UserDeletionRegistrations(ctx context.Context, bindingID int64, id string) ([]Registration, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+registrationColumns+` FROM registrations WHERE external_user_id = ? AND binding_id = ? ORDER BY id`, id, bindingID)
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
		if IsRegistrationActive(reg.Status) {
			return nil, ErrRegistrationTransition
		}
		regs = append(regs, reg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(regs) == 0 {
		var tracked bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM managed_users WHERE external_user_id = ? AND binding_id = ?)`, id, bindingID).Scan(&tracked); err != nil {
			return nil, err
		}
		if !tracked {
			return nil, ErrNotFound
		}
	}
	return regs, nil
}
