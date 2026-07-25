package httpserver

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

func (s *Server) setupForm(w http.ResponseWriter, r *http.Request) {
	complete, err := s.setupComplete(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if complete {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	csrf, err := s.anonymousCSRF(w, r)
	if err != nil {
		s.error(w, err)
		return
	}
	publicURL := settings.PublicURL
	if s.cfg.PublicURLManaged {
		publicURL = s.cfg.PublicURL
	}
	if publicURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		publicURL = scheme + "://" + r.Host
	}
	render(w, "setup", s.setupViewData(settings.Provider, publicURL, settings.ServerURL, csrf, ""))
}
func (s *Server) setupPost(w http.ResponseWriter, r *http.Request) {
	complete, err := s.setupComplete(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if complete {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !s.validAnonymousCSRF(r) {
		s.message(w, "Invalid request", "Refresh the page and try again.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The setup form could not be read.", http.StatusBadRequest)
		return
	}
	providerValue := strings.TrimSpace(r.FormValue("provider"))
	if s.cfg.ProviderManaged {
		providerValue = s.cfg.MediaProvider
	}
	provider, ok := mediaserver.ParseProvider(providerValue)
	if !ok {
		s.renderSetupError(w, r, providerValue, r.FormValue("public_url"), r.FormValue("server_url"), "Choose Jellyfin or Emby.")
		return
	}
	publicURL := strings.TrimSpace(r.FormValue("public_url"))
	if s.cfg.PublicURLManaged {
		publicURL = s.cfg.PublicURL
	}
	publicURL, err = validatePublicURL(publicURL)
	if err != nil {
		s.renderSetupError(w, r, string(provider), publicURL, r.FormValue("server_url"), err.Error())
		return
	}
	serverURL := strings.TrimSpace(r.FormValue("server_url"))
	if s.cfg.ServerURLManaged {
		serverURL = s.cfg.ServerURL
	}
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if s.cfg.APIKeyManaged {
		apiKey = s.cfg.APIKey
	}
	serverURL, err = validateServerURL(provider, serverURL)
	if err != nil {
		s.renderSetupError(w, r, string(provider), publicURL, serverURL, err.Error())
		return
	}
	if err := validateAPIKey(apiKey); err != nil {
		s.renderSetupError(w, r, string(provider), publicURL, serverURL, err.Error())
		return
	}
	previousProvider, _, _ := s.runtimeSettings()
	restoreProvider := func() {
		if previous, ok := mediaserver.ParseProvider(previousProvider); ok {
			_ = s.media.SetProvider(previous)
		}
	}
	if err := s.media.SetProvider(provider); err != nil {
		s.renderSetupError(w, r, string(provider), publicURL, serverURL, "Could not select that media server.")
		return
	}
	if apiKey != "" {
		if err := s.media.Ping(r.Context(), serverURL, apiKey); err != nil {
			restoreProvider()
			s.renderSetupError(w, r, string(provider), publicURL, serverURL, "Could not reach the media server with that API key.")
			return
		}
	}
	if err := s.store.UpdateSetupSettings(r.Context(), string(provider), publicURL, serverURL, apiKey); err != nil {
		restoreProvider()
		s.error(w, err)
		return
	}
	cookieSecure := strings.HasPrefix(publicURL, "https://")
	if s.cfg.CookieManaged {
		cookieSecure = s.cfg.CookieSecure
	}
	s.setRuntime(string(provider), publicURL, cookieSecure)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
func (s *Server) loginForm(w http.ResponseWriter, r *http.Request) {
	csrf, err := s.anonymousCSRF(w, r)
	if err != nil {
		s.error(w, err)
		return
	}
	render(w, "login", viewData{CSRF: csrf, ServerName: s.serverName()})
}
func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if !s.validAnonymousCSRF(r) {
		s.message(w, "Invalid request", "Refresh the page and try again.", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.message(w, "Invalid request", "The login form could not be read.", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	remoteIP := clientIP(r, s.trustedProxies)
	rateKey := "login:" + remoteIP + ":" + strings.ToLower(username)
	if !s.loginIPLimiter.Allow("login-ip:"+remoteIP) || !s.limiter.Allow(rateKey) {
		s.message(w, "Too many attempts", "Wait a few minutes and try again.", http.StatusTooManyRequests)
		return
	}
	settings, err := s.settings(r.Context())
	if err != nil {
		s.error(w, err)
		return
	}
	if len(username) > 128 || len(password) > maxPasswordLength {
		s.renderLoginFailure(w, r)
		return
	}
	auth, err := s.media.Authenticate(r.Context(), settings.ServerURL, username, password)
	if err != nil || !auth.IsAdmin {
		s.renderLoginFailure(w, r)
		return
	}
	s.limiter.Reset(rateKey)
	sessionID, _, err := s.store.CreateSession(r.Context(), auth.UserID, auth.Username, auth.AccessToken, auth.DeviceID, 24*time.Hour)
	if err != nil {
		s.error(w, err)
		return
	}
	http.SetCookie(w, s.sessionCookie(sessionID, 24*time.Hour))
	s.loginIPLimiter.Reset("login-ip:" + remoteIP)
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (s *Server) setupViewData(provider, publicURL, serverURL, csrf, message string) viewData {
	if provider == "" {
		provider = s.cfg.MediaProvider
	}
	if provider == "" {
		provider = string(mediaserver.ProviderJellyfin)
	}
	return viewData{
		Error:            message,
		Provider:         provider,
		PublicURL:        publicURL,
		ServerURL:        serverURL,
		CSRF:             csrf,
		ProviderManaged:  s.cfg.ProviderManaged,
		PublicURLManaged: s.cfg.PublicURLManaged,
		ServerURLManaged: s.cfg.ServerURLManaged,
		APIKeyManaged:    s.cfg.APIKeyManaged,
	}
}

func (s *Server) renderSetupError(w http.ResponseWriter, r *http.Request, provider, publicURL, serverURL, message string) {
	csrf, err := s.anonymousCSRF(w, r)
	if err != nil {
		s.error(w, err)
		return
	}
	render(w, "setup", s.setupViewData(provider, publicURL, serverURL, csrf, message))
}

func validatePublicURL(value string) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("enter the full URL used to open Aperture, without a path, query, or fragment")
	}
	return value, nil
}

func (s *Server) renderLoginFailure(w http.ResponseWriter, r *http.Request) {
	csrf, err := s.anonymousCSRF(w, r)
	if err != nil {
		s.error(w, err)
		return
	}
	render(w, "login", viewData{
		Error:      "Login failed or the media-server user is not an administrator.",
		CSRF:       csrf,
		ServerName: s.serverName(),
	})
}
func (s *Server) logoutPost(w http.ResponseWriter, r *http.Request) {
	session, ok, err := s.session(r)
	if err != nil {
		s.error(w, err)
		return
	}
	if !ok {
		http.SetCookie(w, s.sessionCookie("", -time.Hour))
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if !s.validCSRF(r, session.CSRFSecret) {
		s.message(w, "Invalid request", "Refresh the page and try again.", http.StatusBadRequest)
		return
	}
	if err := s.store.DeleteSession(r.Context(), session.ID); err != nil {
		slog.Warn("could not delete session during logout", "error", safeError(err))
	}
	http.SetCookie(w, s.sessionCookie("", -time.Hour))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
