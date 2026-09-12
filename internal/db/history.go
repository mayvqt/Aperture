package db

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s *Store) Invite(ctx context.Context, id int64) (Invite, error) {
	v, err := s.scanInvite(s.db.QueryRowContext(ctx, `SELECT `+inviteColumns+` FROM invites i JOIN templates t ON t.id=i.template_id WHERE i.id=? AND i.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Invite{}, ErrNotFound
	}
	return v, err
}

// History pages use id cursors so new arrivals do not shift older pages.
func (s *Store) InvitePage(ctx context.Context, before int64, limit int) ([]Invite, error) {
	query := `SELECT ` + inviteColumns + ` FROM invites i JOIN templates t ON t.id=i.template_id WHERE i.deleted_at IS NULL`
	args := []any{}
	if before > 0 {
		query += ` AND i.id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY i.id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		v, err := s.scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RegistrationPage(ctx context.Context, bindingID, before int64, review bool, limit int) ([]Registration, error) {
	query := `SELECT ` + registrationColumns + ` FROM registrations WHERE 1=1`
	args := []any{}
	if before > 0 {
		query += ` AND id < ?`
		args = append(args, before)
	}
	if review {
		query += ` AND (COALESCE(binding_id,0)<>? OR cleanup_pending=1 OR status IN ('needs_attention','failed_create_user','failed_apply_template','disable_failed'))`
		args = append(args, bindingID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Registration
	for rows.Next() {
		v, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) InvitePageActivity(ctx context.Context, ids []int64) (map[int64]InviteActivity, error) {
	out := map[int64]InviteActivity{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx, `SELECT r.invite_id,r.username,r.status,r.created_at FROM registrations r WHERE r.id IN (SELECT MAX(id) FROM registrations WHERE invite_id IN (`+strings.TrimRight(strings.Repeat("?,", len(ids)), ",")+`) GROUP BY invite_id)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v InviteActivity
		if err := rows.Scan(&v.InviteID, &v.Username, &v.Status, &v.CreatedAt); err != nil {
			return nil, err
		}
		out[v.InviteID] = v
	}
	return out, rows.Err()
}

func (s *Store) ManagedUserReviewPage(ctx context.Context, bindingID, before int64, limit int) ([]ManagedUser, error) {
	query := `SELECT id,external_user_id,username,created_at,updated_at,COALESCE(binding_id,0) FROM managed_users WHERE COALESCE(binding_id,0)<>?`
	args := []any{bindingID}
	if before > 0 {
		query += ` AND id < ?`
		args = append(args, before)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ManagedUser
	for rows.Next() {
		var v ManagedUser
		if err := rows.Scan(&v.ID, &v.ExternalUserID, &v.Username, &v.CreatedAt, &v.UpdatedAt, &v.BindingID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) ManagedUser(ctx context.Context, id int64) (ManagedUser, error) {
	var v ManagedUser
	err := s.db.QueryRowContext(ctx, `SELECT id,external_user_id,username,created_at,updated_at,COALESCE(binding_id,0) FROM managed_users WHERE id=?`, id).Scan(&v.ID, &v.ExternalUserID, &v.Username, &v.CreatedAt, &v.UpdatedAt, &v.BindingID)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return v, err
}
func (s *Store) AdoptManagedUser(ctx context.Context, id, bindingID int64) error {
	if bindingID <= 0 {
		return ErrConnectionChanged
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var externalID string
	if err := tx.QueryRowContext(ctx, `SELECT external_user_id FROM managed_users WHERE id=? AND COALESCE(binding_id,0)<>?`, id, bindingID).Scan(&externalID); errors.Is(err, sql.ErrNoRows) {
		return ErrRegistrationTransition
	} else if err != nil {
		return err
	}
	// Tracking a live account before reviewing its legacy row must not strand
	// that history. Keep the reviewed record and the earliest tracking date.
	if _, err := tx.ExecContext(ctx, `UPDATE managed_users SET created_at=MIN(created_at,COALESCE((SELECT created_at FROM managed_users WHERE binding_id=? AND external_user_id=?),created_at)) WHERE id=?`, bindingID, externalID, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM managed_users WHERE binding_id=? AND external_user_id=?`, bindingID, externalID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE managed_users SET binding_id=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, bindingID, id); err != nil {
		return err
	}
	return tx.Commit()
}
