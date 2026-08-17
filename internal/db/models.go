package db

import (
	"database/sql"
	"time"
)

const TemplatePolicyDefaultJSON = "{}"

const (
	RegistrationReserved            = "reserved"
	RegistrationCreatingUser        = "creating_user"
	RegistrationApplyingTemplate    = "applying_template"
	RegistrationRetryingTemplate    = "retrying_template"
	RegistrationLegacyPending       = "pending"
	RegistrationAbandonedBeforeUser = "abandoned_before_user"
	RegistrationComplete            = "complete"
	RegistrationNeedsAttention      = "needs_attention"
	RegistrationFailedCreateUser    = "failed_create_user"
	RegistrationFailedApplyTemplate = "failed_apply_template"
	RegistrationDisableFailed       = "disable_failed"
	RegistrationDisabledExpired     = "disabled_expired"
)

type Settings struct {
	Provider      string
	PublicURL     string
	ServerURL     string
	APIKey        string
	SessionSecret string
	InviteSecret  string
}

type Template struct {
	ID          int64
	Name        string
	Description string
	PolicyJSON  string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Invite struct {
	ID              int64
	TokenHash       string
	TokenPrefix     string
	Token           string
	Label           string
	TemplateID      int64
	Template        string
	ExpiresAt       sql.NullTime
	MaxUses         int
	Uses            int
	Enabled         bool
	UserExpiryDays  int
	CreatedByUserID string
	LastUsedAt      sql.NullTime
	DeletedAt       sql.NullTime
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type InviteActivity struct {
	InviteID  int64
	Username  string
	Status    string
	CreatedAt time.Time
}

type DashboardCounts struct {
	ActiveInvites         int
	Templates             int
	NeedsAttention        int
	ScheduledUserDisables int
}

type ReconciliationResult struct {
	ReleasedReservations int
	FlaggedAmbiguous     int
}

type RegistrationRecovery struct {
	Registration   Registration
	Template       Template
	UserExpiryDays int
}

func IsRegistrationAttentionStatus(status string) bool {
	switch status {
	case RegistrationNeedsAttention, RegistrationFailedCreateUser, RegistrationFailedApplyTemplate, RegistrationDisableFailed:
		return true
	default:
		return false
	}
}

func CanRetryRegistrationTemplate(status string) bool {
	return status == RegistrationNeedsAttention || status == RegistrationFailedApplyTemplate
}

type Registration struct {
	ID                    int64
	InviteID              int64
	ExternalUserID        sql.NullString
	Username              string
	Status                string
	ErrorMessage          sql.NullString
	UserDisableAt         sql.NullTime
	UserDisabledAt        sql.NullTime
	DisableAttempts       int
	NextDisableAttemptAt  sql.NullTime
	TemplateAttempts      int
	NextTemplateAttemptAt sql.NullTime
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type AuditEvent struct {
	ID           int64
	ActorUserID  string
	Action       string
	TargetType   string
	TargetID     string
	IPAddress    string
	UserAgent    string
	MetadataJSON string
	CreatedAt    time.Time
}

type Webhook struct {
	ID        int64
	Name      string
	URL       string
	Kind      string
	Events    string
	RoleIDs   string
	Enabled   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ManagedUser struct {
	ExternalUserID string
	Username       string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
