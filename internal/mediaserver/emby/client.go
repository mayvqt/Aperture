package emby

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	Header:      "X-Emby-Authorization",
	Scheme:      "Emby",
	TokenHeader: "X-Emby-Token",
}

func (c *Client) CreateUser(ctx context.Context, baseURL, apiKey, username, password string) (mediaserver.User, error) {
	var user mediaserver.User
	if err := c.DoJSON(ctx, baseURL, http.MethodPost, "/Users/New", apiKey, map[string]string{
		"Name": username,
	}, &user); err != nil {
		return mediaserver.User{}, err
	}
	if strings.TrimSpace(user.ID) == "" {
		return mediaserver.User{}, errors.New("emby create-user response was incomplete")
	}
	err := c.DoJSON(ctx, baseURL, http.MethodPost, "/Users/"+url.PathEscape(user.ID)+"/Password", apiKey, map[string]any{
		"NewPw": password, "ResetPassword": false,
	}, nil)
	if err == nil {
		return user, nil
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if disableErr := c.DisableUser(cleanupCtx, baseURL, apiKey, user.ID); disableErr != nil {
		return user, fmt.Errorf("set Emby password: %w; disable incomplete account: %v", err, disableErr)
	}
	return user, fmt.Errorf("set Emby password (incomplete account disabled): %w", err)
}

func normalizeURL(value string) (string, error) {
	return mediaserver.NormalizeBaseURL(mediaserver.ProviderEmby, value)
}

var _ mediaserver.Server = (*Client)(nil)
