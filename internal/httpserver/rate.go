package httpserver

import (
	"sync"
	"time"
)

const maxRateLimiterEntries = 10_000

type rateLimiter struct {
	mu         sync.Mutex
	limit      int
	window     time.Duration
	attempts   map[string]rateEntry
	nextPrune  time.Time
	maxEntries int
}

type rateEntry struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:      limit,
		window:     window,
		attempts:   map[string]rateEntry{},
		nextPrune:  time.Now().Add(window),
		maxEntries: maxRateLimiterEntries,
	}
}

func (r *rateLimiter) Allow(key string) bool {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()

	entry := r.attempts[key]
	if entry.reset.IsZero() || now.After(entry.reset) {
		if !now.Before(r.nextPrune) {
			r.prune(now)
			r.nextPrune = now.Add(r.window)
		}
		if entry.reset.IsZero() && len(r.attempts) >= r.maxEntries {
			return false
		}
		r.attempts[key] = rateEntry{count: 1, reset: now.Add(r.window)}
		return true
	}
	if entry.count >= r.limit {
		return false
	}
	entry.count++
	r.attempts[key] = entry
	return true
}

func (r *rateLimiter) Reset(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.attempts, key)
}

func (r *rateLimiter) prune(now time.Time) {
	for key, entry := range r.attempts {
		if now.After(entry.reset) {
			delete(r.attempts, key)
		}
	}
}
