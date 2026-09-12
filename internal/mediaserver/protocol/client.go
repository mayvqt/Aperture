package protocol

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

const maxResponseBodyBytes = 8 << 20

type Client struct {
	http         *http.Client
	auth         Authorization
	normalizeURL func(string) (string, error)
}

type Authorization struct {
	Header      string
	Scheme      string
	TokenHeader string
	TokenInAuth bool
}

func New(auth Authorization, normalizeURL func(string) (string, error)) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 20
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second
	return NewWithHTTPClient(auth, normalizeURL, &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
}

func NewWithHTTPClient(auth Authorization, normalizeURL func(string) (string, error), client *http.Client) *Client {
	return &Client{http: client, auth: auth, normalizeURL: normalizeURL}
}

func (c *Client) Authenticate(ctx context.Context, baseURL, username, password string) (mediaserver.AuthResult, error) {
	deviceID, err := randomDeviceID()
	if err != nil {
		return mediaserver.AuthResult{}, err
	}
	var out struct {
		AccessToken string `json:"AccessToken"`
		User        struct {
			ID     string             `json:"Id"`
			Name   string             `json:"Name"`
			Policy mediaserver.Policy `json:"Policy"`
		} `json:"User"`
	}
	err = c.DoJSONForDevice(ctx, baseURL, http.MethodPost, "/Users/AuthenticateByName", "", deviceID, map[string]string{
		"Username": username,
		"Pw":       password,
	}, &out)
	if err != nil {
		return mediaserver.AuthResult{}, err
	}
	if strings.TrimSpace(out.User.ID) == "" || strings.TrimSpace(out.User.Name) == "" || strings.TrimSpace(out.AccessToken) == "" {
		return mediaserver.AuthResult{}, errors.New("media-server authentication response was incomplete")
	}
	return mediaserver.AuthResult{
		UserID: out.User.ID, Username: out.User.Name, AccessToken: out.AccessToken,
		DeviceID: deviceID, IsAdmin: out.User.Policy.IsAdministrator,
	}, nil
}

func (c *Client) IsAdmin(ctx context.Context, baseURL, accessToken, deviceID, userID string) (bool, error) {
	var user struct {
		Policy mediaserver.Policy `json:"Policy"`
	}
	if err := c.DoJSONForDevice(ctx, baseURL, http.MethodGet, "/Users/"+url.PathEscape(userID), accessToken, deviceID, nil, &user); err != nil {
		var httpErr *mediaserver.HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
			return false, nil
		}
		return false, err
	}
	return user.Policy.IsAdministrator, nil
}

func (c *Client) Ping(ctx context.Context, baseURL, apiKey string) error {
	return c.DoJSON(ctx, baseURL, http.MethodGet, "/System/Info", apiKey, nil, nil)
}

func (c *Client) Inspect(ctx context.Context, baseURL, token, deviceID string) (mediaserver.ServerInfo, error) {
	var info mediaserver.ServerInfo
	if err := c.DoJSONForDevice(ctx, baseURL, http.MethodGet, "/System/Info", token, deviceID, nil, &info); err != nil {
		return mediaserver.ServerInfo{}, err
	}
	info.ID = strings.TrimSpace(info.ID)
	if info.ID == "" {
		return mediaserver.ServerInfo{}, errors.New("media-server identity response was incomplete")
	}
	return info, nil
}

func (c *Client) DisableUser(ctx context.Context, baseURL, apiKey, userID string) error {
	var user struct {
		Policy json.RawMessage `json:"Policy"`
	}
	if err := c.DoJSON(ctx, baseURL, http.MethodGet, "/Users/"+url.PathEscape(userID), apiKey, nil, &user); err != nil {
		return err
	}
	policy, err := mediaserver.MergeUserPolicy(user.Policy, "{}", true)
	if err != nil {
		return err
	}
	return c.DoJSON(ctx, baseURL, http.MethodPost, "/Users/"+url.PathEscape(userID)+"/Policy", apiKey, policy, nil)
}

func (c *Client) GetUser(ctx context.Context, baseURL, apiKey, userID string) (mediaserver.User, bool, error) {
	var user mediaserver.User
	err := c.DoJSON(ctx, baseURL, http.MethodGet, "/Users/"+url.PathEscape(userID), apiKey, nil, &user)
	if err == nil {
		return user, true, nil
	}
	var httpErr *mediaserver.HTTPError
	if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
		return mediaserver.User{}, false, nil
	}
	return mediaserver.User{}, false, err
}

func (c *Client) ListUsers(ctx context.Context, baseURL, apiKey string) ([]mediaserver.User, error) {
	var users []mediaserver.User
	if err := c.DoJSON(ctx, baseURL, http.MethodGet, "/Users", apiKey, nil, &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) DeleteUser(ctx context.Context, baseURL, apiKey, userID string) error {
	return c.DoJSON(ctx, baseURL, http.MethodDelete, "/Users/"+url.PathEscape(userID), apiKey, nil, nil)
}

func (c *Client) ApplyTemplate(ctx context.Context, baseURL, apiKey, userID string, tmpl db.Template) error {
	user, found, err := c.GetUser(ctx, baseURL, apiKey, userID)
	if err != nil {
		return err
	}
	if !found {
		return mediaserver.ErrUserNotFound
	}
	policy, err := mediaserver.MergeUserPolicy(user.Policy, tmpl.PolicyJSON, false)
	if err != nil {
		return err
	}
	return c.DoJSON(ctx, baseURL, http.MethodPost, "/Users/"+url.PathEscape(userID)+"/Policy", apiKey, policy, nil)
}

func (c *Client) ImportTemplate(ctx context.Context, baseURL, token, deviceID, userRef string) (mediaserver.TemplateData, error) {
	userID, err := c.resolveUserID(ctx, baseURL, token, deviceID, userRef)
	if err != nil {
		return mediaserver.TemplateData{}, err
	}
	var user mediaserver.User
	if err := c.DoJSONForDevice(ctx, baseURL, http.MethodGet, "/Users/"+url.PathEscape(userID), token, deviceID, nil, &user); err != nil {
		return mediaserver.TemplateData{}, err
	}
	policy := user.Policy
	if len(policy) == 0 || strings.TrimSpace(string(policy)) == "null" {
		policy = json.RawMessage(db.TemplatePolicyDefaultJSON)
	}
	return mediaserver.TemplateData{PolicyJSON: string(policy)}, nil
}

func (c *Client) DoJSON(ctx context.Context, baseURL, method, requestPath, apiKey string, body, out any) error {
	return c.DoJSONForDevice(ctx, baseURL, method, requestPath, apiKey, "aperture", body, out)
}

func (c *Client) DoJSONForDevice(ctx context.Context, baseURL, method, requestPath, apiKey, deviceID string, body, out any) error {
	baseURL, err := c.normalizeURL(baseURL)
	if err != nil {
		return err
	}
	endpoint, err := join(baseURL, requestPath)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	auth := fmt.Sprintf(`%s Client="Aperture", Device="Aperture", DeviceId="%s", Version="dev"`, c.auth.Scheme, url.QueryEscape(deviceID))
	if apiKey != "" && c.auth.TokenInAuth {
		auth += fmt.Sprintf(`, Token="%s"`, url.QueryEscape(apiKey))
	}
	req.Header.Set(c.auth.Header, auth)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" && c.auth.TokenHeader != "" {
		req.Header.Set(c.auth.TokenHeader, apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("send media-server request: %w", ctxErr)
		}
		return errors.New("send media-server request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		return &mediaserver.HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	if out == nil {
		read, err := io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes+1))
		if err != nil {
			return fmt.Errorf("read media-server response: %w", err)
		}
		if read > maxResponseBodyBytes {
			return errors.New("media-server response exceeded the maximum size")
		}
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read media-server response: %w", err)
	}
	if len(data) > maxResponseBodyBytes {
		return errors.New("media-server response exceeded the maximum size")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("media server returned an empty response")
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode media-server response: %w", err)
	}
	return nil
}

func (c *Client) resolveUserID(ctx context.Context, baseURL, token, deviceID, userRef string) (string, error) {
	userRef = strings.TrimSpace(userRef)
	if userRef == "" {
		return "", errors.New("media-server user is required")
	}
	var users []mediaserver.User
	if err := c.DoJSONForDevice(ctx, baseURL, http.MethodGet, "/Users", token, deviceID, nil, &users); err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	for _, user := range users {
		if strings.EqualFold(user.ID, userRef) || strings.EqualFold(strings.TrimSpace(user.Name), userRef) {
			return user.ID, nil
		}
	}
	return "", fmt.Errorf("%w: %q", mediaserver.ErrUserNotFound, userRef)
}

func join(baseURL, requestPath string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(requestPath)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + rel.Path
	u.RawQuery = rel.RawQuery
	return u.String(), nil
}

func randomDeviceID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
