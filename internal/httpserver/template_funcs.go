package httpserver

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

type templateSummaryItem struct {
	Label string
	Value string
}

var templateFuncs = template.FuncMap{
	"stylesheetHash": func() string { return stylesheetHash },
	"scriptHash":     func() string { return scriptHash },
	"boolText": func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	},
	"statusClass": func(v string) string {
		if db.IsRegistrationAttentionStatus(v) {
			return "status bad"
		}
		switch v {
		case db.RegistrationComplete, db.RegistrationDisabledExpired:
			return "status good"
		default:
			return "status warn"
		}
	},
	"registrationStatus": func(v string) string {
		switch v {
		case db.RegistrationReserved, db.RegistrationCreatingUser, db.RegistrationApplyingTemplate:
			return "Creating account"
		case db.RegistrationRetryingTemplate:
			return "Retrying access"
		case db.RegistrationComplete:
			return "Active"
		case db.RegistrationNeedsAttention, db.RegistrationFailedApplyTemplate:
			return "Review needed"
		case db.RegistrationFailedCreateUser:
			return "Account not created"
		case db.RegistrationDisableFailed:
			return "Could not disable"
		case db.RegistrationDisabledExpired:
			return "Access expired"
		case db.RegistrationAbandonedBeforeUser:
			return "Cancelled"
		default:
			return "In progress"
		}
	},
	"inviteState": func(enabled bool, uses, maxUses int) string {
		if !enabled {
			return "Disabled"
		}
		if uses >= maxUses {
			return "Used"
		}
		return "Active"
	},
	"inviteStateClass": func(enabled bool, uses, maxUses int) string {
		if !enabled {
			return "bad"
		}
		if uses >= maxUses {
			return "warn"
		}
		return "good"
	},
	"inviteURL": func(publicURL, token string) string {
		if token == "" {
			return ""
		}
		return publicURL + "/i/" + token
	},
	"dateOnly": func(t time.Time) string {
		if t.IsZero() {
			return ""
		}
		return t.Format("2006-01-02")
	},
	"nullDate": func(t sql.NullTime) string {
		if t.Valid && !t.Time.IsZero() {
			return t.Time.Format("2006-01-02")
		}
		return ""
	},
	"inviteExpiryText": func(t sql.NullTime) string {
		if !t.Valid || t.Time.IsZero() {
			return "Never"
		}
		return t.Time.Format("2006-01-02")
	},
	"nullDateTime": func(t sql.NullTime) string {
		if t.Valid && !t.Time.IsZero() {
			return t.Time.UTC().Format("2006-01-02 15:04 UTC")
		}
		return ""
	},
	"canRetryTemplate": func(status string, userID sql.NullString) bool {
		return userID.Valid && db.CanRetryRegistrationTemplate(status)
	},
	"userExpiryText": func(days int) string {
		if days <= 0 {
			return "Never"
		}
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	},
	"textareaJSON": func(value, fallback string) string {
		if value == "" {
			return fallback
		}
		return value
	},
	"templateSummary": templateSummary,
	"auditAction": func(value string) string {
		parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '_' })
		for i := range parts {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
		return strings.Join(parts, " ")
	},
	"shortID": func(value string) string {
		if len(value) <= 10 {
			return value
		}
		return value[:8] + "…"
	},
	"dateTime": func(value time.Time) string {
		return value.UTC().Format("2006-01-02 15:04 UTC")
	},
	"hasMetadata": func(value string) bool {
		value = strings.TrimSpace(value)
		return value != "" && value != "{}" && value != "null"
	},
	"prettyJSON": func(value string) string {
		var out bytes.Buffer
		if json.Indent(&out, []byte(value), "", "  ") == nil {
			return out.String()
		}
		return value
	},
}

func templateSummary(t db.Template) []templateSummaryItem {
	policy := map[string]any{}
	_ = json.Unmarshal([]byte(t.PolicyJSON), &policy)

	admin := "Blocked"
	if value, ok := policy["IsAdministrator"].(bool); ok && value {
		admin = "Will be forced off"
	}
	libraryText := "Policy default"
	if enabled, ok := policy["EnableAllFolders"].(bool); ok && enabled {
		libraryText = "All libraries"
	}
	if folders, ok := policy["EnabledFolders"].([]any); ok && len(folders) > 0 {
		libraryText = fmt.Sprintf("%d folders", len(folders))
	}
	return []templateSummaryItem{
		{Label: "Admin", Value: admin},
		{Label: "Libraries", Value: libraryText},
		{Label: "Policy", Value: presenceText(len(policy))},
	}
}

func presenceText(count int) string {
	if count <= 0 {
		return "Default"
	}
	return fmt.Sprintf("%d settings", count)
}
