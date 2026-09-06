package emby

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

	user, err := client.CreateUser(t.Context(), "http://emby.test", "api-key", "alice", "password")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || requests != 2 {
		t.Fatalf("user = %#v, requests = %d", user, requests)
	}
}

func TestPasswordFailureDisablesIncompleteAccount(t *testing.T) {
	var disabled bool
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/New":
			return response(http.StatusOK, `{"Id":"user-1","Name":"alice"}`), nil
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/user-1/Password":
			return response(http.StatusInternalServerError, ""), nil
		case r.Method == http.MethodGet && r.URL.Path == "/emby/Users/user-1":
			return response(http.StatusOK, `{"Policy":{"IsAdministrator":false}}`), nil
		case r.Method == http.MethodPost && r.URL.Path == "/emby/Users/user-1/Policy":
			var policy map[string]any
			if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
				t.Fatal(err)
			}
			disabled = policy["IsDisabled"] == true && policy["IsAdministrator"] == false
			return response(http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return nil, nil
	})})

	user, err := client.CreateUser(t.Context(), "http://emby.test", "api-key", "alice", "password")
	if err == nil || user.ID != "user-1" || !disabled {
		t.Fatalf("user = %#v, error = %v, disabled = %t", user, err, disabled)
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

func TestPasswordCancellationStillDisablesIncompleteAccount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	disabled := false
	client := NewWithHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		switch r.URL.Path {
		case "/emby/Users/New":
			return response(http.StatusOK, `{"Id":"partial-user","Name":"alice"}`), nil
		case "/emby/Users/partial-user/Password":
			cancel()
			return nil, ctx.Err()
		case "/emby/Users/partial-user":
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 10*time.Second {
				t.Fatal("cleanup context is not bounded")
			}
			return response(http.StatusOK, `{"Policy":{"IsAdministrator":false}}`), nil
		case "/emby/Users/partial-user/Policy":
			var policy map[string]any
			if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
				t.Fatal(err)
			}
			disabled = policy["IsDisabled"] == true
			return response(http.StatusNoContent, ""), nil
		default:
			t.Fatalf("unexpected cleanup target %s", r.URL.Path)
			return nil, nil
		}
	})})
	user, err := client.CreateUser(ctx, "http://emby.test", "api-key", "alice", "password")
	if err == nil || user.ID != "partial-user" || !disabled {
		t.Fatalf("password cancellation left incomplete account enabled: user=%q disabled=%t err=%v", user.ID, disabled, err)
	}
}
