package protocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestPingReturnsTypedHTTPError(t *testing.T) {
	client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(http.StatusForbidden, ""), nil
	})})

	err := client.Ping(t.Context(), "http://media.test", "bad-key")
	var httpErr *mediaserver.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusForbidden {
		t.Fatalf("error = %#v, want typed 403", err)
	}
}

func TestTransportErrorsDoNotExposeRequestSecretsOrPrivateURL(t *testing.T) {
	const (
		apiKey   = "transport-api-key"
		password = "transport-password"
		baseURL  = "http://private-media.test"
	)
	client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("request " + r.URL.String() + " Authorization=" + r.Header.Get("Authorization") + " password=" + password)
	})})

	err := client.DoJSON(t.Context(), baseURL, http.MethodPost, "/Users/New", apiKey, map[string]string{"Password": password}, nil)
	if err == nil {
		t.Fatal("expected transport error")
	}
	for _, secret := range []string{apiKey, password, baseURL, "private-media.test"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("transport error exposed %q in %q", secret, err)
		}
	}
}

func TestAuthenticateUsesUniqueDeviceID(t *testing.T) {
	var first string
	client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		header := r.Header.Get("X-Emby-Authorization")
		if first == "" {
			first = header
		} else if header == first {
			t.Fatal("authentication reused device authorization")
		}
		return response(http.StatusOK, `{"AccessToken":"token","User":{"Id":"user-1","Name":"admin","Policy":{"IsAdministrator":true}}}`), nil
	})})

	if _, err := client.Authenticate(t.Context(), "http://media.test", "admin", "password"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Authenticate(t.Context(), "http://media.test", "admin", "password"); err != nil {
		t.Fatal(err)
	}
}

func TestImportTemplateResolvesUsernameFromUserList(t *testing.T) {
	var paths []string
	client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/Users":
			return response(http.StatusOK, `[{"Id":"user-1","Name":"Alice"}]`), nil
		case "/Users/user-1":
			return response(http.StatusOK, `{"Id":"user-1","Name":"Alice","Policy":{"EnableAllFolders":false}}`), nil
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
			return nil, nil
		}
	})})

	imported, err := client.ImportTemplate(t.Context(), "http://media.test", "api-key", "device-id", "alice")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(paths, ",") != "/Users,/Users/user-1" {
		t.Fatalf("request paths = %v", paths)
	}
	if !strings.Contains(imported.PolicyJSON, `"EnableAllFolders":false`) {
		t.Fatalf("policy = %s", imported.PolicyJSON)
	}
}

func TestImportTemplateResolvesUserIDFromUserList(t *testing.T) {
	client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/Users":
			return response(http.StatusOK, `[{"Id":"user-1","Name":"Alice"}]`), nil
		case "/Users/user-1":
			return response(http.StatusOK, `{"Id":"user-1","Name":"Alice","Policy":{}}`), nil
		default:
			t.Fatalf("unexpected request path %q", r.URL.Path)
			return nil, nil
		}
	})})

	if _, err := client.ImportTemplate(t.Context(), "http://media.test", "api-key", "device-id", "USER-1"); err != nil {
		t.Fatal(err)
	}
}

func identityURL(value string) (string, error) { return value, nil }

var testAuthorization = Authorization{Header: "Authorization", Scheme: "MediaBrowser", TokenInAuth: true}

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
