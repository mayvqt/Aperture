package httpserver

import (
	"errors"
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
	if err := s.recoverAccount(r.Context(), settings, id, false); err != nil {
		if errors.Is(err, db.ErrNotFound) || errors.Is(err, db.ErrRegistrationTransition) {
			s.registrationRecoveryError(w, err)
		} else {
			s.message(w, "Template retry failed", "Access could not be confirmed. Aperture will keep trying to disable the incomplete account; review the registration details before trying again.", http.StatusBadGateway)
		}
		return
	}
	s.audit(r, session, "registration.retry_template", "registration", strconv.FormatInt(id, 10), nil)
	http.Redirect(w, r, "/admin/registrations", http.StatusSeeOther)
}

func (s *Server) registrationsDelete(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid registration", "That registration does not exist.", http.StatusBadRequest)
		return
	}
	registration, err := s.store.Registration(r.Context(), id)
	if err != nil {
		s.registrationRecoveryError(w, err)
		return
	}
	deletedUpstream := false
	if registration.ExternalUserID.Valid && strings.TrimSpace(registration.ExternalUserID.String) != "" {
		settings, err := s.settings(r.Context())
		if err != nil {
			s.error(w, err)
			return
		}
		if strings.TrimSpace(settings.APIKey) == "" {
			s.message(w, "API key required", "Aperture must verify that the media-server user has been deleted first.", http.StatusConflict)
			return
		}
		deletedUpstream, err = s.deleteUserAndRecords(r.Context(), settings, registration.ExternalUserID.String)
		if err != nil {
			s.userDeleteError(w, err, deletedUpstream)
			return
		}
	} else {
		release, err := s.claimAccountOperations(id)
		if err != nil {
			s.registrationRecoveryError(w, err)
			return
		}
		defer release()
		if err := s.store.DeleteRegistration(r.Context(), id); err != nil {
			s.registrationRecoveryError(w, err)
			return
		}
	}
	s.audit(r, session, "registration.delete", "registration", strconv.FormatInt(id, 10), map[string]any{"username": registration.Username, "deleted_from_media_server": deletedUpstream})
	destination := "/admin/registrations"
	if r.FormValue("return_to") == "/admin" {
		destination = "/admin"
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
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
