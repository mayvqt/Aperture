package httpserver

import (
	"context"
	"database/sql"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

type fakeStore struct {
	settings              db.Settings
	session               db.Session
	template              db.Template
	invite                db.Invite
	invites               []db.Invite
	registrations         []db.Registration
	deletedRegistrationID int64
	managedUsers          []db.ManagedUser
	inviteActivity        map[int64]db.InviteActivity
	createdInvite         db.Invite
	createdTemplate       db.Template
	updatedTemplate       db.Template
	defaultTemplateID     int64
	deletedTemplateID     int64
	completedDisableAt    sql.NullTime
	dueDisables           []db.Registration
	markedDisabledID      int64
	failedDisableID       int64
	inviteErr             error
	templateErr           error
	reservedInviteUse     bool
	beganUserCreation     bool
	recordedUserID        string
	completedStatus       string
	recovery              db.RegistrationRecovery
	recoveryFailure       string
	recoveryCompleted     bool
	reconciledStale       bool
	reconciliation        db.ReconciliationResult
	settingWrites         []settingWrite
	setupSettingsErr      error
	deletedSessionID      string
	createdDeviceID       string
	webhooks              []db.Webhook
	auditEvents           []db.AuditEvent
	prunedAuditEvents     bool
	dueTemplateRetries    []db.RegistrationRecovery
}

type settingWrite struct {
	key    string
	value  string
	secret bool
}

func newFakeStore() *fakeStore {
	settings := db.Settings{
		Provider:      "jellyfin",
		ServerURL:     "http://media:8096",
		APIKey:        "api-key",
		SessionSecret: "session-secret-with-at-least-32-characters",
		InviteSecret:  "invite-secret-with-at-least-32-characters",
	}
	tmpl := db.Template{ID: 1, Name: "Default", PolicyJSON: `{"IsAdministrator":false}`, IsDefault: true}
	invite := db.Invite{ID: 1, TokenHash: "hash", TokenPrefix: "prefix", Token: "saved-token", Label: "Family", TemplateID: 1, Template: "Default", MaxUses: 3, Enabled: true, UserExpiryDays: 0}
	return &fakeStore{
		settings:      settings,
		session:       db.Session{ID: "session-id", UserID: "admin-id", Username: "admin", AccessToken: "access-token", DeviceID: "device-id", CSRFSecret: "csrf-secret", ExpiresAt: time.Now().Add(time.Hour)},
		template:      tmpl,
		invite:        invite,
		invites:       []db.Invite{invite},
		registrations: []db.Registration{{ID: 1, Username: "alice", Status: db.RegistrationComplete, ExternalUserID: sql.NullString{String: "media-alice", Valid: true}}},
		inviteActivity: map[int64]db.InviteActivity{
			1: {InviteID: 1, Username: "alice", Status: db.RegistrationComplete, CreatedAt: time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)},
		},
		recovery: db.RegistrationRecovery{
			Registration:   db.Registration{ID: 1, InviteID: 1, ExternalUserID: sql.NullString{String: "media-alice", Valid: true}, Status: db.RegistrationNeedsAttention, CreatedAt: time.Now().Add(-time.Hour)},
			Template:       tmpl,
			UserExpiryDays: 7,
		},
	}
}

func (f *fakeStore) Settings(context.Context) (db.Settings, error) { return f.settings, nil }
func (f *fakeStore) SetSetting(_ context.Context, key, value string, secret bool) error {
	f.settingWrites = append(f.settingWrites, settingWrite{key: key, value: value, secret: secret})
	switch key {
	case "media_provider":
		f.settings.Provider = value
	case "server_url":
		f.settings.ServerURL = value
	case "api_key":
		f.settings.APIKey = value
	}
	return nil
}
func (f *fakeStore) UpdateApplicationSettings(ctx context.Context, provider, publicURL, serverURL, apiKey *string) error {
	if provider != nil {
		if err := f.SetSetting(ctx, "media_provider", *provider, false); err != nil {
			return err
		}
	}
	if publicURL != nil {
		if err := f.SetSetting(ctx, "public_url", *publicURL, false); err != nil {
			return err
		}
		f.settings.PublicURL = *publicURL
	}
	if serverURL != nil {
		if err := f.SetSetting(ctx, "server_url", *serverURL, false); err != nil {
			return err
		}
	}
	if apiKey != nil {
		return f.SetSetting(ctx, "api_key", *apiKey, true)
	}
	return nil
}
func (f *fakeStore) UpdateSetupSettings(ctx context.Context, provider, publicURL, serverURL, apiKey string) error {
	if f.setupSettingsErr != nil {
		return f.setupSettingsErr
	}
	for _, setting := range []struct {
		key, value string
		secret     bool
	}{
		{"media_provider", provider, false},
		{"public_url", publicURL, false},
		{"server_url", serverURL, false},
		{"api_key", apiKey, true},
	} {
		if err := f.SetSetting(ctx, setting.key, setting.value, setting.secret); err != nil {
			return err
		}
	}
	f.settings.PublicURL = publicURL
	return nil
}
func (f *fakeStore) CreateSession(_ context.Context, _, _, _, deviceID string, _ time.Duration) (string, string, error) {
	f.createdDeviceID = deviceID
	return f.session.ID, f.session.CSRFSecret, nil
}
func (f *fakeStore) Session(_ context.Context, id string) (db.Session, error) {
	if id != f.session.ID {
		return db.Session{}, db.ErrNotFound
	}
	return f.session, nil
}
func (f *fakeStore) DeleteSession(_ context.Context, id string) error {
	f.deletedSessionID = id
	return nil
}
func (f *fakeStore) ListTemplates(context.Context) ([]db.Template, error) {
	return []db.Template{f.template}, nil
}
func (f *fakeStore) Template(context.Context, int64) (db.Template, error) {
	if f.templateErr != nil {
		return db.Template{}, f.templateErr
	}
	return f.template, nil
}
func (f *fakeStore) CreateTemplate(_ context.Context, tmpl db.Template) error {
	f.createdTemplate = tmpl
	return nil
}
func (f *fakeStore) UpdateTemplate(_ context.Context, tmpl db.Template) error {
	f.updatedTemplate = tmpl
	return nil
}
func (f *fakeStore) SetDefaultTemplate(_ context.Context, id int64) error {
	f.defaultTemplateID = id
	return nil
}
func (f *fakeStore) DeleteTemplate(_ context.Context, id int64) error {
	f.deletedTemplateID = id
	return nil
}
func (f *fakeStore) CreateInvite(_ context.Context, invite db.Invite) (int64, error) {
	f.createdInvite = invite
	f.createdInvite.ID = 99
	return 99, nil
}
func (f *fakeStore) ListInvites(context.Context) ([]db.Invite, error) { return f.invites, nil }
func (f *fakeStore) InvitePreview(_ context.Context, limit int) ([]db.Invite, error) {
	if limit > len(f.invites) {
		limit = len(f.invites)
	}
	invites := make([]db.Invite, limit)
	copy(invites, f.invites[:limit])
	for i := range invites {
		invites[i].Token = ""
		invites[i].TokenHash = ""
		invites[i].TokenPrefix = ""
	}
	return invites, nil
}
func (f *fakeStore) InvitePreset(_ context.Context, id int64) (db.Invite, error) {
	for _, invite := range f.invites {
		if invite.ID == id {
			invite.Token = ""
			invite.TokenHash = ""
			invite.TokenPrefix = ""
			return invite, nil
		}
	}
	return db.Invite{}, db.ErrNotFound
}
func (f *fakeStore) InviteByHash(context.Context, string) (db.Invite, error) {
	if f.inviteErr != nil {
		return db.Invite{}, f.inviteErr
	}
	return f.invite, nil
}
func (f *fakeStore) ReserveInviteUse(context.Context, int64, string, string, string) (int64, db.Template, error) {
	if f.templateErr != nil {
		return 0, db.Template{}, f.templateErr
	}
	f.reservedInviteUse = true
	return 123, f.template, nil
}
func (f *fakeStore) BeginUserCreation(context.Context, int64) error {
	f.beganUserCreation = true
	return nil
}
func (f *fakeStore) FailUserCreation(_ context.Context, _ int64, _ string) error {
	f.completedStatus = db.RegistrationFailedCreateUser
	return nil
}
func (f *fakeStore) RecordCreatedUser(_ context.Context, _ int64, userID string) error {
	f.recordedUserID = userID
	return nil
}
func (f *fakeStore) CompleteRegistration(_ context.Context, _ int64, status string, _ string, disableAt sql.NullTime) error {
	f.completedStatus = status
	f.completedDisableAt = disableAt
	return nil
}
func (f *fakeStore) ReconcileStaleRegistrations(context.Context, time.Time, int) (db.ReconciliationResult, error) {
	f.reconciledStale = true
	return f.reconciliation, nil
}
func (f *fakeStore) DueUserDisables(context.Context, int) ([]db.Registration, error) {
	return f.dueDisables, nil
}
func (f *fakeStore) MarkUserDisabled(_ context.Context, registrationID int64) error {
	f.markedDisabledID = registrationID
	return nil
}
func (f *fakeStore) MarkUserDisableFailed(_ context.Context, id int64, _ string) error {
	f.failedDisableID = id
	return nil
}
func (f *fakeStore) SetInviteEnabled(context.Context, int64, bool) error { return nil }
func (f *fakeStore) DeleteInvite(context.Context, int64) error           { return nil }
func (f *fakeStore) Audit(context.Context, string, string, string, string, string, string, string) error {
	return nil
}
func (f *fakeStore) RecentRegistrations(context.Context, int) ([]db.Registration, error) {
	return f.registrations, nil
}
func (f *fakeStore) RegistrationUsers(context.Context) ([]db.Registration, error) {
	return f.registrations, nil
}
func (f *fakeStore) Registration(_ context.Context, id int64) (db.Registration, error) {
	for _, registration := range f.registrations {
		if registration.ID == id {
			return registration, nil
		}
	}
	return db.Registration{}, db.ErrNotFound
}
func (f *fakeStore) DeleteRegistration(_ context.Context, id int64) error {
	f.deletedRegistrationID = id
	return nil
}
func (f *fakeStore) ListManagedUsers(context.Context) ([]db.ManagedUser, error) {
	return f.managedUsers, nil
}
func (f *fakeStore) SaveManagedUser(_ context.Context, user db.ManagedUser) error {
	for i := range f.managedUsers {
		if f.managedUsers[i].ExternalUserID == user.ExternalUserID {
			f.managedUsers[i] = user
			return nil
		}
	}
	f.managedUsers = append(f.managedUsers, user)
	return nil
}
func (f *fakeStore) DeleteUserRecords(_ context.Context, id string) error {
	for i := len(f.registrations) - 1; i >= 0; i-- {
		if f.registrations[i].ExternalUserID.String == id {
			f.registrations = append(f.registrations[:i], f.registrations[i+1:]...)
		}
	}
	for i := len(f.managedUsers) - 1; i >= 0; i-- {
		if f.managedUsers[i].ExternalUserID == id {
			f.managedUsers = append(f.managedUsers[:i], f.managedUsers[i+1:]...)
		}
	}
	return nil
}
func (f *fakeStore) ClaimTemplateRecovery(context.Context, int64) (db.RegistrationRecovery, error) {
	recovery := f.recovery
	recovery.Registration.Status = db.RegistrationRetryingTemplate
	return recovery, nil
}
func (f *fakeStore) RecordTemplateRetryFailure(_ context.Context, _ int64, message string) error {
	f.recoveryFailure = message
	return nil
}
func (f *fakeStore) CompleteTemplateRecovery(context.Context, int64, sql.NullTime) error {
	f.recoveryCompleted = true
	return nil
}
func (f *fakeStore) DashboardCounts(context.Context) (db.DashboardCounts, error) {
	stats := dashboardStatsFrom(f.invites, f.registrations)
	return db.DashboardCounts{
		ActiveInvites:         stats.ActiveInvites,
		Templates:             1,
		NeedsAttention:        stats.NeedsAttention,
		ScheduledUserDisables: stats.ScheduledUserDisables,
	}, nil
}
func (f *fakeStore) LatestInviteActivity(context.Context) (map[int64]db.InviteActivity, error) {
	return f.inviteActivity, nil
}
func (f *fakeStore) ListAuditEvents(context.Context, int) ([]db.AuditEvent, error) {
	return f.auditEvents, nil
}
func (f *fakeStore) PruneAuditEvents(context.Context, time.Time, int) (int64, error) {
	f.prunedAuditEvents = true
	return 0, nil
}
func (f *fakeStore) ListWebhooks(context.Context) ([]db.Webhook, error) { return f.webhooks, nil }
func (f *fakeStore) CreateWebhook(_ context.Context, hook db.Webhook) (int64, error) {
	hook.ID = 1
	f.webhooks = append(f.webhooks, hook)
	return 1, nil
}
func (f *fakeStore) DeleteWebhook(_ context.Context, id int64) error {
	for i, v := range f.webhooks {
		if v.ID == id {
			f.webhooks = append(f.webhooks[:i], f.webhooks[i+1:]...)
			return nil
		}
	}
	return db.ErrNotFound
}
func (f *fakeStore) DueTemplateRecoveries(context.Context, int) ([]db.RegistrationRecovery, error) {
	return f.dueTemplateRetries, nil
}

var _ Store = (*fakeStore)(nil)
