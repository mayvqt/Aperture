package httpserver

import (
	"context"
	"github.com/mayvqt/aperture/internal/connection"
	"sync"
	"testing"
)

func TestMediaHealthCacheCoalescesConcurrentChecks(t *testing.T) {
	media := &fakeMediaServer{
		pingStarted: make(chan struct{}),
		pingRelease: make(chan struct{}),
	}
	s := NewServer(testConfig(), newFakeStore(), testMediaFactory(media))

	const requests = 20
	results := make(chan bool, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.mediaHealthy(context.Background(), "http://media:8096", "api-key")
		}()
	}
	<-media.pingStarted
	close(media.pingRelease)
	wg.Wait()
	close(results)

	for healthy := range results {
		if !healthy {
			t.Fatal("concurrent health check unexpectedly failed")
		}
	}
	if media.pingCalls.Load() != 1 {
		t.Fatalf("concurrent media-server ping calls = %d, want 1", media.pingCalls.Load())
	}
}

func TestMediaHealthCacheSeparatesProviders(t *testing.T) {
	media := &fakeMediaServer{}
	s := NewServer(testConfig(), newFakeStore(), testMediaFactory(media))
	if !s.mediaHealthy(t.Context(), "http://media:8096", "api-key") {
		t.Fatal("initial health check failed")
	}

	media.pingErr = errFakePing
	snap, err := s.snapshot(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	snap.Settings.Provider = "emby"
	ctx := connection.WithSnapshot(t.Context(), snap)
	if s.mediaHealthy(ctx, "http://media:8096", "api-key") {
		t.Fatal("Emby health check reused Jellyfin cache entry")
	}
	if media.pingCalls.Load() != 2 {
		t.Fatalf("media-server ping calls = %d, want 2", media.pingCalls.Load())
	}
}
