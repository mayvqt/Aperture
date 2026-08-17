package db

import (
	"context"
	"database/sql"
	"errors"
)

func (s *Store) ListManagedUsers(ctx context.Context) ([]ManagedUser, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT external_user_id, username, created_at, updated_at FROM managed_users ORDER BY username COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []ManagedUser
	for rows.Next() {
		var user ManagedUser
		if err := rows.Scan(&user.ExternalUserID, &user.Username, &user.CreatedAt, &user.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) SaveManagedUser(ctx context.Context, user ManagedUser) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO managed_users (external_user_id, username, created_at, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(external_user_id) DO UPDATE SET username = excluded.username, updated_at = CURRENT_TIMESTAMP
	`, user.ExternalUserID, user.Username)
	return err
}

func (s *Store) DeleteManagedUser(ctx context.Context, externalUserID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM managed_users WHERE external_user_id = ?`, externalUserID)
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

func (s *Store) ManagedUser(ctx context.Context, externalUserID string) (ManagedUser, error) {
	var user ManagedUser
	err := s.db.QueryRowContext(ctx, `SELECT external_user_id, username, created_at, updated_at FROM managed_users WHERE external_user_id = ?`, externalUserID).
		Scan(&user.ExternalUserID, &user.Username, &user.CreatedAt, &user.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedUser{}, ErrNotFound
	}
	return user, err
}

func (s *Store) DeleteUserRecords(ctx context.Context, externalUserID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM registrations WHERE external_user_id = ?`, externalUserID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_users WHERE external_user_id = ?`, externalUserID); err != nil {
		return err
	}
	return tx.Commit()
}
