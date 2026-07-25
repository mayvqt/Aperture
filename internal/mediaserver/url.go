package mediaserver

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

func NormalizeBaseURL(provider Provider, baseURL string) (string, error) {
	normalizedProvider, ok := ParseProvider(string(provider))
	if !ok {
		return "", fmt.Errorf("unsupported media provider %q", provider)
	}
	provider = normalizedProvider
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", errors.New("media-server URL is not configured")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", errors.New("media-server URL is missing a host")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("media-server URL must start with http:// or https://")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("media-server URL must not include credentials, query strings, or fragments")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	if provider == ProviderEmby && !strings.EqualFold(path.Base(u.Path), "emby") {
		u.Path += "/emby"
	}
	return strings.TrimRight(u.String(), "/"), nil
}
