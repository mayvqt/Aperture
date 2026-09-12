// Package connection owns effective configuration and immutable media-server
// snapshots. Publishing settings never mutates an adapter already in use.
package connection

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

var ErrUnavailable = errors.New("media-server identity is unavailable")

type Factory func(mediaserver.Provider) (mediaserver.Server, error)

type Store interface {
	Settings(context.Context) (db.Settings, error)
	MediaConnection(context.Context) (db.MediaConnection, error)
	PublishMediaConnection(context.Context, db.ConnectionUpdate) (db.MediaConnection, error)
}

type Snapshot struct {
	Settings     db.Settings
	Identity     db.MediaConnection
	Media        mediaserver.Server
	CookieSecure bool
	Verified     bool
	version      uint64
}

type Manager struct {
	cfg      config.Config
	store    Store
	factory  Factory
	updateMu sync.Mutex
	mu       sync.RWMutex
	current  Snapshot
	revision uint64
}

func New(cfg config.Config, store Store, factory Factory) *Manager {
	return &Manager{cfg: cfg, store: store, factory: factory}
}

// Current resolves environment overrides before deciding whether saved session
// ownership still applies. It makes no upstream requests.
func (m *Manager) Current(ctx context.Context) (Snapshot, error) {
	m.mu.RLock()
	current := m.current
	m.mu.RUnlock()
	if current.version != 0 {
		return current, nil
	}
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	m.mu.RLock()
	current = m.current
	m.mu.RUnlock()
	if current.version != 0 {
		return current, nil
	}
	settings, err := m.store.Settings(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	settings = m.effective(settings)
	provider, ok := mediaserver.ParseProvider(settings.Provider)
	if !ok {
		return Snapshot{}, errors.New("unsupported media provider")
	}
	if settings.ServerURL != "" {
		settings.ServerURL, err = mediaserver.NormalizeBaseURL(provider, settings.ServerURL)
		if err != nil {
			return Snapshot{}, err
		}
	}
	media, err := m.factory(provider)
	if err != nil {
		return Snapshot{}, err
	}
	identity, err := m.store.MediaConnection(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	if identity.Provider != settings.Provider || identity.BaseURL != settings.ServerURL {
		identity, err = m.store.PublishMediaConnection(ctx, db.ConnectionUpdate{Origin: db.MediaBinding{Provider: settings.Provider, BaseURL: settings.ServerURL}, ExpectedGeneration: identity.Generation})
		if err != nil {
			return Snapshot{}, err
		}
	}
	m.revision++
	current = Snapshot{Settings: settings, Identity: identity, Media: media, CookieSecure: m.cookieSecure(settings.PublicURL), version: m.revision}
	m.replace(current)
	return current, nil
}

func (m *Manager) effective(s db.Settings) db.Settings {
	if m.cfg.ProviderManaged || s.Provider == "" {
		s.Provider = m.cfg.MediaProvider
	}
	if s.Provider == "" {
		s.Provider = string(mediaserver.ProviderJellyfin)
	}
	if m.cfg.PublicURLManaged || s.PublicURL == "" {
		s.PublicURL = m.cfg.PublicURL
	}
	if m.cfg.ServerURLManaged {
		s.ServerURL = m.cfg.ServerURL
	}
	if m.cfg.APIKeyManaged || m.cfg.APIKey != "" {
		s.APIKey = m.cfg.APIKey
	}
	if m.cfg.SessionSecret != "" {
		s.SessionSecret = m.cfg.SessionSecret
	}
	if m.cfg.InviteSecret != "" {
		s.InviteSecret = m.cfg.InviteSecret
	}
	return s
}

func (m *Manager) cookieSecure(publicURL string) bool {
	if m.cfg.CookieManaged {
		return m.cfg.CookieSecure
	}
	return strings.HasPrefix(publicURL, "https://")
}

func (m *Manager) replace(s Snapshot) { m.mu.Lock(); m.current = s; m.mu.Unlock() }
func (m *Manager) Peek() (Snapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current, m.current.version != 0
}

// Verify checks the configured server for this operation. A previously saved
// binding alone cannot authorize changes after a server replacement.
func (m *Manager) Verify(ctx context.Context, s Snapshot, token, deviceID string) (Snapshot, error) {
	if s.Settings.ServerURL == "" || token == "" {
		return Snapshot{}, ErrUnavailable
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	info, err := s.Media.Inspect(checkCtx, s.Settings.ServerURL, token, deviceID)
	if err != nil {
		return Snapshot{}, err
	}
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	m.mu.RLock()
	current := m.current
	m.mu.RUnlock()
	if current.version != s.version {
		return Snapshot{}, db.ErrConnectionChanged
	}
	if current.Identity.Binding.ID == 0 || current.Identity.Binding.ServerID != info.ID {
		identity, err := m.store.PublishMediaConnection(ctx, db.ConnectionUpdate{Origin: db.MediaBinding{Provider: s.Settings.Provider, BaseURL: s.Settings.ServerURL, ServerID: info.ID, Name: info.Name}, ExpectedGeneration: s.Identity.Generation})
		if err != nil {
			m.replace(Snapshot{})
			return Snapshot{}, err
		}
		current.Identity = identity
		m.revision++
		current.version = m.revision
		m.replace(current)
	}
	current.Verified = true
	return current, nil
}

// Publish validates a candidate independently before atomically making its
// settings and ownership current. Operations already accepted retain their copy.
func (m *Manager) Publish(ctx context.Context, expected Snapshot, target db.Settings, u db.ConnectionUpdate) (Snapshot, error) {
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	m.mu.RLock()
	current := m.current
	m.mu.RUnlock()
	if current.version != expected.version {
		return Snapshot{}, db.ErrConnectionChanged
	}
	target = m.effective(target)
	provider, ok := mediaserver.ParseProvider(target.Provider)
	if !ok {
		return Snapshot{}, errors.New("unsupported media provider")
	}
	baseURL, err := mediaserver.NormalizeBaseURL(provider, target.ServerURL)
	if err != nil {
		return Snapshot{}, err
	}
	target.ServerURL = baseURL
	media, err := m.factory(provider)
	if err != nil {
		return Snapshot{}, err
	}
	origin := db.MediaBinding{Provider: target.Provider, BaseURL: baseURL}
	if target.APIKey != "" {
		checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		info, err := media.Inspect(checkCtx, baseURL, target.APIKey, "aperture")
		cancel()
		if err != nil {
			return Snapshot{}, err
		}
		origin.ServerID, origin.Name = info.ID, info.Name
	} else if current.Settings.Provider == target.Provider && current.Settings.ServerURL == baseURL {
		origin = current.Identity.Binding
		origin.Provider, origin.BaseURL = target.Provider, baseURL
	}
	u.Origin = origin
	u.ExpectedGeneration = current.Identity.Generation
	identity, err := m.store.PublishMediaConnection(ctx, u)
	if err != nil {
		m.replace(Snapshot{})
		return Snapshot{}, err
	}
	m.revision++
	updated := Snapshot{Settings: target, Identity: identity, Media: media, CookieSecure: m.cookieSecure(target.PublicURL), version: m.revision}
	m.replace(updated)
	return updated, nil
}

type snapshotKey struct{}

func WithSnapshot(ctx context.Context, s Snapshot) context.Context {
	return context.WithValue(ctx, snapshotKey{}, s)
}
func FromContext(ctx context.Context) (Snapshot, bool) {
	s, ok := ctx.Value(snapshotKey{}).(Snapshot)
	return s, ok
}
