package protocol

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
)

func TestApplyTemplateSendsCompleteTargetPolicy(t *testing.T) {
	for _, cleanup := range []bool{false, true} {
		t.Run(map[bool]string{false: "apply", true: "disable"}[cleanup], func(t *testing.T) {
			requests := 0
			client := NewWithHTTPClient(testAuthorization, identityURL, &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				requests++
				if requests == 1 {
					if r.Method != http.MethodGet || r.URL.Path != "/Users/new-user" {
						t.Fatalf("first request = %s %s", r.Method, r.URL.Path)
					}
					return response(200, `{"Id":"new-user","Policy":{"AuthenticationProviderId":"default-auth","PasswordResetProviderId":"default-reset","EnableAllFolders":true}}`), nil
				}
				if requests != 2 || r.Method != http.MethodPost || r.URL.Path != "/Users/new-user/Policy" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var policy struct {
					AuthenticationProviderID string `json:"AuthenticationProviderId"`
					PasswordResetProviderID  string `json:"PasswordResetProviderId"`
					IsAdministrator          bool
					IsDisabled               bool
					EnableAllFolders         bool
				}
				if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
					t.Fatal(err)
				}
				if policy.AuthenticationProviderID != "default-auth" || policy.PasswordResetProviderID != "default-reset" || policy.IsAdministrator || policy.IsDisabled != cleanup || !policy.EnableAllFolders {
					t.Fatalf("policy = %+v", policy)
				}
				return response(204, ""), nil
			})})
			var err error
			if cleanup {
				err = client.DisableUser(t.Context(), "http://media.test", "key", "new-user")
			} else {
				err = client.ApplyTemplate(t.Context(), "http://media.test", "key", "new-user", db.Template{PolicyJSON: db.TemplatePolicyDefaultJSON})
			}
			if err != nil || requests != 2 {
				t.Fatalf("requests/error = %d/%v", requests, err)
			}
		})
	}
}
