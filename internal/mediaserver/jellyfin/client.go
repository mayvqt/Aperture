package jellyfin

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/mayvqt/aperture/internal/mediaserver"
	"github.com/mayvqt/aperture/internal/mediaserver/protocol"
)

type Client struct {
	*protocol.Client
}

func New() *Client {
	return &Client{Client: protocol.New(authorization, normalizeURL)}
}

func NewWithHTTPClient(client *http.Client) *Client {
	return &Client{Client: protocol.NewWithHTTPClient(authorization, normalizeURL, client)}
}

var authorization = protocol.Authorization{
	Header:      "Authorization",
	Scheme:      "MediaBrowser",
	TokenInAuth: true,
}

func (c *Client) CreateUser(ctx context.Context, baseURL, apiKey, username, password string, created func(mediaserver.User) error) (mediaserver.User, error) {
	var user mediaserver.User
	if err := c.DoJSON(ctx, baseURL, http.MethodPost, "/Users/New", apiKey, map[string]string{
		"Name": username, "Password": password,
	}, &user); err != nil {
		return mediaserver.User{}, err
	}
	if strings.TrimSpace(user.ID) == "" {
		return mediaserver.User{}, errors.New("jellyfin create-user response was incomplete")
	}
	return user, created(user)
}

func normalizeURL(value string) (string, error) {
	return mediaserver.NormalizeBaseURL(mediaserver.ProviderJellyfin, value)
}

var _ mediaserver.Server = (*Client)(nil)
