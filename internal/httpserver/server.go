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
	auditRetention            = 90 * 24 * time.Hour
	auditPruneLimit           = 1000
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
	setupMu          sync.Mutex
	provider         string
	runtimePublicURL string
	cookieSecure     bool
	webhookWG        sync.WaitGroup
	accountMu        sync.Mutex
	activeAccounts   map[int64]bool
	accountWG        sync.WaitGroup
	httpWG           sync.WaitGroup
	closing          bool
	handler          http.Handler
}

func New(cfg config.Config, store Store, media MediaServer) http.Handler {
	handler, _ := NewWithShutdown(cfg, store, media)
	return handler
}

// NewWithShutdown returns the HTTP handler and a function that waits for
// accepted webhook deliveries to finish during graceful shutdown.
func NewWithShutdown(cfg config.Config, store Store, media MediaServer) (http.Handler, func(context.Context) error) {
	s := NewServer(cfg, store, media)
	return s, s.Drain
}

// NewServer owns HTTP, maintenance and accepted background deliveries together.
func NewServer(cfg config.Config, store Store, media MediaServer) *Server {
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
	s.handler = s.securityHeaders(s.mux)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.accountMu.Lock()
	if s.closing {
		s.accountMu.Unlock()
		http.Error(w, "Aperture is restarting. Try again shortly.", http.StatusServiceUnavailable)
		return
	}
	s.httpWG.Add(1)
	s.accountMu.Unlock()
	defer s.httpWG.Done()
	s.handler.ServeHTTP(w, r)
}

func (s *Server) CloseAdmission() {
	s.accountMu.Lock()
	s.closing = true
	s.accountMu.Unlock()
}

// Drain is called after the HTTP server and maintenance stop accepting work.
func (s *Server) Drain(ctx context.Context) error {
	s.CloseAdmission()
	done := make(chan struct{})
	go func() { s.httpWG.Wait(); s.accountWG.Wait(); close(done) }()
	select {
	case <-done:
		return s.waitForWebhooks(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
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

func (s *Server) RunMaintenance(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	s.runMaintenance(ctx)
	ticker := time.NewTicker(maintenanceInterval)
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
	s.processAndLogDueUserDisables(ctx)
	s.processDueTemplateRetries(ctx)
	s.pruneAuditLog(ctx)
}

func (s *Server) pruneAuditLog(ctx context.Context) {
	deleted, err := s.store.PruneAuditEvents(ctx, time.Now().Add(-auditRetention), auditPruneLimit)
	if err != nil {
		slog.Warn("could not prune audit log", "error", safeError(err))
		return
	}
	if deleted > 0 {
		slog.Info("pruned expired audit events", "deleted", deleted)
	}
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
