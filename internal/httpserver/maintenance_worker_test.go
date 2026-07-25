package httpserver

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/mayvqt/aperture/internal/config"
	"github.com/mayvqt/aperture/internal/db"
)

func TestMaintenanceWorkerReconcilesAndProcessesExpiryImmediately(t *testing.T) {
	store := newFakeStore()
	store.dueDisables = []db.Registration{{ID: 42, ExternalUserID: sql.NullString{String: "expired-user", Valid: true}}}
	media := &fakeMediaServer{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	runMaintenanceWorker(ctx, config.Config{}, store, media, time.Hour)
	if !store.reconciledStale {
		t.Fatal("maintenance worker did not reconcile stale registrations")
	}
	if store.markedDisabledID != 42 {
		t.Fatalf("marked registration = %d, want 42", store.markedDisabledID)
	}
	if media.disabledUserID != "expired-user" {
		t.Fatalf("disabled user = %q, want expired-user", media.disabledUserID)
	}
}
