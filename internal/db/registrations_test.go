package db

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestDueUserDisablesOnlyReturnsDueEnabledUsers(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:      "hash",
		TokenPrefix:    "prefix",
		Label:          "test",
		TemplateID:     1,
		MaxUses:        3,
		UserExpiryDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	dueRegID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	futureRegID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, registrationID := range []int64{dueRegID, futureRegID} {
		if err := store.BeginUserCreation(ctx, registrationID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordCreatedUser(ctx, dueRegID, "jellyfin-alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, futureRegID, "jellyfin-bob"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, dueRegID, RegistrationComplete, "", sql.NullTime{Time: time.Now().Add(-time.Hour).UTC(), Valid: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, futureRegID, RegistrationComplete, "", sql.NullTime{Time: time.Now().Add(time.Hour).UTC(), Valid: true}); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueUserDisables(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 || due[0].ID != dueRegID {
		t.Fatalf("due disables = %#v, want only registration %d", due, dueRegID)
	}
	if err := store.MarkUserDisabled(ctx, dueRegID); err != nil {
		t.Fatal(err)
	}
	due, err = store.DueUserDisables(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("due disables after mark = %#v, want none", due)
	}
	if err := store.MarkUserDisabled(ctx, dueRegID); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("second MarkUserDisabled error = %v, want ErrRegistrationTransition", err)
	}
}

func TestDisableFailureUsesDurableExponentialBackoff(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "hash-backoff", TemplateID: 1, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	registrationID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, registrationID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, registrationID, "jf-alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, registrationID, RegistrationComplete, "", sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}); err != nil {
		t.Fatal(err)
	}
	before := time.Now().UTC()
	if err := store.MarkUserDisableFailed(ctx, registrationID, "jellyfin unavailable"); err != nil {
		t.Fatal(err)
	}
	regs, err := store.RecentRegistrations(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	reg := regs[0]
	if reg.Status != RegistrationDisableFailed || reg.DisableAttempts != 1 || !reg.NextDisableAttemptAt.Valid {
		t.Fatalf("registration after disable failure = %#v", reg)
	}
	if reg.NextDisableAttemptAt.Time.Before(before.Add(30*time.Second)) || reg.NextDisableAttemptAt.Time.After(before.Add(2*time.Minute)) {
		t.Fatalf("next attempt = %s, want about one minute after failure", reg.NextDisableAttemptAt.Time)
	}
	due, err := store.DueUserDisables(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("backed-off registration returned as due: %#v", due)
	}
}

func TestCompleteRegistrationRejectsRepeatedTransition(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "hash-transition", TemplateID: 1, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	registrationID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, registrationID); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, registrationID, "jf-alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, registrationID, RegistrationComplete, "", sql.NullTime{}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, registrationID, RegistrationComplete, "", sql.NullTime{}); !errors.Is(err, ErrRegistrationTransition) {
		t.Fatalf("second CompleteRegistration error = %v, want ErrRegistrationTransition", err)
	}
}

func TestUncertainUserCreationRetainsInviteUseAndNullUserID(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{TokenHash: "hash-uncertain", TemplateID: 1, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	registrationID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.BeginUserCreation(ctx, registrationID); err != nil {
		t.Fatal(err)
	}
	if err := store.FailUserCreation(ctx, registrationID, "connection reset"); err != nil {
		t.Fatal(err)
	}
	regs, err := store.RecentRegistrations(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if regs[0].ExternalUserID.Valid || regs[0].Status != RegistrationFailedCreateUser {
		t.Fatalf("uncertain registration = %#v", regs[0])
	}
	if _, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "bob"); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("second reservation error = %v, want ErrInviteUnavailable", err)
	}
}

func TestLatestInviteActivityReturnsNewestRegistrationPerInvite(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:   "hash-activity",
		TokenPrefix: "prefix",
		Label:       "Family",
		TemplateID:  1,
		MaxUses:     5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "bob"); err != nil {
		t.Fatal(err)
	}

	activity, err := store.LatestInviteActivity(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if activity[inviteID].Username != "bob" {
		t.Fatalf("latest username = %q, want bob", activity[inviteID].Username)
	}
}

func TestDashboardCountsCoverFullHistory(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:  "hash-dashboard",
		Label:      "Dashboard",
		TemplateID: 1,
		MaxUses:    3,
	})
	if err != nil {
		t.Fatal(err)
	}
	attentionID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	scheduledID, _, err := store.ReserveInviteUse(ctx, inviteID, "127.0.0.1", "test", "bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, registrationID := range []int64{attentionID, scheduledID} {
		if err := store.BeginUserCreation(ctx, registrationID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RecordCreatedUser(ctx, attentionID, "jf-alice"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCreatedUser(ctx, scheduledID, "jf-bob"); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, attentionID, RegistrationNeedsAttention, "policy failed", sql.NullTime{}); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteRegistration(ctx, scheduledID, RegistrationComplete, "", sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true}); err != nil {
		t.Fatal(err)
	}

	counts, err := store.DashboardCounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if counts.ActiveInvites != 1 || counts.Templates != 1 || counts.NeedsAttention != 1 || counts.ScheduledUserDisables != 1 {
		t.Fatalf("DashboardCounts = %#v", counts)
	}
}
