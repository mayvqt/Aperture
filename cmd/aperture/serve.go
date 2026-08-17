package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/httpserver"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/mediaserver/router"
)

func serve(args []string) error {
	cfg, err := config.Load(args)
	if err != nil {
		return err
	}
	if err := config.EnsureEncryptionKey(&cfg); err != nil {
		return err
	}

	store, err := db.Open(cfg.DBPath, cfg.EncryptionKey)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.InitSchema(context.Background()); err != nil {
		return err
	}
	if err := store.EnsureRuntimeSecrets(context.Background(), cfg.SessionSecret, cfg.InviteSecret); err != nil {
		return err
	}
	settings, err := store.Settings(context.Background())
	if err != nil {
		return err
	}
	setupLogger(cfg.LogLevel,
		cfg.APIKey, cfg.EncryptionKey, cfg.SessionSecret, cfg.InviteSecret,
		settings.APIKey, settings.SessionSecret, settings.InviteSecret,
	)
	if cfg.ProviderManaged {
		if err := store.ValidateMediaProvider(context.Background(), cfg.MediaProvider); err != nil {
			return err
		}
	} else if provider, ok := mediaserver.ParseProvider(settings.Provider); ok {
		cfg.MediaProvider = string(provider)
	}
	if !cfg.PublicURLManaged && settings.PublicURL != "" {
		cfg.PublicURL = settings.PublicURL
	}
	if !cfg.CookieManaged {
		cfg.CookieSecure = strings.HasPrefix(cfg.PublicURL, "https://")
	}
	provider, _ := mediaserver.ParseProvider(cfg.MediaProvider)
	media, err := router.New(provider)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		httpserver.RunMaintenanceWorker(ctx, cfg, store, media)
	}()

	handler, waitForWebhooks := httpserver.NewWithShutdown(cfg, store, media)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	slog.Info("starting aperture", "addr", cfg.HTTPAddr, "db", cfg.DBPath)
	err = srv.ListenAndServe()
	stop()
	<-workerDone
	deliveryCtx, cancelDeliveries := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelDeliveries()
	if waitErr := waitForWebhooks(deliveryCtx); waitErr != nil {
		slog.Warn("webhook deliveries did not finish before shutdown", "error", waitErr)
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
