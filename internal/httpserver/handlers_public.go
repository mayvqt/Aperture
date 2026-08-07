package httpserver

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/security"
)

func (s *Server) publicInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invite, err := s.validInvite(r.Context(), token)
	if err != nil {
		s.message(w, "Invite unavailable", "This invite is no longer available.", http.StatusNotFound)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		s.message(w, "Registration unavailable", "Ask the server admin to finish configuring account registration.", http.StatusServiceUnavailable)
		return
	}
	csrf, err := s.publicCSRF(w, r, token)
	if err != nil {
		s.error(w, err)
		return
	}
	render(w, "public-invite", viewData{Token: token, CSRF: csrf, Invite: invite})
}
func (s *Server) publicRegister(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !s.validPublicCSRF(r, token) {
		s.message(w, "Invalid request", "Refresh the invite page and try again.", http.StatusBadRequest)
		return
	}
	remoteIP := clientIP(r, s.trustedProxies)
	if !s.limiter.Allow("register:" + remoteIP + ":" + security.Prefix(token)) {
		s.message(w, "Too many attempts", "Wait a few minutes and try again.", http.StatusTooManyRequests)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The registration form could not be read.", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	invite, err := s.validInvite(r.Context(), token)
	if err != nil {
		s.message(w, "Invite unavailable", "This invite is no longer available.", http.StatusNotFound)
		return
	}
	if validationError := validateRegistration(username, password, r.FormValue("confirm_password")); validationError != "" {
		csrf, err := s.publicCSRF(w, r, token)
		if err != nil {
			s.error(w, err)
			return
		}
		render(w, "public-invite", viewData{Token: token, CSRF: csrf, Invite: invite, FormUsername: username, Error: validationError})
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		s.message(w, "Registration unavailable", "Ask the server admin to finish configuring account registration.", http.StatusServiceUnavailable)
		return
	}
	regID, template, err := s.store.ReserveInviteUse(r.Context(), invite.ID, remoteIP, requestUserAgent(r), username)
	if err != nil {
		if errors.Is(err, db.ErrInviteUnavailable) {
			s.message(w, "Invite unavailable", "This invite is no longer available.", http.StatusConflict)
		} else {
			s.error(w, err)
		}
		return
	}
	if err := s.store.BeginUserCreation(r.Context(), regID); err != nil {
		s.error(w, err)
		return
	}
	user, err := s.media.CreateUser(r.Context(), settings.ServerURL, settings.APIKey, username, password)
	if err != nil {
		if user.ID != "" {
			if recordErr := s.store.RecordCreatedUser(r.Context(), regID, user.ID); recordErr != nil {
				s.error(w, recordErr)
				return
			}
			if recordErr := s.store.CompleteRegistration(r.Context(), regID, db.RegistrationNeedsAttention, safeError(err), sql.NullTime{}); recordErr != nil {
				s.error(w, recordErr)
				return
			}
			slog.Warn("media-server user creation was partial", "registration_id", regID, "error", safeError(err))
			s.notify(webhookNotice{Event: "registration.failed", Title: "Registration needs review", Description: "The account was created but setup was incomplete.", Color: 0xe67e22, Fields: map[string]string{"Username": username, "Registration": strconv.FormatInt(regID, 10), "Error": safeError(err)}})
			s.message(w, "Account needs review", "The account was created but disabled because setup was incomplete. Ask the server admin to review it.", http.StatusAccepted)
			return
		}
		if recordErr := s.store.FailUserCreation(r.Context(), regID, safeError(err)); recordErr != nil {
			slog.Error("could not record uncertain media-server user creation", "registration_id", regID, "error", safeError(recordErr))
		}
		slog.Warn("media-server user creation failed", "error", safeError(err))
		s.notify(webhookNotice{Event: "registration.failed", Title: "Registration failed", Color: 0xe74c3c, Fields: map[string]string{"Username": username, "Registration": strconv.FormatInt(regID, 10), "Error": safeError(err)}})
		s.message(w, "Registration failed", "The account could not be created. Ask the server admin to check this invite.", http.StatusInternalServerError)
		return
	}
	if err := s.store.RecordCreatedUser(r.Context(), regID, user.ID); err != nil {
		s.error(w, err)
		return
	}
	if err := s.media.ApplyTemplate(r.Context(), settings.ServerURL, settings.APIKey, user.ID, template); err != nil {
		if recordErr := s.store.CompleteRegistration(r.Context(), regID, db.RegistrationNeedsAttention, safeError(err), sql.NullTime{}); recordErr != nil {
			slog.Error("could not record partial media-server registration", "registration_id", regID, "error", safeError(recordErr))
		}
		slog.Warn("media-server template application failed", "error", safeError(err))
		s.notify(webhookNotice{Event: "template.failed", Title: "Template application failed", Description: "Aperture will retry automatically.", Color: 0xe67e22, Fields: map[string]string{"Username": username, "Registration": strconv.FormatInt(regID, 10), "Template": template.Name, "Error": safeError(err)}})
		s.message(w, "Account needs review", "The account was created, but an admin needs to finish applying access.", http.StatusAccepted)
		return
	}
	if err := s.store.CompleteRegistration(r.Context(), regID, db.RegistrationComplete, "", disableAtFor(invite.UserExpiryDays)); err != nil {
		s.error(w, err)
		return
	}
	if err := s.store.Audit(r.Context(), "", "registration.complete", "invite", strconv.FormatInt(invite.ID, 10), remoteIP, requestUserAgent(r), "{}"); err != nil {
		slog.Warn("could not record registration audit event", "registration_id", regID, "error", safeError(err))
	}
	s.notify(webhookNotice{Event: "registration.complete", Title: "Registration completed", Color: 0x2ecc71, Fields: map[string]string{"Username": username, "Registration": strconv.FormatInt(regID, 10), "Invite": invite.Label, "Template": template.Name, "Account ID": user.ID}})
	http.Redirect(w, r, "/guide", http.StatusSeeOther)
}
func (s *Server) success(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/guide", http.StatusSeeOther)
}
func (s *Server) guide(w http.ResponseWriter, _ *http.Request) {
	render(w, "guide", viewData{})
}
