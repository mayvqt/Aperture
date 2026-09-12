package httpserver

import "github.com/mayvqt/aperture/internal/db"

type viewData struct {
	BindingID                 int64
	HistoryBefore, NextBefore int64
	ReviewOnly                bool
	Review                    serverReview
	ManagedHistory            []db.ManagedUser
	Admin                     bool
	Username                  string
	CSRF                      string
	Error                     string
	Message                   string
	Title                     string
	AuthTitle                 string
	CurrentPage               string
	ServerName                string
	Provider                  string
	ServerURL                 string
	ProviderManaged           bool
	PublicURLManaged          bool
	ServerURLManaged          bool
	APIKeyManaged             bool
	CookieManaged             bool
	Template                  db.Template
	Templates                 []db.Template
	Invites                   []db.Invite
	InviteRows                []inviteRow
	Invite                    db.Invite
	// InviteExpiryChoice preserves the submitted selector when rendering a
	// validation error; parsed expiry timestamps alone cannot distinguish a
	// quick choice from a custom date.
	InviteExpiryChoice string
	Registrations      []db.Registration
	PublicURL          string
	Token              string
	FormUsername       string
	Stats              dashboardStats
	HealthChecks       []healthCheck
	Webhooks           []db.Webhook
	AuditEvents        []db.AuditEvent
	WebhookEvents      []webhookEventOption
	UserRows           []managedUserRow
}

type managedUserRow struct {
	ID             string
	Name           string
	Source         string
	Status         string
	StatusClass    string
	Tracked        bool
	Missing        bool
	Administrator  bool
	RegistrationID int64
}

type webhookEventOption struct{ Value, Label string }

type dashboardStats struct {
	ActiveInvites         int
	Registrations         int
	NeedsAttention        int
	ScheduledUserDisables int
}

type inviteRow struct {
	Invite   db.Invite
	Activity db.InviteActivity
}

type healthCheck struct {
	Level  string
	Title  string
	Detail string
	URL    string
	Action string
}
