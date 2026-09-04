package httpserver

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/security"
)

func (s *Server) invitesList(w http.ResponseWriter, r *http.Request, session db.Session) {
	invites, err := s.store.ListInvites(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	activity, err := s.store.LatestInviteActivity(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.Invites = invites
	data.InviteRows = inviteRows(invites, activity)
	_, publicURL, _ := s.runtimeSettings()
	data.PublicURL = strings.TrimRight(publicURL, "/")
	data.Stats = dashboardStatsFrom(invites, nil)
	render(w, "invites", data)
}
func (s *Server) invitesNew(w http.ResponseWriter, r *http.Request, session db.Session) {
	templates, err := s.store.ListTemplates(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	invite := defaultInviteForm()
	if presetID, err := strconv.ParseInt(r.URL.Query().Get("preset_id"), 10, 64); err == nil && presetID > 0 {
		preset, err := s.store.InvitePreset(r.Context(), presetID)
		if err != nil && !errors.Is(err, db.ErrNotFound) {
			s.error(w, err)
			return
		}
		if err == nil {
			invite = presetInviteForm(preset)
		}
	}
	data := s.data(r, session)
	data.Templates = templates
	data.Invite = invite
	render(w, "invite-new", data)
}
func (s *Server) invitesCreate(w http.ResponseWriter, r *http.Request, session db.Session) {
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The invite form could not be read.", http.StatusBadRequest)
		return
	}
	// Keep only the non-sensitive fields needed to restore the form after a
	// validation failure. In particular, never copy token or credential values
	// from the request into view data.
	invite := db.Invite{
		Label:          strings.TrimSpace(r.FormValue("label")),
		MaxUses:        boundedFormInt(r.Form, "max_uses", 1, 1, 500),
		UserExpiryDays: boundedFormInt(r.Form, "user_expiry_days", 0, 0, 3650),
	}
	expiresAt, err := parseInviteExpiry(r.Form)
	if err != nil {
		s.renderInviteCreateError(w, r, session, invite, err.Error(), http.StatusBadRequest)
		return
	}
	invite.ExpiresAt = expiresAt
	templateID, err := strconv.ParseInt(r.FormValue("template_id"), 10, 64)
	if err != nil {
		s.renderInviteCreateError(w, r, session, invite, "Choose a valid template.", http.StatusBadRequest)
		return
	}
	invite.TemplateID = templateID
	if len([]rune(invite.Label)) > maxInviteLabelLength {
		s.renderInviteCreateError(w, r, session, invite, "Invite labels must be at most 200 characters.", http.StatusBadRequest)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if strings.TrimSpace(settings.APIKey) == "" {
		s.renderInviteCreateError(w, r, session, invite, "Save a media-server API key in settings before creating invite links.", http.StatusConflict)
		return
	}
	token, err := security.RandomToken(32)
	if err != nil {
		s.error(w, err)
		return
	}
	hash := security.HashToken(settings.InviteSecret, token)
	inviteID, err := s.store.CreateInvite(r.Context(), db.Invite{
		TokenHash:       hash,
		TokenPrefix:     security.Prefix(token),
		Token:           token,
		Label:           invite.Label,
		TemplateID:      templateID,
		ExpiresAt:       expiresAt,
		MaxUses:         invite.MaxUses,
		UserExpiryDays:  invite.UserExpiryDays,
		CreatedByUserID: session.UserID,
	})
	if err != nil {
		s.error(w, err)
		return
	}
	s.audit(r, session, "invite.create", "invite", strconv.FormatInt(inviteID, 10), map[string]any{"label": invite.Label, "max_uses": invite.MaxUses})
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}

func (s *Server) renderInviteCreateError(w http.ResponseWriter, r *http.Request, session db.Session, invite db.Invite, message string, status int) {
	templates, err := s.store.ListTemplates(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.Error = message
	data.Templates = templates
	data.Invite = invite
	data.InviteExpiryChoice = strings.TrimSpace(r.FormValue("expires_after_days"))
	renderStatus(w, "invite-new", data, status)
}
func (s *Server) invitesDisable(w http.ResponseWriter, r *http.Request, session db.Session) {
	s.setInviteState(w, r, session, false)
}
func (s *Server) invitesEnable(w http.ResponseWriter, r *http.Request, session db.Session) {
	s.setInviteState(w, r, session, true)
}
func (s *Server) setInviteState(w http.ResponseWriter, r *http.Request, session db.Session, enabled bool) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid invite", "That invite does not exist.", http.StatusBadRequest)
		return
	}
	if err := s.store.SetInviteEnabled(r.Context(), id, enabled); err != nil {
		s.inviteError(w, err)
		return
	}
	action := "invite.disable"
	if enabled {
		action = "invite.enable"
	}
	s.audit(r, session, action, "invite", strconv.FormatInt(id, 10), nil)
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}
func (s *Server) invitesDelete(w http.ResponseWriter, r *http.Request, session db.Session) {
	id, err := idFromPath(r, "id")
	if err != nil {
		s.message(w, "Invalid invite", "That invite does not exist.", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteInvite(r.Context(), id); err != nil {
		s.inviteError(w, err)
		return
	}
	s.audit(r, session, "invite.delete", "invite", strconv.FormatInt(id, 10), nil)
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}

func (s *Server) inviteError(w http.ResponseWriter, err error) {
	if errors.Is(err, db.ErrNotFound) {
		s.message(w, "Invite not found", "That invite does not exist.", http.StatusNotFound)
		return
	}
	s.error(w, err)
}
