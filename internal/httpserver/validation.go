package httpserver

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

const (
	maxPasswordLength            = 1024
	maxTemplateNameLength        = 100
	maxTemplateDescriptionLength = 1000
	maxTemplatePolicyBytes       = 256 << 10
	maxInviteLabelLength         = 200
	maxUserAgentLength           = 512
)

func validateAPIKey(value string) error {
	if len(value) > config.MaxAPIKeyLength {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return errors.New("The API key is too long.")
	}
	return nil
}

func validateRegistration(username, password, confirm string) string {
	if !usernamePattern.MatchString(username) {
		return "Use a username with 3-32 letters, numbers, dots, underscores, or hyphens."
	}
	if len(password) < 8 {
		return "Use a password with at least 8 characters."
	}
	if len(password) > maxPasswordLength {
		return "Use a password with at most 1024 characters."
	}
	if strings.ContainsAny(password, "\x00\r\n") {
		return "Use a password without line breaks."
	}
	if password != confirm {
		return "Passwords do not match."
	}
	return ""
}
func templateFromForm(r *http.Request, id int64) (db.Template, error) {
	return cleanTemplate(db.Template{
		ID:          id,
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("description")),
		PolicyJSON:  r.FormValue("policy_json"),
	})
}
func cleanTemplate(t db.Template) (db.Template, error) {
	if t.Name == "" {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return db.Template{}, errors.New("Name is required.")
	}
	if len([]rune(t.Name)) > maxTemplateNameLength {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return db.Template{}, errors.New("Name must be at most 100 characters.")
	}
	if len([]rune(t.Description)) > maxTemplateDescriptionLength {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return db.Template{}, errors.New("Description must be at most 1000 characters.")
	}
	if len(t.PolicyJSON) > maxTemplatePolicyBytes {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return db.Template{}, errors.New("Policy JSON is too large.")
	}
	cleanPolicy, err := mediaserver.NormalizeTemplatePolicy(t.PolicyJSON)
	if err != nil {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return db.Template{}, errors.New("The policy must be a JSON object with unique property names and boolean access flags.")
	}
	t.PolicyJSON = cleanPolicy
	return t, nil
}
