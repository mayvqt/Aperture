package router

import (
	"testing"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestNewCreatesIndependentAdapters(t *testing.T) {
	a, err := New(mediaserver.ProviderJellyfin)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(mediaserver.ProviderEmby)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("adapters share mutable state")
	}
}

func TestNewRejectsUnsupportedProvider(t *testing.T) {
	if _, err := New("plex"); err == nil {
		t.Fatal("unsupported provider was accepted")
	}
}
