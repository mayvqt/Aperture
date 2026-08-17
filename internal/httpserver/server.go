package httpserver

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mayvqt/aperture/internal/config"
)

const (
	maintenanceInterval       = time.Minute
	staleRegistrationAge      = 15 * time.Minute
	maintenanceReconcileLimit = 100
)

type Server struct {
	cfg              config.Config
	store            Store
	media            MediaServer
	mux              *http.ServeMux
	limiter          *rateLimiter
	loginIPLimiter   *rateLimiter
	healthCache      mediaHealthCache
	trustedProxies   []*net.IPNet
	hstsHost         string
	runtimeMu        sync.RWMutex
	provider         string
	runtimePublicURL string
	cookieSecure     bool
	webhookWG        sync.WaitGroup
}

func New(cfg config.Config, store Store, media MediaServer) http.Handler {
	handler, _ := NewWithShutdown(cfg, store, media)
	return handler
}

// NewWithShutdown returns the HTTP handler and a function that waits for
// accepted webhook deliveries to finish during graceful shutdown.
func NewWithShutdown(cfg config.Config, store Store, media MediaServer) (http.Handler, func(context.Context) error) {
	s := &Server{
		cfg:              cfg,
		store:            store,
		media:            media,
		mux:              http.NewServeMux(),
		limiter:          newRateLimiter(10, 10*time.Minute),
		loginIPLimiter:   newRateLimiter(50, 10*time.Minute),
		trustedProxies:   parseTrustedProxies(cfg.TrustedProxyCIDRs),
		hstsHost:         securePublicHost(cfg.PublicURL),
		provider:         cfg.MediaProvider,
		runtimePublicURL: cfg.PublicURL,
		cookieSecure:     cfg.CookieSecure,
	}
	s.routes()
	return s.securityHeaders(s.mux), s.waitForWebhooks
}

func (s *Server) waitForWebhooks(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.webhookWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) setRuntime(provider, publicURL string, cookieSecure bool) {
	s.runtimeMu.Lock()
	s.provider = provider
	s.runtimePublicURL = publicURL
	s.cookieSecure = cookieSecure
	s.hstsHost = securePublicHost(publicURL)
	s.runtimeMu.Unlock()
}

func (s *Server) runtimeSettings() (provider, publicURL string, cookieSecure bool) {
	s.runtimeMu.RLock()
	defer s.runtimeMu.RUnlock()
	return s.provider, s.runtimePublicURL, s.cookieSecure
}

func RunMaintenanceWorker(ctx context.Context, cfg config.Config, store Store, media MediaServer) {
	runMaintenanceWorker(ctx, cfg, store, media, maintenanceInterval)
}

func runMaintenanceWorker(ctx context.Context, cfg config.Config, store Store, media MediaServer, interval time.Duration) {
	s := &Server{cfg: cfg, store: store, media: media}
	s.runMaintenance(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runMaintenance(ctx)
		}
	}
}

func (s *Server) runMaintenance(ctx context.Context) {
	s.reconcileAndLogStaleRegistrations(ctx)
	s.processDueTemplateRetries(ctx)
	s.processAndLogDueUserDisables(ctx)
}

func (s *Server) reconcileAndLogStaleRegistrations(ctx context.Context) {
	result, err := s.store.ReconcileStaleRegistrations(ctx, time.Now().Add(-staleRegistrationAge), maintenanceReconcileLimit)
	if err != nil {
		slog.Warn("could not reconcile stale registrations", "error", safeError(err))
		return
	}
	if result.ReleasedReservations > 0 || result.FlaggedAmbiguous > 0 {
		slog.Info("reconciled stale registrations",
			"released_reservations", result.ReleasedReservations,
			"flagged_ambiguous", result.FlaggedAmbiguous)
	}
}

func (s *Server) processAndLogDueUserDisables(ctx context.Context) {
	if disabled := s.processDueUserDisables(ctx); disabled > 0 {
		slog.Info("processed expired media-server users", "disabled", disabled)
	}
}
