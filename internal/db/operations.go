package db

import (
	"context"
	"time"
)

func (s *Store) ListAuditEvents(ctx context.Context, limit int) ([]AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, COALESCE(actor_user_id,''), action, COALESCE(target_type,''), COALESCE(target_id,''), COALESCE(ip_address,''), COALESCE(user_agent,''), metadata_json, created_at FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var v AuditEvent
		if err := rows.Scan(&v.ID, &v.ActorUserID, &v.Action, &v.TargetType, &v.TargetID, &v.IPAddress, &v.UserAgent, &v.MetadataJSON, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) PruneAuditEvents(ctx context.Context, before time.Time, limit int) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM audit_log
		WHERE id IN (
			SELECT id FROM audit_log
			WHERE created_at < ?
			ORDER BY id
			LIMIT ?
		)
	`, before.UTC(), limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,url_encrypted,kind,events,role_ids,enabled,created_at,updated_at FROM webhooks ORDER BY name,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		var v Webhook
		var encrypted string
		if err := rows.Scan(&v.ID, &v.Name, &encrypted, &v.Kind, &v.Events, &v.RoleIDs, &v.Enabled, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		v.URL, err = s.encryptor.DecryptString(encrypted)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) CreateWebhook(ctx context.Context, v Webhook) (int64, error) {
	encrypted, err := s.encryptor.EncryptString(v.URL)
	if err != nil {
		return 0, err
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO webhooks(name,url_encrypted,kind,events,role_ids,enabled,created_at,updated_at) VALUES(?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, v.Name, encrypted, v.Kind, v.Events, v.RoleIDs, v.Enabled)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
func (s *Store) DeleteWebhook(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrNotFound
	}
	return err
}
func (s *Store) ListDueTemplateRecoveryIDs(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id FROM registrations r WHERE r.external_user_id IS NOT NULL AND r.status IN (?,?) AND r.template_attempts < 6 AND (r.next_template_attempt_at IS NULL OR r.next_template_attempt_at <= CURRENT_TIMESTAMP) ORDER BY COALESCE(r.next_template_attempt_at,r.updated_at),r.id LIMIT ?`, RegistrationNeedsAttention, RegistrationFailedApplyTemplate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}
