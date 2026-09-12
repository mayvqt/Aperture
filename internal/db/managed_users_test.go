package db

import "testing"

func TestManagedUserRoundTrip(t *testing.T) {
	ctx, store := testStore(t)
	user := ManagedUser{ExternalUserID: "media-1", Username: "Alice", BindingID: 1}
	if err := store.SaveManagedUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	user.Username = "Alice Renamed"
	if err := store.SaveManagedUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	users, err := store.ListManagedUsers(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].Username != user.Username {
		t.Fatalf("users = %#v", users)
	}
	if err := store.DeleteUserRecords(ctx, 1, user.ExternalUserID); err != nil {
		t.Fatal(err)
	}
	users, err = store.ListManagedUsers(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 0 {
		t.Fatalf("users after delete = %#v", users)
	}
}
