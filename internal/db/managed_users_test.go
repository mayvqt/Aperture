package db

import "testing"

func TestManagedUserRoundTrip(t *testing.T) {
	ctx, store := testStore(t)
	user := ManagedUser{ExternalUserID: "media-1", Username: "Alice"}
	if err := store.SaveManagedUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	user.Username = "Alice Renamed"
	if err := store.SaveManagedUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListManagedUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != user.Username {
		t.Fatalf("users = %#v", users)
	}
	if err := store.DeleteManagedUser(ctx, user.ExternalUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ManagedUser(ctx, user.ExternalUserID); err != ErrNotFound {
		t.Fatalf("lookup after delete error = %v, want ErrNotFound", err)
	}
}
