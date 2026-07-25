package httpserver

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"
)

const mediaHealthTTL = 30 * time.Second

type mediaHealthCache struct {
	mu        sync.Mutex
	key       [sha256.Size]byte
	checkedAt time.Time
	healthy   bool
}

func (s *Server) mediaHealthy(ctx context.Context, baseURL, apiKey string) bool {
	provider, _, _ := s.runtimeSettings()
	key := sha256.Sum256([]byte(provider + "\x00" + baseURL + "\x00" + apiKey))
	now := time.Now()

	s.healthCache.mu.Lock()
	defer s.healthCache.mu.Unlock()
	if s.healthCache.key == key && now.Sub(s.healthCache.checkedAt) < mediaHealthTTL {
		return s.healthCache.healthy
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	healthy := s.media.Ping(pingCtx, baseURL, apiKey) == nil
	if ctx.Err() == nil {
		s.healthCache.key = key
		s.healthCache.checkedAt = time.Now()
		s.healthCache.healthy = healthy
	}
	return healthy
}
