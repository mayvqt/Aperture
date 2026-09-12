package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountCleanupSurvivesExhaustedTemplateRetries(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "cleanup", TemplateID: 1, MaxUses: 1, UserExpiryDays: 7, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	reserved, err := store.Registration(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !reserved.UserDisableAt.Valid || reserved.UserDisableAt.Time.Sub(reserved.CreatedAt) != 7*24*time.Hour {
		t.Fatalf("reservation expiry=%+v", reserved)
	}
	if err := store.BeginUserCreation(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProvisioningUser(ctx, id, "alice-id"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, id, "alice-id"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, id, RegistrationNeedsAttention, "policy response lost"); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 6; attempt++ {
		if _, err := store.ClaimTemplateRecovery(ctx, id, 1, false); err != nil {
			t.Fatal(err)
		}
		if err := store.RecordTemplateRetryFailure(ctx, id, "response lost"); err != nil {
			t.Fatal(err)
		}
	}
	if ids, err := store.ListDueTemplateRecoveryIDs(ctx, 1, 10); err != nil || len(ids) != 0 {
		t.Fatalf("exhausted retries=%v %v", ids, err)
	}
	if _, err := store.ClaimTemplateRecovery(ctx, id, 1, true); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("automatic claim bypassed cap: %v", err)
	}
	due, err := store.DueUserDisables(ctx, 1, 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("missing durable cleanup=%+v %v", due, err)
	}
	if err := store.MarkUserDisabled(ctx, id); err != nil {
		t.Fatal(err)
	}
	clean, err := store.Registration(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if clean.CleanupPending || clean.UserDisabledAt.Valid || !clean.UserDisableAt.Time.Equal(reserved.UserDisableAt.Time) || clean.Status != RegistrationNeedsAttention {
		t.Fatalf("cleanup changed access window: %+v", clean)
	}
	if _, err := store.ClaimTemplateRecovery(ctx, id, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTemplateRecovery(ctx, id); err != nil {
		t.Fatal(err)
	}
	complete, err := store.Registration(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if complete.CleanupPending || !complete.UserDisableAt.Time.Equal(reserved.UserDisableAt.Time) {
		t.Fatalf("recovery extended expiry: %+v", complete)
	}
}

func TestInterruptedPasswordSetupHasCleanupButCannotActivate(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "password-crash", TemplateID: 1, MaxUses: 1, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordProvisioningUser(ctx, id, "partial-id"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE registrations SET updated_at = datetime('now','-1 hour') WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ReconcileStaleRegistrations(ctx, time.Now().Add(-15*time.Minute), 10); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimTemplateRecovery(ctx, id, 1, false); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("password-incomplete activation: %v", err)
	}
	due, err := store.DueUserDisables(ctx, 1, 10)
	if err != nil || len(due) != 1 || due[0].ExternalUserID.String != "partial-id" {
		t.Fatalf("interrupted password cleanup=%+v %v", due, err)
	}
}

func TestCleanupBackoffCannotPostponeAccountExpiry(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "cleanup-expiry", TemplateID: 1, MaxUses: 1, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE registrations SET external_user_id='alice', status='needs_attention',cleanup_pending=1,disable_attempts=9,user_disable_at=datetime('now','+5 minutes') WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkUserDisableFailed(ctx, id, "offline"); err != nil {
		t.Fatal(err)
	}
	r, err := store.Registration(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !r.NextDisableAttemptAt.Time.Equal(r.UserDisableAt.Time) {
		t.Fatalf("cleanup backoff passed expiry: %+v", r)
	}
}

func TestCleanupMigrationPreservesHistoryAndSecrets(t *testing.T) {
	for revision := 1; revision <= 5; revision++ {
		t.Run(fmt.Sprint(revision), func(t *testing.T) {
			store := legacyCleanupStore(t, revision)
			ctx := t.Context()
			var before string
			if err := store.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='api_key'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err := store.InitSchema(ctx); err != nil {
				t.Fatal(err)
			}
			if err := store.InitSchema(ctx); err != nil {
				t.Fatal(err)
			}
			var after string
			if err := store.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='api_key'`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatal("migration rewrote encrypted secret")
			}
			settings, err := store.Settings(ctx)
			if err != nil || settings.APIKey != "synthetic-api-key" {
				t.Fatalf("secret preservation: %v", err)
			}
			inv, err := store.Invite(ctx, 1)
			if err != nil || inv.Token != "synthetic-invite-token" {
				t.Fatalf("invite preservation: %v", err)
			}
			rows, err := store.RecentRegistrations(ctx, 10)
			if err != nil || len(rows) != 2 {
				t.Fatalf("history=%+v %v", rows, err)
			}
			for _, r := range rows {
				want := time.Date(2020, 1, 8, 0, 0, 0, 0, time.UTC)
				if r.Username == "existing-deadline" {
					want = time.Date(2020, 1, 3, 0, 0, 0, 0, time.UTC)
				}
				if !r.UserDisableAt.Valid || !r.UserDisableAt.Time.Equal(want) || !r.CleanupPending || r.NextDisableAttemptAt.Valid {
					t.Fatalf("migration extended or deferred cleanup: %+v", r)
				}
			}
		})
	}
}

func TestCleanupMigrationRollsBackAsOneTransaction(t *testing.T) {
	store := legacyCleanupStore(t, 4)
	if _, err := store.db.Exec(`CREATE TRIGGER reject_cleanup BEFORE UPDATE ON registrations BEGIN SELECT RAISE(ABORT,'synthetic migration failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.InitSchema(t.Context()); err == nil {
		t.Fatal("expected migration failure")
	}
	var revision int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	exists, err := schemaColumnExists(t.Context(), store.db, "registrations", "cleanup_pending")
	if err != nil || exists || revision != 4 {
		t.Fatalf("partial migration persisted: column=%v revision=%d err=%v", exists, revision, err)
	}
	var deadline sql.NullTime
	if err := store.db.QueryRow(`SELECT user_disable_at FROM registrations WHERE username='missing-deadline'`).Scan(&deadline); err != nil || deadline.Valid {
		t.Fatalf("failed migration altered deadline: %+v %v", deadline, err)
	}
}

func legacyCleanupStore(t *testing.T, revision int) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "aperture.db"), "test-encryption-key-32-characters")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if revision == 1 {
		if _, err := store.db.Exec(`DROP INDEX idx_registrations_template_retry; ALTER TABLE registrations DROP COLUMN template_attempts; ALTER TABLE registrations DROP COLUMN next_template_attempt_at`); err != nil {
			t.Fatal(err)
		}
	}
	if revision <= 2 {
		if _, err := store.db.Exec(`ALTER TABLE webhooks DROP COLUMN kind; ALTER TABLE webhooks DROP COLUMN role_ids`); err != nil {
			t.Fatal(err)
		}
	}
	if revision < 4 {
		if _, err := store.db.Exec(`DROP TABLE managed_users`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, revision)); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.SetSetting(ctx, "api_key", "synthetic-api-key", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO templates(id,name,policy_json,created_at,updated_at) VALUES(1,'Legacy','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}

	encrypted, err := store.encryptor.EncryptString("synthetic-invite-token")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.db.Exec(`INSERT INTO invites(token_hash,token_encrypted,template_id,max_uses,user_expiry_days,created_at,updated_at) VALUES('legacy-token-hash',?,1,2,7,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, encrypted)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO registrations(invite_id,username,external_user_id,status,user_disable_at,next_disable_attempt_at,created_at,updated_at) VALUES
	 (?, 'missing-deadline','first-user','needs_attention',NULL,datetime('now','+30 days'),'2020-01-01 00:00:00','2020-01-01 00:00:00'),
	 (?, 'existing-deadline','second-user','failed_apply_template','2020-01-03 00:00:00',NULL,'2020-01-01 00:00:00','2020-01-01 00:00:00')`, id, id); err != nil {
		t.Fatal(err)
	}
	if revision == 5 {
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := migrateAccountCleanup(ctx, tx); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	return store
}
