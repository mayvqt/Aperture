package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/security"
)

func (s *Server) validInvite(ctx context.Context, token string) (db.Invite, error) {
	settings, err := s.settings(ctx)
	if err != nil {
		return db.Invite{}, err
	}
	invite, err := s.store.InviteByHash(ctx, security.HashToken(settings.InviteSecret, token))
	if err != nil {
		return db.Invite{}, err
	}
	now := time.Now()
	if !invite.Enabled || invite.DeletedAt.Valid || invite.Uses >= invite.MaxUses || (invite.ExpiresAt.Valid && invite.ExpiresAt.Time.Before(now)) {
		return db.Invite{}, db.ErrInviteUnavailable
	}
	return invite, nil
}
func (s *Server) serverName() string {
	value, _, _ := s.runtimeSettings()
	provider, _ := mediaserver.ParseProvider(value)
	return provider.Name()
}
func (s *Server) settings(ctx context.Context) (db.Settings, error) {
	settings, err := s.store.Settings(ctx)
	if err != nil {
		return db.Settings{}, err
	}
	if s.cfg.ProviderManaged {
		settings.Provider = s.cfg.MediaProvider
	}
	if s.cfg.PublicURLManaged {
		settings.PublicURL = s.cfg.PublicURL
	}
	if s.cfg.ServerURLManaged {
		settings.ServerURL = s.cfg.ServerURL
	}
	if s.cfg.APIKey != "" {
		settings.APIKey = s.cfg.APIKey
	}
	if s.cfg.SessionSecret != "" {
		settings.SessionSecret = s.cfg.SessionSecret
	}
	if s.cfg.InviteSecret != "" {
		settings.InviteSecret = s.cfg.InviteSecret
	}
	return settings, nil
}
func (s *Server) setupComplete(ctx context.Context) (bool, error) {
	settings, err := s.settings(ctx)
	if err != nil {
		return false, err
	}
	_, publicURL, _ := s.runtimeSettings()
	return settings.Provider != "" && settings.ServerURL != "" && publicURL != "", nil
}
func (s *Server) message(w http.ResponseWriter, title, message string, status int) {
	renderStatus(w, "message", viewData{AuthTitle: "Message · Aperture", Title: title, Message: message}, status)
}
func (s *Server) error(w http.ResponseWriter, err error) {
	slog.Error("request failed", "error", safeError(err))
	s.message(w, "Something went wrong", "Aperture could not complete that request.", http.StatusInternalServerError)
}
func (s *Server) validateServerURL(value string) (string, error) {
	providerValue, _, _ := s.runtimeSettings()
	provider, _ := mediaserver.ParseProvider(providerValue)
	return validateServerURL(provider, value)
}
func validateServerURL(provider mediaserver.Provider, value string) (string, error) {
	if len(value) > config.MaxServerURLLength {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return "", errors.New("Server URL is too long.")
	}
	normalized, err := mediaserver.NormalizeBaseURL(provider, value)
	if err != nil {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return "", errors.New("Enter a valid server URL without credentials, query strings, or fragments.")
	}
	return normalized, nil
}
func requestUserAgent(r *http.Request) string {
	value := []rune(strings.TrimSpace(r.UserAgent()))
	if len(value) > maxUserAgentLength {
		value = value[:maxUserAgentLength]
	}
	return string(value)
}
func idFromPath(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}
func clientIP(r *http.Request, trustedProxies []*net.IPNet) string {
	remote := directClientIP(r.RemoteAddr)
	if trustedProxy(remote, trustedProxies) {
		if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
			if ip := forwardedClientIP(forwarded, trustedProxies); ip != "" {
				return ip
			}
		}
		if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			if ip := cleanClientIP(realIP); ip != "" {
				return ip
			}
		}
	}
	return remote
}

func forwardedClientIP(value string, trustedProxies []*net.IPNet) string {
	parts := strings.Split(value, ",")
	var leftmost string
	for i := len(parts) - 1; i >= 0; i-- {
		ip := cleanClientIP(parts[i])
		if ip == "" {
			return ""
		}
		leftmost = ip
		if !trustedProxy(ip, trustedProxies) {
			return ip
		}
	}
	return leftmost
}

func directClientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		if ip := cleanClientIP(remoteAddr); ip != "" {
			return ip
		}
		return "unknown"
	}
	if ip := cleanClientIP(host); ip != "" {
		return ip
	}
	return "unknown"
}

func parseTrustedProxies(cidrs []string) []*net.IPNet {
	networks := make([]*net.IPNet, 0, len(cidrs))
	for _, value := range cidrs {
		if _, network, err := net.ParseCIDR(value); err == nil {
			networks = append(networks, network)
		}
	}
	return networks
}

func trustedProxy(ip string, networks []*net.IPNet) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, network := range networks {
		if network.Contains(parsed) {
			return true
		}
	}
	return false
}
func cleanClientIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return ""
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	msg := security.RedactText(err.Error())
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return msg
}
func safeAdminImportError(err error) string {
	if err == nil {
		return "Aperture could not import that media-server user's template."
	}
	msg := safeError(err)
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "403"):
		return "The media server rejected the import request. Save an API key in settings or log in again with an administrator account."
	case errors.Is(err, mediaserver.ErrUserNotFound):
		return "That media-server user was not found. Enter the username or user ID exactly as it appears on the media server."
	case strings.Contains(msg, "list users"):
		return "Aperture could not list media-server users. Check the server URL and administrator/API-key permissions."
	default:
		return "Aperture could not import that media-server user's template: " + msg
	}
}
