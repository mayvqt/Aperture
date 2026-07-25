package db

import (
	"context"
	"database/sql"
	"errors"
)

const templateColumns = `id, name, COALESCE(description, ''), policy_json, is_default, created_at, updated_at`

func (s *Store) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+templateColumns+`
		FROM templates
		ORDER BY is_default DESC, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var templates []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}
func (s *Store) Template(ctx context.Context, id int64) (Template, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+templateColumns+`
		FROM templates
		WHERE id = ?
	`, id)
	t, err := scanTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Template{}, ErrNotFound
	}
	return t, err
}
func (s *Store) CreateTemplate(ctx context.Context, t Template) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO templates (name, description, policy_json, is_default, created_at, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, t.Name, t.Description, t.PolicyJSON, t.IsDefault)
	return err
}
func (s *Store) UpdateTemplate(ctx context.Context, t Template) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE templates
		SET name = ?, description = ?, policy_json = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, t.Name, t.Description, t.PolicyJSON, t.ID)
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

func (s *Store) SetDefaultTemplate(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM templates WHERE id = ?`, id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE templates SET is_default = 0, updated_at = CURRENT_TIMESTAMP WHERE is_default = 1`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE templates SET is_default = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteTemplate(ctx context.Context, id int64) error {
	var isDefault bool
	if err := s.db.QueryRowContext(ctx, `SELECT is_default FROM templates WHERE id = ?`, id).Scan(&isDefault); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if isDefault {
		return ErrTemplateIsDefault
	}
	var uses int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invites WHERE template_id = ?`, id).Scan(&uses); err != nil {
		return err
	}
	if uses > 0 {
		return ErrTemplateInUse
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE id = ?`, id)
	return err
}

func scanTemplate(scanner rowScanner) (Template, error) {
	var t Template
	err := scanner.Scan(&t.ID, &t.Name, &t.Description, &t.PolicyJSON, &t.IsDefault, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}
