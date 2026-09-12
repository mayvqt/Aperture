package router

import (
	"context"
	"fmt"
	"sync"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/mediaserver/emby"
	"github.com/mayvqt/aperture/internal/mediaserver/jellyfin"
)

type Server struct {
	mu       sync.RWMutex
	provider mediaserver.Provider
	active   mediaserver.Server
}

func New(provider mediaserver.Provider) (*Server, error) {
	s := &Server{}
	if err := s.SetProvider(provider); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) SetProvider(provider mediaserver.Provider) error {
	var active mediaserver.Server
	switch provider {
	case mediaserver.ProviderJellyfin:
		active = jellyfin.New()
	case mediaserver.ProviderEmby:
		active = emby.New()
	default:
		return fmt.Errorf("unsupported media provider %q", provider)
	}
	s.mu.Lock()
	s.provider = provider
	s.active = active
	s.mu.Unlock()
	return nil
}

func (s *Server) Provider() mediaserver.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

func (s *Server) current() mediaserver.Server {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

func (s *Server) Authenticate(ctx context.Context, baseURL, username, password string) (mediaserver.AuthResult, error) {
	return s.current().Authenticate(ctx, baseURL, username, password)
}
func (s *Server) IsAdmin(ctx context.Context, baseURL, token, deviceID, userID string) (bool, error) {
	return s.current().IsAdmin(ctx, baseURL, token, deviceID, userID)
}
func (s *Server) Ping(ctx context.Context, baseURL, apiKey string) error {
	return s.current().Ping(ctx, baseURL, apiKey)
}
func (s *Server) CreateUser(ctx context.Context, baseURL, apiKey, username, password string, created func(mediaserver.User) error) (mediaserver.User, error) {
	return s.current().CreateUser(ctx, baseURL, apiKey, username, password, created)
}
func (s *Server) ApplyTemplate(ctx context.Context, baseURL, apiKey, userID string, tmpl db.Template) error {
	return s.current().ApplyTemplate(ctx, baseURL, apiKey, userID, tmpl)
}
func (s *Server) DisableUser(ctx context.Context, baseURL, apiKey, userID string) error {
	return s.current().DisableUser(ctx, baseURL, apiKey, userID)
}
func (s *Server) GetUser(ctx context.Context, baseURL, apiKey, userID string) (mediaserver.User, bool, error) {
	return s.current().GetUser(ctx, baseURL, apiKey, userID)
}
func (s *Server) ListUsers(ctx context.Context, baseURL, apiKey string) ([]mediaserver.User, error) {
	return s.current().ListUsers(ctx, baseURL, apiKey)
}
func (s *Server) DeleteUser(ctx context.Context, baseURL, apiKey, userID string) error {
	return s.current().DeleteUser(ctx, baseURL, apiKey, userID)
}
func (s *Server) ImportTemplate(ctx context.Context, baseURL, token, deviceID, userRef string) (mediaserver.TemplateData, error) {
	return s.current().ImportTemplate(ctx, baseURL, token, deviceID, userRef)
}

var _ mediaserver.Server = (*Server)(nil)
