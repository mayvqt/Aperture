package httpserver

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
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
	counts, err := s.store.DashboardCounts(ctx)
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

func disableAtFor(days int) sql.NullTime {
	return disableAtFrom(time.Now(), days)
}

func disableAtFrom(createdAt time.Time, days int) sql.NullTime {
	if days <= 0 {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: createdAt.AddDate(0, 0, days).UTC(), Valid: true}
}
func (s *Server) processDueUserDisables(ctx context.Context) int {
	settings, err := s.settings(ctx)
	if err != nil || settings.ServerURL == "" || settings.APIKey == "" {
		return 0
	}
	regs, err := s.store.DueUserDisables(ctx, 25)
	if err != nil {
		slog.Warn("could not list due media-server user disables", "error", safeError(err))
		return 0
	}
	disabled := 0
	for _, reg := range regs {
		if !reg.ExternalUserID.Valid {
			continue
		}
		if err := s.media.DisableUser(ctx, settings.ServerURL, settings.APIKey, reg.ExternalUserID.String); err != nil {
			if recordErr := s.store.MarkUserDisableFailed(ctx, reg.ID, safeError(err)); recordErr != nil {
				slog.Error("could not record media-server user disable failure", "registration_id", reg.ID, "error", safeError(recordErr))
			}
			slog.Warn("media-server user disable failed", "registration_id", reg.ID, "error", safeError(err))
			s.notify(webhookNotice{Event: "user.disable_failed", Title: "Expired user disable failed", Description: "Aperture will retry automatically.", Color: 0xe67e22, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10), "Error": safeError(err)}})
			continue
		}
		if err := s.store.MarkUserDisabled(ctx, reg.ID); err != nil {
			slog.Warn("could not mark media-server user disabled", "registration_id", reg.ID, "error", safeError(err))
			continue
		}
		disabled++
		s.notify(webhookNotice{Event: "user.disabled", Title: "Expired user disabled", Color: 0x2ecc71, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10)}})
	}
	return disabled
}

func (s *Server) processDueTemplateRetries(ctx context.Context) {
	settings, err := s.settings(ctx)
	if err != nil || settings.ServerURL == "" || settings.APIKey == "" {
		return
	}
	recoveries, err := s.store.DueTemplateRecoveries(ctx, 10)
	if err != nil {
		slog.Warn("could not claim template retries", "error", safeError(err))
		return
	}
	for _, recovery := range recoveries {
		reg := recovery.Registration
		if err := s.media.ApplyTemplate(ctx, settings.ServerURL, settings.APIKey, reg.ExternalUserID.String, recovery.Template); err != nil {
			_ = s.store.RecordTemplateRetryFailure(ctx, reg.ID, safeError(err))
			s.notify(webhookNotice{Event: "template.failed", Title: "Template retry failed", Description: "Aperture will retry with bounded backoff.", Color: 0xe67e22, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10), "Attempt": strconv.Itoa(reg.TemplateAttempts + 1), "Error": safeError(err)}})
			continue
		}
		if err := s.store.CompleteTemplateRecovery(ctx, reg.ID, disableAtFrom(reg.CreatedAt, recovery.UserExpiryDays)); err != nil {
			slog.Warn("could not complete automatic template recovery", "registration_id", reg.ID, "error", safeError(err))
			continue
		}
		s.notify(webhookNotice{Event: "template.recovered", Title: "Access template recovered", Color: 0x2ecc71, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10), "Template": recovery.Template.Name}})
	}
}
