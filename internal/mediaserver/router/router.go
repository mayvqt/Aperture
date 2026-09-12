package router

import (
	"fmt"
	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/mediaserver/emby"
	"github.com/mayvqt/aperture/internal/mediaserver/jellyfin"
)

// New returns an immutable provider adapter. Existing operations retain it when
// a different connection is published.
func New(provider mediaserver.Provider) (mediaserver.Server, error) {
	switch provider {
	case mediaserver.ProviderJellyfin:
		return jellyfin.New(), nil
	case mediaserver.ProviderEmby:
		return emby.New(), nil
	default:
		return nil, fmt.Errorf("unsupported media provider %q", provider)
	}
}
