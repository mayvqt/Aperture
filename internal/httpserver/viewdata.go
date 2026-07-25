package httpserver

import "github.com/mayvqt/aperture/internal/db"

type viewData struct {
	Session          *db.Session
	CSRF             string
	Error            string
	Message          string
	Title            string
	Settings         db.Settings
	ServerName       string
	Provider         string
	ServerURL        string
	ProviderManaged  bool
	PublicURLManaged bool
	ServerURLManaged bool
	APIKeyManaged    bool
	CookieManaged    bool
	Template         db.Template
	Templates        []db.Template
	Invites          []db.Invite
	InviteRows       []inviteRow
	Invite           db.Invite
	Registrations    []db.Registration
	NewURL           string
	PublicURL        string
	Token            string
	FormUsername     string
	Stats            dashboardStats
	HealthChecks     []healthCheck
}

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
