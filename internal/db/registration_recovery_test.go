package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestTemplateRecoveryUsesReservationSnapshot(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.UpdateTemplate(ctx, Template{
		ID:          1,
		Name:        "Original",
		Description: "original",
		PolicyJSON:  `{"EnableAllFolders":false}`,
	}); err != nil {
		t.Fatal(err)
	}
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "hash-recovery", TemplateID: 1, MaxUses: 1, UserExpiryDays: 7, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	registrationID, snapshot, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Name != "Original" || snapshot.PolicyJSON != `{"EnableAllFolders":false}` {
		t.Fatalf("reservation snapshot = %#v", snapshot)
	}
	if err := store.BeginUserCreation(ctx, registrationID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, registrationID, "jf-alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, registrationID, RegistrationNeedsAttention, "policy failed"); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateTemplate(ctx, Template{
		ID:          1,
		Name:        "Changed",
		Description: "changed",
		PolicyJSON:  `{"EnableAllFolders":true}`,
	}); err != nil {
		t.Fatal(err)
	}

	recovery, err := store.ClaimTemplateRecovery(ctx, registrationID, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if recovery.Template.Name != "Original" || recovery.Template.PolicyJSON != `{"EnableAllFolders":false}` {
		t.Fatalf("recovery template = %#v, want reservation snapshot", recovery.Template)
	}
	if recovery.UserExpiryDays != 7 || recovery.Registration.ExternalUserID.String != "jf-alice" {
		t.Fatalf("recovery metadata = %#v", recovery)
	}
	if _, err := store.ClaimTemplateRecovery(ctx, registrationID, 1, false); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("concurrent ClaimTemplateRecovery error = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE registrations SET updated_at = datetime('now', '-1 hour') WHERE id = ?`, registrationID); err != nil {
		t.Fatal(err)
	}
	reconciled, err := store.ReconcileStaleRegistrations(ctx, time.Now().Add(-15*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.FlaggedAmbiguous != 1 {
		t.Fatalf("retry reconciliation = %#v", reconciled)
	}
	recovery, err = store.ClaimTemplateRecovery(ctx, registrationID, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTemplateRecovery(ctx, registrationID); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteTemplateRecovery(ctx, registrationID); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("second CompleteTemplateRecovery error = %v", err)
	}
}

func TestReconcileStaleRegistrationsReleasesOnlySafeReservations(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "hash-reconcile", TemplateID: 1, MaxUses: 4, BindingID: 1})
	if err != nil {
		t.Fatal(err)
	}
	reservedID, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "reserved")
	if err != nil {
		t.Fatal(err)
	}
	creatingID, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "creating")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, creatingID); err != nil {
		t.Fatal(err)
	}
	applyingID, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "applying")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, applyingID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, applyingID, "jf-applying"); err != nil {
		t.Fatal(err)
	}
	legacyID, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE registrations SET status = 'pending' WHERE id = ?`, legacyID); err != nil {
		t.Fatal(err)
	}
	staleAt := time.Now().Add(-time.Hour).UTC()
	if _, err := store.db.ExecContext(ctx, `UPDATE registrations SET updated_at = ? WHERE id IN (?, ?, ?, ?)`, staleAt, reservedID, creatingID, applyingID, legacyID); err != nil {
		t.Fatal(err)
	}

	result, err := store.ReconcileStaleRegistrations(ctx, time.Now().Add(-15*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReleasedReservations != 1 || result.FlaggedAmbiguous != 3 {
		t.Fatalf("ReconcileStaleRegistrations = %#v", result)
	}
	var uses int
	if err := store.db.QueryRowContext(ctx, `SELECT uses FROM invites WHERE id = ?`, inviteID).Scan(&uses); err != nil {
		t.Fatal(err)
	}
	if uses != 3 {
		t.Fatalf("invite uses = %d, want 3", uses)
	}
	statuses := map[int64]string{}
	rows, err := store.db.QueryContext(ctx, `SELECT id, status FROM registrations WHERE id IN (?, ?, ?, ?)`, reservedID, creatingID, applyingID, legacyID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			t.Fatal(err)
		}
		statuses[id] = status
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if statuses[reservedID] != RegistrationAbandonedBeforeUser {
		t.Fatalf("reserved status = %q", statuses[reservedID])
	}
	for _, id := range []int64{creatingID, applyingID, legacyID} {
		want := RegistrationFailedCreateUser
		if id == applyingID {
			want = RegistrationNeedsAttention
		}
		if statuses[id] != want {
			t.Fatalf("ambiguous registration %d status = %q", id, statuses[id])
		}
	}
	var applyingUserID sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT external_user_id FROM registrations WHERE id = ?`, applyingID).Scan(&applyingUserID); err != nil {
		t.Fatal(err)
	}
	if !applyingUserID.Valid || applyingUserID.String != "jf-applying" {
		t.Fatalf("applying registration lost media-server user ID: %#v", applyingUserID)
	}
	result, err = store.ReconcileStaleRegistrations(ctx, time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if result != (ReconciliationResult{}) {
		t.Fatalf("second reconciliation = %#v, want no changes", result)
	}
}
