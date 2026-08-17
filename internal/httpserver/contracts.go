package httpserver

import (
	"context"
	"database/sql"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

type Store interface {
	Settings(context.Context) (db.Settings, error)
	UpdateApplicationSettings(context.Context, *string, *string, *string, *string) error
	UpdateSetupSettings(context.Context, string, string, string, string) error
	CreateSession(context.Context, string, string, string, string, time.Duration) (string, string, error)
	Session(context.Context, string) (db.Session, error)
	DeleteSession(context.Context, string) error
	ListTemplates(context.Context) ([]db.Template, error)
	Template(context.Context, int64) (db.Template, error)
	CreateTemplate(context.Context, db.Template) error
	UpdateTemplate(context.Context, db.Template) error
	SetDefaultTemplate(context.Context, int64) error
	DeleteTemplate(context.Context, int64) error
	CreateInvite(context.Context, db.Invite) (int64, error)
	ListInvites(context.Context) ([]db.Invite, error)
	InvitePreview(context.Context, int) ([]db.Invite, error)
	InvitePreset(context.Context, int64) (db.Invite, error)
	InviteByHash(context.Context, string) (db.Invite, error)
	ReserveInviteUse(context.Context, int64, string, string, string) (int64, db.Template, error)
	BeginUserCreation(context.Context, int64) error
	FailUserCreation(context.Context, int64, string) error
	RecordCreatedUser(context.Context, int64, string) error
	CompleteRegistration(context.Context, int64, string, string, sql.NullTime) error
	ReconcileStaleRegistrations(context.Context, time.Time, int) (db.ReconciliationResult, error)
	DueUserDisables(context.Context, int) ([]db.Registration, error)
	MarkUserDisabled(context.Context, int64) error
	MarkUserDisableFailed(context.Context, int64, string) error
	SetInviteEnabled(context.Context, int64, bool) error
	DeleteInvite(context.Context, int64) error
	Audit(context.Context, string, string, string, string, string, string, string) error
	RecentRegistrations(context.Context, int) ([]db.Registration, error)
	Registration(context.Context, int64) (db.Registration, error)
	DeleteRegistration(context.Context, int64) error
	ClaimTemplateRecovery(context.Context, int64) (db.RegistrationRecovery, error)
	RecordTemplateRetryFailure(context.Context, int64, string) error
	CompleteTemplateRecovery(context.Context, int64, sql.NullTime) error
	DashboardCounts(context.Context) (db.DashboardCounts, error)
	LatestInviteActivity(context.Context) (map[int64]db.InviteActivity, error)
	ListAuditEvents(context.Context, int) ([]db.AuditEvent, error)
	PruneAuditEvents(context.Context, time.Time, int) (int64, error)
	ListWebhooks(context.Context) ([]db.Webhook, error)
	CreateWebhook(context.Context, db.Webhook) (int64, error)
	DeleteWebhook(context.Context, int64) error
	DueTemplateRecoveries(context.Context, int) ([]db.RegistrationRecovery, error)
}

type MediaServer interface {
	mediaserver.Server
	SetProvider(mediaserver.Provider) error
}

var _ Store = (*db.Store)(nil)
