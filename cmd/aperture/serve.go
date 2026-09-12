package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/httpserver"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	handler := httpserver.NewServer(cfg, store, router.New)
	if err := handler.Initialize(ctx); err != nil {
		return err
	}
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		handler.RunMaintenance(ctx)
	}()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      4 * time.Minute,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		handler.CloseAdmission()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Warn("HTTP shutdown deadline reached", "error", err)
			_ = srv.Close()
		}
	}()

	slog.Info("starting aperture", "addr", cfg.HTTPAddr, "db", cfg.DBPath)
	err = srv.ListenAndServe()
	stop()
	<-shutdownDone
	<-workerDone
	// Each accepted operation and webhook already has its own deadline. Closing
	// SQLite after a separate drain timeout could race their final state writes.
	if waitErr := handler.Drain(context.Background()); waitErr != nil {
		return waitErr
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
