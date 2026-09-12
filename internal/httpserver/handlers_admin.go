package httpserver

import (
	"net/http"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

func (s *Server) adminHome(w http.ResponseWriter, r *http.Request, session db.Session) {
	data := s.data(r, session)
	invites, regs, counts, err := s.dashboardData(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data.Invites = invites
	data.Registrations = regs
	data.Stats = dashboardStats{
		ActiveInvites:         counts.ActiveInvites,
		Registrations:         len(regs),
		NeedsAttention:        counts.NeedsAttention,
		ScheduledUserDisables: counts.ScheduledUserDisables,
	}
	data.HealthChecks = s.dashboardHealth(r.Context(), settings, counts)
	render(w, "admin", data)
}
func (s *Server) settingsForm(w http.ResponseWriter, r *http.Request, session db.Session) {
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	data := s.data(r, session)
	data.Provider = settings.Provider
	data.PublicURL = settings.PublicURL
	data.ServerURL = settings.ServerURL
	s.setMediaViewData(&data)
	render(w, "settings", data)
}
func (s *Server) settingsPost(w http.ResponseWriter, r *http.Request, session db.Session) {
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The settings form could not be read.", http.StatusBadRequest)
		return
	}
	current, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	providerValue := strings.TrimSpace(r.FormValue("provider"))
	if s.cfg.ProviderManaged {
		providerValue = s.cfg.MediaProvider
	}
	provider, ok := mediaserver.ParseProvider(providerValue)
	if !ok {
		s.renderSettingsError(w, r, session, providerValue, r.FormValue("public_url"), r.FormValue("server_url"), "Choose Jellyfin or Emby.")
		return
	}
	publicURL := strings.TrimSpace(r.FormValue("public_url"))
	if s.cfg.PublicURLManaged {
		publicURL = s.cfg.PublicURL
	}
	publicURL, err = validatePublicURL(publicURL)
	if err != nil {
		s.renderSettingsError(w, r, session, string(provider), publicURL, r.FormValue("server_url"), err.Error())
		return
	}
	serverURL := strings.TrimSpace(r.FormValue("server_url"))
	if s.cfg.ServerURLManaged {
		serverURL = s.cfg.ServerURL
	}
	serverURL, err = validateServerURL(provider, serverURL)
	if err != nil {
		s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, err.Error())
		return
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if err := validateAPIKey(apiKey); err != nil {
		s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, err.Error())
		return
	}
	removeAPIKey := !s.cfg.APIKeyManaged && r.FormValue("remove_api_key") == "on"
	if removeAPIKey && apiKey != "" {
		s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, "Enter a new API key or remove the saved key, not both.")
		return
	}
	effectiveAPIKey := apiKey
	if s.cfg.APIKeyManaged {
		effectiveAPIKey = s.cfg.APIKey
	} else if removeAPIKey {
		effectiveAPIKey = ""
	} else if apiKey == "" {
		effectiveAPIKey = current.APIKey
	}
	expected, err := s.snapshot(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	target := current
	target.Provider, target.PublicURL, target.ServerURL, target.APIKey = string(provider), publicURL, serverURL, effectiveAPIKey
	update := s.settingsUpdate(target, apiKey != "" || removeAPIKey)
	published, err := s.connections.Publish(r.Context(), expected, target, update)
	if err != nil {
		s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, "Could not verify or save this connection. Refresh the page, check the details, and try again.")
		return
	}
	s.audit(r, session, "settings.update", "settings", "application", map[string]any{"provider": string(provider), "public_url": publicURL, "server_url": serverURL, "api_key_changed": update.APIKey != nil})
	if published.Identity.Generation != session.Generation {
		s.clearSession(w, r, session.ID)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
}

func (s *Server) setMediaViewData(data *viewData) {
	data.ServerName = s.serverName()
	data.ProviderManaged = s.cfg.ProviderManaged
	data.PublicURLManaged = s.cfg.PublicURLManaged
	data.ServerURLManaged = s.cfg.ServerURLManaged
	data.APIKeyManaged = s.cfg.APIKeyManaged
	data.CookieManaged = s.cfg.CookieManaged
}

func (s *Server) renderSettingsError(w http.ResponseWriter, r *http.Request, session db.Session, provider, publicURL, serverURL, message string) {
	data := s.data(r, session)
	data.Error = message
	data.Provider = provider
	data.PublicURL = publicURL
	data.ServerURL = serverURL
	s.setMediaViewData(&data)
	render(w, "settings", data)
}

func (s *Server) settingsUpdate(target db.Settings, replaceKey bool) db.ConnectionUpdate {
	var update db.ConnectionUpdate
	if !s.cfg.ProviderManaged {
		update.Provider = &target.Provider
	}
	if !s.cfg.PublicURLManaged {
		update.PublicURL = &target.PublicURL
	}
	if !s.cfg.ServerURLManaged {
		update.ServerURL = &target.ServerURL
	}
	if !s.cfg.APIKeyManaged && replaceKey {
		update.APIKey = &target.APIKey
	}
	return update
}
