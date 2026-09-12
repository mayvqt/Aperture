package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestReserveInviteUseHonorsMaxUses(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:   "hash",
		TokenPrefix: "prefix",
		Label:       "test",
		TemplateID:  1,
		MaxUses:     1, BindingID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	regID, _, err := store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if regID == 0 {
		t.Fatal("expected registration id")
	}
	_, _, err = store.ReserveInviteUse(ctx, inviteID, 1, "127.0.0.1", "test", "bob")
	if !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("expected ErrInviteUnavailable, got %v", err)
	}
}

func TestCreateInviteEncryptsRetainedTokenForCopyLinks(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:       "hash",
		TokenPrefix:     "prefix",
		Token:           "raw-token-for-copy",
		Label:           "test",
		TemplateID:      1,
		MaxUses:         1,
		CreatedByUserID: "admin-id", BindingID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw sql.NullString
	if err := store.db.QueryRowContext(ctx, `SELECT token_encrypted FROM invites WHERE id = ?`, inviteID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if !raw.Valid || raw.String == "" {
		t.Fatal("expected retained token ciphertext")
	}
	if raw.String == "raw-token-for-copy" {
		t.Fatal("invite token stored in plaintext")
	}
	var createdBy string
	if err := store.db.QueryRowContext(ctx, `SELECT created_by_user_id FROM invites WHERE id = ?`, inviteID).Scan(&createdBy); err != nil {
		t.Fatal(err)
	}
	if createdBy != "admin-id" {
		t.Fatalf("created_by_user_id = %q", createdBy)
	}
	invites, err := store.InvitePage(ctx, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) != 1 || invites[0].Token != "raw-token-for-copy" {
		t.Fatalf("ListInvites token = %#v, want decrypted token", invites)
	}
	preview, err := store.InvitePreview(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview) != 1 || preview[0].Label != "test" {
		t.Fatalf("InvitePreview = %#v, want invite summary", preview)
	}
	if preview[0].Token != "" || preview[0].TokenHash != "" || preview[0].TokenPrefix != "" {
		t.Fatalf("InvitePreview exposed token data: %#v", preview[0])
	}
	preset, err := store.InvitePreset(ctx, inviteID)
	if err != nil {
		t.Fatal(err)
	}
	if preset.Token != "" || preset.TokenHash != "" || preset.TokenPrefix != "" {
		t.Fatalf("InvitePreset exposed token data: %#v", preset)
	}
	if preset.Label != "test" || preset.TemplateID != 1 || preset.MaxUses != 1 {
		t.Fatalf("InvitePreset = %#v, want reusable invite fields", preset)
	}
}

func TestInviteStateMutationsReportMissingRows(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.SetInviteEnabled(ctx, 404, 1, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetInviteEnabled missing error = %v, want ErrNotFound", err)
	}
	if err := store.DeleteInvite(ctx, 404); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteInvite missing error = %v, want ErrNotFound", err)
	}
}

func TestDeletedInviteCannotBeReenabledOrDeletedAgain(t *testing.T) {
	ctx, store := testStore(t)
	inviteID, err := store.CreateInvite(ctx, Invite{
		TokenHash:   "hash",
		TokenPrefix: "prefix",
		Label:       "test",
		TemplateID:  1,
		MaxUses:     1, BindingID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteInvite(ctx, inviteID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InvitePreset(ctx, inviteID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("InvitePreset deleted error = %v, want ErrNotFound", err)
	}
	if err := store.SetInviteEnabled(ctx, inviteID, 1, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetInviteEnabled deleted error = %v, want ErrNotFound", err)
	}
	if err := store.DeleteInvite(ctx, inviteID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteInvite deleted error = %v, want ErrNotFound", err)
	}
}
