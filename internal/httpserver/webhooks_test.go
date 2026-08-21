package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

func TestWebhooksPageUsesExpandableCreateForm(t *testing.T) {
	handler := New(testConfig(), newFakeStore(), &fakeMediaServer{})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, adminRequest(t, http.MethodGet, "/admin/webhooks", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d; body %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, `<details class="panel workflow-card">`) || !strings.Contains(body, "Add webhook") {
		t.Fatalf("webhooks page is missing expandable create form:\n%s", body)
	}
}

func TestSendWebhookDiscordEmbed(t *testing.T) {
	var payload discordPayload
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil))}, nil
	})}
	hook := db.Webhook{URL: "https://example.test/hook", Kind: "discord", RoleIDs: "12345678901234567,98765432109876543"}
	if err := sendWebhookWithClient(context.Background(), client, hook, webhookNotice{Event: "registration.complete", Title: "Registration completed", Color: 123, Fields: map[string]string{"Username": "alice", "Template": "Family"}}); err != nil {
		t.Fatal(err)
	}
	if len(payload.Embeds) != 1 || payload.Embeds[0].Title != "Registration completed" || len(payload.Embeds[0].Fields) != 2 || payload.Content != "<@&12345678901234567> <@&98765432109876543>" || len(payload.AllowedMentions.Roles) != 2 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNormalizeRoleIDs(t *testing.T) {
	got, err := normalizeRoleIDs("12345678901234567, 98765432109876543")
	if err != nil || got != "12345678901234567,98765432109876543" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := normalizeRoleIDs("12345678901234567,@everyone"); err == nil {
		t.Fatal("accepted invalid role")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestValidateWebhookURL(t *testing.T) {
	for _, raw := range []string{"http://example.com/hook", "ftp://example.com/hook", "https://user:pass@example.com/hook"} {
		if validateWebhookURL(raw) == nil {
			t.Errorf("accepted %q", raw)
		}
	}
	for _, raw := range []string{"https://discord.com/api/webhooks/1/x", "http://localhost:9999/hook"} {
		if err := validateWebhookURL(raw); err != nil {
			t.Errorf("rejected %q: %v", raw, err)
		}
	}
}

func TestValidateWebhookKindURL(t *testing.T) {
	if err := validateWebhookKindURL("discord", "https://discord.com/api/webhooks/1/token"); err != nil {
		t.Fatal(err)
	}
	if err := validateWebhookKindURL("discord", "https://example.com/hook"); err == nil {
		t.Fatal("accepted non-Discord URL")
	}
	if err := validateWebhookKindURL("generic", "https://example.com/hook"); err != nil {
		t.Fatal(err)
	}
}

func TestWaitForWebhooksDrainsAcceptedDeliveries(t *testing.T) {
	s := &Server{}
	release := make(chan struct{})
	s.webhookWG.Add(1)
	go func() {
		defer s.webhookWG.Done()
		<-release
	}()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.waitForWebhooks(ctx) }()

	select {
	case <-done:
		t.Fatal("shutdown returned before delivery completed")
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
