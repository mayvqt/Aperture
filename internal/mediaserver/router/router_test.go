package router

import (
	"testing"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

func TestSetProviderSwitchesAdapters(t *testing.T) {
	server, err := New(mediaserver.ProviderJellyfin)
	if err != nil {
		t.Fatal(err)
	}
	if server.Provider() != mediaserver.ProviderJellyfin {
		t.Fatalf("provider = %q", server.Provider())
	}
	if err := server.SetProvider(mediaserver.ProviderEmby); err != nil {
		t.Fatal(err)
	}
	if server.Provider() != mediaserver.ProviderEmby {
		t.Fatalf("provider = %q", server.Provider())
	}
	if err := server.SetProvider("plex"); err == nil {
		t.Fatal("unsupported provider was accepted")
	}
	if server.Provider() != mediaserver.ProviderEmby {
		t.Fatal("failed switch changed the active provider")
	}
}

func TestNewRejectsUnsupportedProvider(t *testing.T) {
	if _, err := New("plex"); err == nil {
		t.Fatal("unsupported provider was accepted")
	}
}
