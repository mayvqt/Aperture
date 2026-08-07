package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
)

func (s *Server) registrationsList(w http.ResponseWriter, r *http.Request, session db.Session) {
	regs, err := s.store.RecentRegistrations(r.Context(), 100)
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.Registrations = regs
	render(w, "registrations", data)
}

func (s *Server) registrationsRetryTemplate(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid registration", "That registration does not exist.", http.StatusBadRequest)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		s.message(w, "API key required", "Save a media-server API key before retrying template application.", http.StatusConflict)
		return
	}
	recovery, err := s.store.ClaimTemplateRecovery(r.Context(), id)
	if err != nil {
		s.registrationRecoveryError(w, err)
		return
	}
	if strings.TrimSpace(recovery.Template.PolicyJSON) == "" {
		s.recordTemplateRetryFailure(r, id, errors.New("saved registration template is unavailable"))
		s.message(w, "Recovery unavailable", "The saved registration template is unavailable.", http.StatusConflict)
		return
	}
	if err := s.media.ApplyTemplate(r.Context(), settings.ServerURL, settings.APIKey, recovery.Registration.ExternalUserID.String, recovery.Template); err != nil {
		s.recordTemplateRetryFailure(r, id, err)
		s.message(w, "Template retry failed", "The media server did not accept the template. Review the saved details and try again.", http.StatusBadGateway)
		return
	}
	if err := s.store.CompleteTemplateRecovery(r.Context(), id, disableAtFrom(recovery.Registration.CreatedAt, recovery.UserExpiryDays)); err != nil {
		s.registrationRecoveryError(w, err)
		return
	}
	s.notify(webhookNotice{Event: "template.recovered", Title: "Access template recovered", Color: 0x2ecc71, Fields: map[string]string{"Username": recovery.Registration.Username, "Registration": strconv.FormatInt(id, 10), "Template": recovery.Template.Name}})
	s.audit(r, session, "registration.retry_template", "registration", strconv.FormatInt(id, 10), nil)
	http.Redirect(w, r, "/admin/registrations", http.StatusSeeOther)
}

func (s *Server) recordTemplateRetryFailure(r *http.Request, registrationID int64, err error) {
	if recordErr := s.store.RecordTemplateRetryFailure(r.Context(), registrationID, safeError(err)); recordErr != nil {
		slog.Error("could not record template retry failure", "registration_id", registrationID, "error", safeError(recordErr))
	}
}

func (s *Server) registrationRecoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		s.message(w, "Registration not found", "That registration does not exist.", http.StatusNotFound)
	case errors.Is(err, db.ErrRegistrationTransition):
		s.message(w, "Recovery unavailable", "That registration is no longer eligible for recovery.", http.StatusConflict)
	default:
		s.error(w, err)
	}
}
