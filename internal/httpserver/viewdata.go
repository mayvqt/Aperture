package httpserver

import "github.com/mayvqt/aperture/internal/db"

type sessionView struct {
	Username string
}

type settingsView struct {
	Provider  string
	PublicURL string
	ServerURL string
}

type viewData struct {
	Session          *sessionView
	CSRF             string
	Error            string
	Message          string
	Title            string
	Settings         settingsView
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
