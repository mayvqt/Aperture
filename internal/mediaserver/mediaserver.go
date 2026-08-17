package mediaserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mayvqt/aperture/internal/db"
)

type Provider string

const (
	ProviderJellyfin Provider = "jellyfin"
	ProviderEmby     Provider = "emby"
)

var ErrUserNotFound = errors.New("media-server user not found")

type Server interface {
	Authenticate(context.Context, string, string, string) (AuthResult, error)
	IsAdmin(context.Context, string, string, string, string) (bool, error)
	Ping(context.Context, string, string) error
	CreateUser(context.Context, string, string, string, string) (User, error)
	ApplyTemplate(context.Context, string, string, string, db.Template) error
	DisableUser(context.Context, string, string, string) error
	GetUser(context.Context, string, string, string) (User, bool, error)
	ListUsers(context.Context, string, string) ([]User, error)
	DeleteUser(context.Context, string, string, string) error
	ImportTemplate(context.Context, string, string, string, string) (TemplateData, error)
}

type AuthResult struct {
	UserID      string
	Username    string
	AccessToken string
	DeviceID    string
	IsAdmin     bool
}

type User struct {
	ID     string          `json:"Id"`
	Name   string          `json:"Name"`
	Policy json.RawMessage `json:"Policy"`
}

type Policy struct {
	IsAdministrator bool `json:"IsAdministrator"`
	IsDisabled      bool `json:"IsDisabled,omitempty"`
}

type TemplateData struct {
	PolicyJSON string
}

type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("media server returned HTTP %d (%s)", e.StatusCode, e.Status)
}

func ParseProvider(value string) (Provider, bool) {
	switch Provider(strings.ToLower(strings.TrimSpace(value))) {
	case ProviderJellyfin:
		return ProviderJellyfin, true
	case ProviderEmby:
		return ProviderEmby, true
	default:
		return "", false
	}
}

func (p Provider) Name() string {
	switch p {
	case ProviderJellyfin:
		return "Jellyfin"
	case ProviderEmby:
		return "Emby"
	default:
		return "Media server"
	}
}
