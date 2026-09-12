package httpserver

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/mayvqt/aperture/internal/db"
)

func TestMaintenanceWorkerReconcilesAndProcessesExpiryImmediately(t *testing.T) {
	store := newFakeStore()
	store.dueDisables = []db.Registration{{ID: 42, CleanupPending: true, ExternalUserID: sql.NullString{String: "expired-user", Valid: true}}}
	media := &fakeMediaServer{}
	ctx := context.Background()
	s := NewServer(testConfig(), store, media)
	s.runMaintenance(ctx)
	if !store.reconciledStale {
		t.Fatal("maintenance worker did not reconcile stale registrations")
	}
	if store.markedDisabledID != 42 {
		t.Fatalf("marked registration = %d, want 42", store.markedDisabledID)
	}
	if media.disabledUserID != "expired-user" {
		t.Fatalf("disabled user = %q, want expired-user", media.disabledUserID)
	}
	if !store.prunedAuditEvents {
		t.Fatal("maintenance worker did not prune expired audit events")
	}
}

func TestMaintenanceWorkerRecordsExpiredUserDisableFailure(t *testing.T) {
	store := newFakeStore()
	store.dueDisables = []db.Registration{{ID: 42, CleanupPending: true, ExternalUserID: sql.NullString{String: "expired-user", Valid: true}}}
	media := &fakeMediaServer{disableErr: errors.New("media unavailable")}
	ctx := context.Background()
	s := NewServer(testConfig(), store, media)
	s.runMaintenance(ctx)
	if store.markedDisabledID != 0 || store.failedDisableID != 42 {
		t.Fatalf("disabled/failed IDs = %d/%d", store.markedDisabledID, store.failedDisableID)
	}
}
