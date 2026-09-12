package db

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestBindingMigrationQuarantinesAndPreservesLegacyHistory(t *testing.T) {
	for revision := 1; revision <= 5; revision++ {
		t.Run(fmt.Sprint(revision), func(t *testing.T) {
			store := legacyCleanupStore(t, revision)
			ctx := t.Context()
			var encrypted string
			if err := store.db.QueryRow(`SELECT token_encrypted FROM invites WHERE id=1`).Scan(&encrypted); err != nil {
				t.Fatal(err)
			}
			if revision >= 4 {
				if _, err := store.db.Exec(`INSERT INTO managed_users(external_user_id,username,created_at,updated_at) VALUES('legacy-user','Legacy','2020-01-01','2020-01-01')`); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.InitSchema(ctx); err != nil {
				t.Fatal(err)
			}
			if err := store.InitSchema(ctx); err != nil {
				t.Fatal(err)
			}
			seedTestBinding(t, ctx, store)
			v, err := store.Invite(ctx, 1)
			if err != nil || v.BindingID != 0 || v.Token != "synthetic-invite-token" {
				t.Fatalf("legacy invite assigned/changed: %+v %v", v, err)
			}
			var after string
			if err := store.db.QueryRow(`SELECT token_encrypted FROM invites WHERE id=1`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != encrypted {
				t.Fatal("encrypted invite rewritten")
			}
			if _, err := store.InviteByHash(ctx, "legacy-token-hash", 1); !errors.Is(err, ErrNotFound) {
				t.Fatal("unverified invite usable")
			}
			if due, err := store.DueUserDisables(ctx, 1, 10); err != nil || len(due) != 0 {
				t.Fatal("legacy cleanup targeted current server")
			}
			before, err := store.Registration(ctx, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.AdoptRegistration(ctx, 1, 1); err != nil {
				t.Fatal(err)
			}
			adopted, err := store.Registration(ctx, 1)
			if err != nil || !adopted.UserDisableAt.Time.Equal(before.UserDisableAt.Time) || adopted.BindingID != 1 {
				t.Fatal("adoption changed access deadline")
			}
			if err := store.AdoptInvite(ctx, 1, 1); err != nil {
				t.Fatal(err)
			}
			if _, err := store.InviteByHash(ctx, "legacy-token-hash", 1); err != nil {
				t.Fatal(err)
			}
			other, err := store.Registration(ctx, 2)
			if err != nil || other.BindingID != 0 {
				t.Fatal("invite adoption assigned unrelated account")
			}
			if revision >= 4 {
				users, err := store.ManagedUserReviewPage(ctx, 1, 0, 50)
				if err != nil || len(users) != 1 {
					t.Fatal("lost tracked-only history")
				}
				before := users[0]
				if err := store.AdoptManagedUser(ctx, before.ID, 1); err != nil {
					t.Fatal(err)
				}
				users, err = store.ListManagedUsers(ctx, 1)
				if err != nil || len(users) != 1 || !users[0].CreatedAt.Equal(before.CreatedAt) {
					t.Fatal("tracked user adoption lost creation history")
				}
			}
		})
	}
}

func TestBindingMigrationRollbackPreservesLegacySchema(t *testing.T) {
	store := legacyCleanupStore(t, 5)
	if _, err := store.db.Exec(`CREATE TABLE managed_users_bound(collision INTEGER)`); err != nil {
		t.Fatal(err)
	}
	if err := store.InitSchema(t.Context()); err == nil {
		t.Fatal("expected migration failure")
	}
	exists, err := schemaColumnExists(t.Context(), store.db, "registrations", "binding_id")
	if err != nil || exists {
		t.Fatal("partial binding columns persisted")
	}
	var revision int
	if err := store.db.QueryRow(`PRAGMA user_version`).Scan(&revision); err != nil || revision != 5 {
		t.Fatal("failed migration advanced version")
	}
	if _, err := store.db.Exec(`DROP TABLE managed_users_bound`); err != nil {
		t.Fatal(err)
	}
	if err := store.InitSchema(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestBindingsScopeQueriesBeforeLimitsAndGroupings(t *testing.T) {
	ctx, store := testStore(t)
	first, err := store.CreateInvite(ctx, Invite{BindingID: 1, TokenHash: "first", TemplateID: 1, MaxUses: 40})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		id, _, err := store.ReserveInviteUse(ctx, first, 1, "", "", fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`UPDATE registrations SET status='needs_attention',external_user_id='same-id',cleanup_pending=1 WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	c, err := store.MediaConnection(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, err = store.PublishMediaConnection(ctx, ConnectionUpdate{ExpectedGeneration: c.Generation, Origin: MediaBinding{Provider: "jellyfin", BaseURL: "http://other.test", ServerID: "synthetic-server"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateInvite(ctx, Invite{BindingID: c.Binding.ID, TokenHash: "second", TemplateID: 1, MaxUses: 2})
	if err != nil {
		t.Fatal(err)
	}
	id, _, err := store.ReserveInviteUse(ctx, second, c.Binding.ID, "", "", "current")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE registrations SET status='needs_attention',external_user_id='same-id',cleanup_pending=1 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	due, err := store.DueUserDisables(ctx, c.Binding.ID, 1)
	if err != nil || len(due) != 1 || due[0].ID != id {
		t.Fatal("foreign cleanup starved current work")
	}
	ids, err := store.ListDueTemplateRecoveryIDs(ctx, c.Binding.ID, 1)
	if err != nil || len(ids) != 1 || ids[0] != id {
		t.Fatal("foreign recovery starved current work")
	}
	if _, err := store.ClaimTemplateRecovery(ctx, id, 1, false); !errors.Is(err, ErrNotFound) {
		t.Fatal("cross-origin claim accepted")
	}
	users, err := store.RegistrationUsers(ctx, 1)
	if err != nil || len(users) != 1 || users[0].BindingID != 1 {
		t.Fatal("foreign maximum hid original account")
	}
	for _, bindingID := range []int64{1, c.Binding.ID} {
		if err := store.SaveManagedUser(ctx, ManagedUser{BindingID: bindingID, ExternalUserID: "same-id", Username: "same"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.DeleteUserRecords(ctx, c.Binding.ID, "same-id"); err != nil {
		t.Fatal(err)
	}
	tracked, err := store.ListManagedUsers(ctx, 1)
	if err != nil || len(tracked) != 1 {
		t.Fatal("foreign user collision deleted original tracking")
	}
	users, err = store.RegistrationUsers(ctx, 1)
	if err != nil || len(users) != 1 {
		t.Fatal("foreign deletion removed history")
	}
}

func TestAdoptionRechecksExpiredAccessOnNewServer(t *testing.T) {
	ctx, store := testStore(t)
	id, err := store.CreateInvite(ctx, Invite{BindingID: 1, TokenHash: "expired", TemplateID: 1, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	regID, _, err := store.ReserveInviteUse(ctx, id, 1, "", "", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE registrations SET binding_id=NULL,status='disabled_expired',external_user_id='alice',user_disable_at='2020-01-01',user_disabled_at='2020-01-02',next_disable_attempt_at='2099-01-01' WHERE id=?`, regID); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptRegistration(ctx, regID, 1); err != nil {
		t.Fatal(err)
	}
	r, err := store.Registration(ctx, regID)
	if err != nil || r.UserDisabledAt.Valid || !r.NeedsDisable(time.Now()) {
		t.Fatalf("old-server disable acknowledgement reused: %+v %v", r, err)
	}
}

func TestHistoryPagingFindsOldUnverifiedWork(t *testing.T) {
	ctx, store := testStore(t)
	invite, err := store.CreateInvite(ctx, Invite{BindingID: 1, TokenHash: "history", TemplateID: 1, MaxUses: 110})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 105; i++ {
		id, _, err := store.ReserveInviteUse(ctx, invite, 1, "", "", fmt.Sprint(i))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`UPDATE registrations SET status='complete' WHERE id=?`, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`UPDATE registrations SET binding_id=NULL WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	review, err := store.RegistrationPage(ctx, 1, 0, true, 50)
	if err != nil || len(review) != 1 || review[0].ID != 1 {
		t.Fatal("review filter hid old work")
	}
	first, err := store.RegistrationPage(ctx, 1, 0, false, 50)
	if err != nil || len(first) != 50 {
		t.Fatal("invalid first page")
	}
	second, err := store.RegistrationPage(ctx, 1, first[49].ID, false, 50)
	if err != nil || len(second) != 50 || second[0].ID >= first[49].ID {
		t.Fatal("history cursor repeated or omitted page")
	}
}

func TestTrackedUserAdoptionMergesDuplicateWithoutDeletingAccountHistory(t *testing.T) {
	ctx, store := testStore(t)
	result, err := store.db.Exec(`INSERT INTO managed_users(external_user_id,username,created_at,updated_at) VALUES('same','Legacy','2020-01-01','2020-01-01')`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveManagedUser(ctx, ManagedUser{BindingID: 1, ExternalUserID: "same", Username: "Current"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AdoptManagedUser(ctx, id, 1); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListManagedUsers(ctx, 1)
	if err != nil || len(users) != 1 || users[0].ID != id || users[0].CreatedAt.Year() != 2020 {
		t.Fatalf("duplicate adoption lost history: %+v %v", users, err)
	}
	if pending, err := store.ManagedUserReviewPage(ctx, 1, 0, 50); err != nil || len(pending) != 0 {
		t.Fatal("review row stranded")
	}
}
