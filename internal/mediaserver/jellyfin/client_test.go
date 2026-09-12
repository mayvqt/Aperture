package jellyfin

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientUsesJellyfinAuthorizationAndCreatePayload(t *testing.T) {
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "MediaBrowser ") || !strings.Contains(got, `Token="api-key"`) {
			t.Fatalf("authorization = %q", got)
		}
		if r.Header.Get("X-Emby-Token") != "" {
			t.Fatal("Jellyfin request used deprecated X-Emby-Token header")
		}
		if r.URL.Path != "/Users/New" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["Name"] != "alice" || body["Password"] != "password" {
			t.Fatalf("body = %#v", body)
		}
		return response(http.StatusOK, `{"Id":"user-1","Name":"alice"}`), nil
	})})

	user, err := client.CreateUser(t.Context(), "http://jellyfin.test", "api-key", "alice", "password", func(mediaserver.User) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" {
		t.Fatalf("user = %#v", user)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status),
		Header:  http.Header{"Content-Type": []string{"application/json"}},
		Body:    io.NopCloser(bytes.NewBufferString(body)),
		Request: (&http.Request{}).WithContext(context.Background()),
	}
}
