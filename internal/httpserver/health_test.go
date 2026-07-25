package httpserver

import (
	"context"
	"sync"
	"testing"
)

func TestMediaHealthCacheCoalescesConcurrentChecks(t *testing.T) {
	media := &fakeMediaServer{
		pingStarted: make(chan struct{}),
		pingRelease: make(chan struct{}),
	}
	s := &Server{media: media}

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
	s := &Server{media: media, provider: "jellyfin"}
	if !s.mediaHealthy(t.Context(), "http://media:8096", "api-key") {
		t.Fatal("initial health check failed")
	}

	media.pingErr = errFakePing
	s.setRuntime("emby", "", false)
	if s.mediaHealthy(t.Context(), "http://media:8096", "api-key") {
		t.Fatal("Emby health check reused Jellyfin cache entry")
	}
	if media.pingCalls.Load() != 2 {
		t.Fatalf("media-server ping calls = %d, want 2", media.pingCalls.Load())
	}
}
