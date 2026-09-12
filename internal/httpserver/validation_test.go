package httpserver

import (
	"strings"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
)

func TestValidateRegistration(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		confirm  string
		wantErr  bool
	}{
		{name: "valid", username: "angel_01", password: "correct horse", confirm: "correct horse"},
		{name: "short username", username: "ab", password: "correct horse", confirm: "correct horse", wantErr: true},
		{name: "bad username", username: "bad name", password: "correct horse", confirm: "correct horse", wantErr: true},
		{name: "short password", username: "angel", password: "short", confirm: "short", wantErr: true},
		{name: "long password", username: "angel", password: strings.Repeat("x", maxPasswordLength+1), confirm: strings.Repeat("x", maxPasswordLength+1), wantErr: true},
		{name: "mismatch", username: "angel", password: "correct horse", confirm: "wrong", wantErr: true},
		{name: "line break", username: "angel", password: "correct\nhorse", confirm: "correct\nhorse", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateRegistration(tt.username, tt.password, tt.confirm)
			if (got != "") != tt.wantErr {
				t.Fatalf("validateRegistration() = %q, want error %v", got, tt.wantErr)
			}
		})
	}
}

func TestCleanTemplateBoundsDurableFields(t *testing.T) {
	tests := []db.Template{
		{Name: strings.Repeat("n", maxTemplateNameLength+1), PolicyJSON: `{}`},
		{Name: "Family", Description: strings.Repeat("d", maxTemplateDescriptionLength+1), PolicyJSON: `{}`},
		{Name: "Family", PolicyJSON: `{"value":"` + strings.Repeat("x", maxTemplatePolicyBytes) + `"}`},
	}
	for _, template := range tests {
		if _, err := cleanTemplate(template); err == nil {
			t.Fatalf("cleanTemplate(%q) accepted oversized input", template.Name)
		}
	}
}

func TestCleanTemplateProtectsCaseInsensitiveAdministratorFlag(t *testing.T) {
	clean, err := cleanTemplate(db.Template{Name: "Family", PolicyJSON: `{"isAdministrator":true,"AuthenticationProviderId":"source"}`})
	if err != nil || clean.PolicyJSON != `{"IsAdministrator":false}` {
		t.Fatalf("template/error = %s/%v", clean.PolicyJSON, err)
	}
	if _, err := cleanTemplate(db.Template{Name: "Family", PolicyJSON: `{"IsAdministrator":false,"isAdministrator":true}`}); err == nil {
		t.Fatal("accepted duplicate access flag")
	}
}
