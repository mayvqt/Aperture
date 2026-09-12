package httpserver

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

func (s *Server) dashboardData(ctx context.Context) ([]db.Invite, []db.Registration, db.DashboardCounts, error) {
	invites, err := s.store.InvitePreview(ctx, 8)
	if err != nil {
		return nil, nil, db.DashboardCounts{}, err
	}
	regs, err := s.store.RecentRegistrations(ctx, 8)
	if err != nil {
		return nil, nil, db.DashboardCounts{}, err
	}
	op, err := operationSnapshot(ctx)
	if err != nil {
		return nil, nil, db.DashboardCounts{}, err
	}
	counts, err := s.store.DashboardCounts(ctx, op.Identity.Binding.ID)
	if err != nil {
		return nil, nil, db.DashboardCounts{}, err
	}
	return invites, regs, counts, nil
}
func dashboardStatsFrom(invites []db.Invite, regs []db.Registration) dashboardStats {
	var stats dashboardStats
	now := time.Now()
	for _, invite := range invites {
		if invite.Enabled && !invite.DeletedAt.Valid && invite.Uses < invite.MaxUses && (!invite.ExpiresAt.Valid || invite.ExpiresAt.Time.After(now)) {
			stats.ActiveInvites++
		}
	}
	for _, reg := range regs {
		stats.Registrations++
		if db.IsRegistrationAttentionStatus(reg.Status) {
			stats.NeedsAttention++
		}
		if reg.UserDisableAt.Valid && !reg.UserDisabledAt.Valid {
			stats.ScheduledUserDisables++
		}
	}
	return stats
}

func (s *Server) dashboardHealth(ctx context.Context, settings db.Settings, counts db.DashboardCounts) []healthCheck {
	var checks []healthCheck
	if strings.TrimSpace(settings.ServerURL) == "" {
		checks = append(checks, healthCheck{
			Level:  "bad",
			Title:  s.serverName() + " URL missing",
			Detail: "Save the media-server URL before creating live invites.",
			URL:    "/admin/settings",
			Action: "Open settings",
		})
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		checks = append(checks, healthCheck{
			Level:  "bad",
			Title:  "API key missing",
			Detail: "Add a media-server API key so templates and account expiry can work.",
			URL:    "/admin/settings",
			Action: "Open settings",
		})
	}
	if counts.Templates == 0 {
		checks = append(checks, healthCheck{
			Level:  "warn",
			Title:  "No templates",
			Detail: "Create or import a template before sending invite links.",
			URL:    "/admin/templates",
			Action: "Open templates",
		})
	}
	if settings.ServerURL != "" && settings.APIKey != "" {
		if !s.mediaHealthy(ctx, settings.ServerURL, settings.APIKey) {
			checks = append(checks, healthCheck{
				Level:  "bad",
				Title:  s.serverName() + " check failed",
				Detail: "Aperture could not reach the media server with the saved connection details.",
				URL:    "/admin/settings",
				Action: "Check settings",
			})
		}
	}
	if counts.NeedsAttention > 0 {
		checks = append(checks, healthCheck{Level: "warn", Title: "Registrations need review", Detail: "Review incomplete accounts and records saved for an unverified or previous server.", URL: "/admin/registrations?review=1", Action: "Review accounts"})
	}
	if counts.Templates > 0 && counts.ActiveInvites == 0 {
		checks = append(checks, healthCheck{
			Level:  "warn",
			Title:  "No active invites",
			Detail: "Create an invite when you are ready to onboard the next media-server user.",
			URL:    "/admin/invites/new",
			Action: "Create invite",
		})
	}
	return checks
}

func (s *Server) processDueUserDisables(ctx context.Context) int {
	settings, err := s.settings(ctx)
	if err != nil || settings.ServerURL == "" || settings.APIKey == "" {
		return 0
	}
	op, err := operationSnapshot(ctx)
	if err != nil {
		return 0
	}
	regs, err := s.store.DueUserDisables(ctx, op.Identity.Binding.ID, 25)
	if err != nil {
		slog.Warn("could not list due media-server user disables", "error", safeError(err))
		return 0
	}
	disabled := 0
	for _, reg := range regs {
		if ctx.Err() != nil {
			break
		}
		operationCtx, err := s.refreshAccountContext(ctx)
		if err != nil {
			break
		}
		release, err := s.claimAccountOperations(reg.ID)
		if err != nil {
			continue
		}
		current, err := s.store.Registration(ctx, reg.ID)
		if err != nil || !current.NeedsDisable(time.Now()) {
			release()
			continue
		}
		releaseUser, claimErr := s.claimMediaUser(op.Identity.Binding.ID, current.ExternalUserID.String)
		if claimErr != nil {
			release()
			continue
		}
		err = s.disableAccount(operationCtx, current)
		releaseUser()
		release()
		if err != nil {
			slog.Warn("media-server user disable will retry", "registration_id", reg.ID, "error", safeError(err))
			continue
		}
		disabled++
	}
	return disabled
}

func (s *Server) processDueTemplateRetries(ctx context.Context) {
	settings, err := s.settings(ctx)
	if err != nil || settings.ServerURL == "" || settings.APIKey == "" {
		return
	}
	op, err := operationSnapshot(ctx)
	if err != nil {
		return
	}
	ids, err := s.store.ListDueTemplateRecoveryIDs(ctx, op.Identity.Binding.ID, 10)
	if err != nil {
		slog.Warn("could not list template retries", "error", safeError(err))
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		operationCtx, err := s.refreshAccountContext(ctx)
		if err != nil {
			return
		}
		if err := s.recoverAccount(operationCtx, id, true); err != nil {
			slog.Warn("template recovery did not complete", "registration_id", id, "error", safeError(err))
		}
	}
}
