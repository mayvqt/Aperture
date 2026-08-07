package db

import (
	"strings"
	"testing"
)

func TestWebhookRoundTripEncryptsURL(t *testing.T) {
	ctx, store := testStore(t)
	id, err := store.CreateWebhook(ctx, Webhook{Name: "Ops", URL: "https://discord.example/api/webhooks/secret", Kind: "discord", Events: "registration.complete,template.failed", RoleIDs: "12345678901234567", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := store.db.QueryRowContext(ctx, `SELECT url_encrypted FROM webhooks WHERE id=?`, id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "discord.example") || strings.Contains(raw, "secret") {
		t.Fatal("webhook URL stored in plaintext")
	}
	hooks, err := store.ListWebhooks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || hooks[0].URL != "https://discord.example/api/webhooks/secret" || hooks[0].Kind != "discord" || hooks[0].RoleIDs != "12345678901234567" || !hooks[0].Enabled {
		t.Fatalf("hooks = %#v", hooks)
	}
}

func TestListAuditEventsNewestFirst(t *testing.T) {
	ctx, store := testStore(t)
	_ = store.Audit(ctx, "a", "first", "invite", "1", "ip", "ua", "{}")
	_ = store.Audit(ctx, "a", "second", "invite", "2", "ip", "ua", "{}")
	events, err := store.ListAuditEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Action != "second" {
		t.Fatalf("events = %#v", events)
	}
}
