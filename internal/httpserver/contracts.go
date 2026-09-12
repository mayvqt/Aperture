package httpserver

import (
	"context"
	"time"

	"github.com/mayvqt/aperture/internal/connection"
	"github.com/mayvqt/aperture/internal/db"
)

type Store interface {
	connection.Store
	CreateSession(context.Context, db.SessionInput) (string, string, error)
	Session(context.Context, string) (db.Session, error)
	DeleteSession(context.Context, string) error
	ListTemplates(context.Context) ([]db.Template, error)
	Template(context.Context, int64) (db.Template, error)
	CreateTemplate(context.Context, db.Template) error
	UpdateTemplate(context.Context, db.Template) error
	SetDefaultTemplate(context.Context, int64) error
	DeleteTemplate(context.Context, int64) error
	CreateInvite(context.Context, db.Invite) (int64, error)
	InvitePreview(context.Context, int) ([]db.Invite, error)
	InvitePreset(context.Context, int64) (db.Invite, error)
	InviteByHash(context.Context, string, int64) (db.Invite, error)
	ReserveInviteUse(context.Context, int64, int64, string, string, string) (int64, db.Template, error)
	BeginUserCreation(context.Context, int64) error
	FailUserCreation(context.Context, int64, string) error
	RecordFailedUserCreation(context.Context, int64, string, string) error
	RecordCreatedUser(context.Context, int64, string) error
	RecordProvisioningUser(context.Context, int64, string) error
	CompleteRegistration(context.Context, int64, string, string) error
	RequireAccountCleanup(context.Context, int64, string) error
	ReconcileStaleRegistrations(context.Context, time.Time, int) (db.ReconciliationResult, error)
	DueUserDisables(context.Context, int64, int) ([]db.Registration, error)
	MarkUserDisabled(context.Context, int64) error
	MarkUserDisableFailed(context.Context, int64, string) error
	SetInviteEnabled(context.Context, int64, int64, bool) error
	DeleteInvite(context.Context, int64) error
	Audit(context.Context, string, string, string, string, string, string, string) error
	RecentRegistrations(context.Context, int) ([]db.Registration, error)
	RegistrationUsers(context.Context, int64) ([]db.Registration, error)
	Registration(context.Context, int64) (db.Registration, error)
	DeleteRegistration(context.Context, int64) error
	ListManagedUsers(context.Context, int64) ([]db.ManagedUser, error)
	SaveManagedUser(context.Context, db.ManagedUser) error
	DeleteUserRecords(context.Context, int64, string) error
	ClaimTemplateRecovery(context.Context, int64, int64, bool) (db.RegistrationRecovery, error)
	UserDeletionRegistrations(context.Context, int64, string) ([]db.Registration, error)
	RecordTemplateRetryFailure(context.Context, int64, string) error
	CompleteTemplateRecovery(context.Context, int64) error
	DashboardCounts(context.Context, int64) (db.DashboardCounts, error)
	ListAuditEvents(context.Context, int) ([]db.AuditEvent, error)
	PruneAuditEvents(context.Context, time.Time, int) (int64, error)
	ListWebhooks(context.Context) ([]db.Webhook, error)
	CreateWebhook(context.Context, db.Webhook) (int64, error)
	DeleteWebhook(context.Context, int64) error
	ListDueTemplateRecoveryIDs(context.Context, int64, int) ([]int64, error)
	Invite(context.Context, int64) (db.Invite, error)
	InvitePage(context.Context, int64, int) ([]db.Invite, error)
	InvitePageActivity(context.Context, []int64) (map[int64]db.InviteActivity, error)
	RegistrationPage(context.Context, int64, int64, bool, int) ([]db.Registration, error)
	ManagedUserReviewPage(context.Context, int64, int64, int) ([]db.ManagedUser, error)
	ManagedUser(context.Context, int64) (db.ManagedUser, error)
	AdoptManagedUser(context.Context, int64, int64) error
	MediaBindings(context.Context) ([]db.MediaBinding, error)
	AdoptInvite(context.Context, int64, int64) error
	AdoptRegistration(context.Context, int64, int64) error
}

var _ Store = (*db.Store)(nil)
