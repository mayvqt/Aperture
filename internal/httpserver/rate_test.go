package httpserver

import (
	"testing"
	"time"
)

func TestRateLimiterBlocksAfterLimit(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute)
	if !limiter.Allow("key") {
		t.Fatal("first attempt should be allowed")
	}
	if !limiter.Allow("key") {
		t.Fatal("second attempt should be allowed")
	}
	if limiter.Allow("key") {
		t.Fatal("third attempt should be blocked")
	}
}

func TestRateLimiterResetsWindow(t *testing.T) {
	limiter := newRateLimiter(1, time.Nanosecond)
	if !limiter.Allow("key") {
		t.Fatal("first attempt should be allowed")
	}
	time.Sleep(time.Millisecond)
	if !limiter.Allow("key") {
		t.Fatal("attempt after reset should be allowed")
	}
}

func TestRateLimiterResetClearsAttempts(t *testing.T) {
	limiter := newRateLimiter(1, time.Minute)
	if !limiter.Allow("key") {
		t.Fatal("first attempt should be allowed")
	}
	if limiter.Allow("key") {
		t.Fatal("second attempt should be blocked")
	}
	limiter.Reset("key")
	if !limiter.Allow("key") {
		t.Fatal("attempt after explicit reset should be allowed")
	}
}

func TestRateLimiterFailsClosedAtEntryLimit(t *testing.T) {
	limiter := newRateLimiter(1, time.Hour)
	limiter.maxEntries = 2
	if !limiter.Allow("one") || !limiter.Allow("two") {
		t.Fatal("limiter rejected entries before reaching capacity")
	}
	if limiter.Allow("three") {
		t.Fatal("limiter accepted a new key after reaching capacity")
	}
	if limiter.Allow("one") {
		t.Fatal("existing key bypassed its rate limit at capacity")
	}
}
