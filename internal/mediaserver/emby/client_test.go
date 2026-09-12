package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestClientUsesEmbyAuthorizationAndSetsPasswordSeparately(t *testing.T) {
	var requests int
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		if !strings.HasPrefix(r.Header.Get("X-Emby-Authorization"), "Emby ") {
			t.Fatalf("authorization = %q", r.Header.Get("X-Emby-Authorization"))
		}
		if token := r.Header.Get("X-Emby-Token"); token != "api-key" {
			t.Fatalf("token header = %q", token)
		}
		switch r.URL.Path {
		case "/emby/Users/New":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["Name"] != "alice" || body["Password"] != "" {
				t.Fatalf("create body = %#v", body)
			}
			return response(http.StatusOK, `{"Id":"user-1","Name":"alice"}`), nil
		case "/emby/Users/user-1/Password":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["NewPw"] != "password" || body["ResetPassword"] != false {
				t.Fatalf("password body = %#v", body)
			}
			return response(http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})})

	user, err := client.CreateUser(t.Context(), "http://emby.test", "api-key", "alice", "password", func(mediaserver.User) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || requests != 2 {
		t.Fatalf("user = %#v, requests = %d", user, requests)
	}
}

func TestPasswordFailureReturnsPersistedIncompleteAccount(t *testing.T) {
	persisted := false
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/emby/Users/New" {
			return response(200, `{"Id":"user-1","Name":"alice"}`), nil
		}
		if r.URL.Path != "/emby/Users/user-1/Password" || !persisted {
			t.Fatalf("password request before durable ID: %s", r.URL.Path)
		}
		return response(500, ""), nil
	})})
	user, err := client.CreateUser(t.Context(), "http://emby.test", "api-key", "alice", "password", func(u mediaserver.User) error { persisted = u.ID == "user-1"; return nil })
	if err == nil || user.ID != "user-1" || !persisted {
		t.Fatalf("user=%+v persisted=%v err=%v", user, persisted, err)
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

func TestPersistFailureStopsBeforePasswordSetup(t *testing.T) {
	calls := 0
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return response(200, `{"Id":"partial-user","Name":"alice"}`), nil
	})})
	persistErr := errors.New("store unavailable")
	user, err := client.CreateUser(t.Context(), "http://emby.test", "api-key", "alice", "password", func(mediaserver.User) error { return persistErr })
	if !errors.Is(err, persistErr) || user.ID != "partial-user" || calls != 1 {
		t.Fatalf("user=%+v calls=%d err=%v", user, calls, err)
	}
}
