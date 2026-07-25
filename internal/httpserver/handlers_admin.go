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
	data.Settings = settings
	s.setMediaViewData(&data)
	render(w, "settings", data)
}
func (s *Server) settingsPost(w http.ResponseWriter, r *http.Request, session db.Session) {
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
	previousProvider, _, _ := s.runtimeSettings()
	restoreProvider := func() {
		if previous, ok := mediaserver.ParseProvider(previousProvider); ok {
			_ = s.media.SetProvider(previous)
		}
	}
	if err := s.media.SetProvider(provider); err != nil {
		s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, "Could not select that media server.")
		return
	}
	if effectiveAPIKey != "" {
		if err := s.media.Ping(r.Context(), serverURL, effectiveAPIKey); err != nil {
			restoreProvider()
			s.renderSettingsError(w, r, session, string(provider), publicURL, serverURL, "Could not reach the media server with those connection details.")
			return
		}
	}
	var providerUpdate, publicURLUpdate, serverURLUpdate, apiKeyUpdate *string
	if !s.cfg.ProviderManaged {
		value := string(provider)
		providerUpdate = &value
	}
	if !s.cfg.PublicURLManaged {
		publicURLUpdate = &publicURL
	}
	if !s.cfg.ServerURLManaged {
		serverURLUpdate = &serverURL
	}
	if !s.cfg.APIKeyManaged && (apiKey != "" || removeAPIKey) {
		apiKeyUpdate = &apiKey
	}
	if providerUpdate != nil || publicURLUpdate != nil || serverURLUpdate != nil || apiKeyUpdate != nil {
		if err := s.store.UpdateApplicationSettings(r.Context(), providerUpdate, publicURLUpdate, serverURLUpdate, apiKeyUpdate); err != nil {
			restoreProvider()
			s.error(w, err)
			return
		}
	}
	cookieSecure := strings.HasPrefix(publicURL, "https://")
	if s.cfg.CookieManaged {
		cookieSecure = s.cfg.CookieSecure
	}
	s.setRuntime(string(provider), publicURL, cookieSecure)
	if current.Provider != string(provider) || current.ServerURL != serverURL {
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
	data.Settings = db.Settings{Provider: provider, PublicURL: publicURL, ServerURL: serverURL}
	s.setMediaViewData(&data)
	render(w, "settings", data)
}
