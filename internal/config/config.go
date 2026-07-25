package config

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mayvqt/aperture/internal/mediaserver"
)

const (
	MaxServerURLLength = 2048
	MaxAPIKeyLength    = 4096
)

type Config struct {
	HTTPAddr          string
	ConfigDir         string
	DBPath            string
	PublicURL         string
	LogLevel          string
	EncryptionKey     string
	SessionSecret     string
	InviteSecret      string
	CookieSecure      bool
	TrustedProxyCIDRs []string
	MediaProvider     string
	ServerURL         string
	APIKey            string
	PublicURLManaged  bool
	ProviderManaged   bool
	ServerURLManaged  bool
	APIKeyManaged     bool
	CookieManaged     bool
}

func Load(args []string) (Config, error) {
	publicURLManaged := os.Getenv("APERTURE_PUBLIC_URL") != "" || hasFlag(args, "public-url")
	cookieManaged := os.Getenv("APERTURE_COOKIE_SECURE") != "" || hasFlag(args, "cookie-secure")
	cookieSecure, err := envBool("APERTURE_COOKIE_SECURE", strings.HasPrefix(strings.TrimSpace(os.Getenv("APERTURE_PUBLIC_URL")), "https://"))
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		HTTPAddr:          env("APERTURE_HTTP_ADDR", ":8099"),
		ConfigDir:         env("APERTURE_CONFIG_DIR", "./data"),
		PublicURL:         os.Getenv("APERTURE_PUBLIC_URL"),
		LogLevel:          env("APERTURE_LOG_LEVEL", "info"),
		CookieSecure:      cookieSecure,
		TrustedProxyCIDRs: splitCSV(env("APERTURE_TRUSTED_PROXY_CIDRS", "127.0.0.1/32")),
		MediaProvider:     env("APERTURE_MEDIA_PROVIDER", string(mediaserver.ProviderJellyfin)),
		PublicURLManaged:  publicURLManaged,
		ProviderManaged:   os.Getenv("APERTURE_MEDIA_PROVIDER") != "" || hasFlag(args, "media-provider"),
		ServerURLManaged:  os.Getenv("APERTURE_SERVER_URL") != "" || hasFlag(args, "server-url"),
		APIKeyManaged:     os.Getenv("APERTURE_API_KEY") != "",
		CookieManaged:     cookieManaged,
	}
	dbPathExplicit := os.Getenv("APERTURE_DB_PATH") != "" || hasFlag(args, "db-path")
	cfg.DBPath = env("APERTURE_DB_PATH", filepath.Join(cfg.ConfigDir, "aperture.db"))
	cfg.SessionSecret = os.Getenv("APERTURE_SESSION_SECRET")
	cfg.InviteSecret = os.Getenv("APERTURE_INVITE_SECRET")
	cfg.EncryptionKey = os.Getenv("APERTURE_ENCRYPTION_KEY")
	cfg.ServerURL = os.Getenv("APERTURE_SERVER_URL")
	cfg.APIKey = os.Getenv("APERTURE_API_KEY")

	fs := flag.NewFlagSet("aperture", flag.ContinueOnError)
	fs.StringVar(&cfg.HTTPAddr, "http-addr", cfg.HTTPAddr, "HTTP listen address")
	fs.StringVar(&cfg.ConfigDir, "config-dir", cfg.ConfigDir, "config directory")
	fs.StringVar(&cfg.DBPath, "db-path", cfg.DBPath, "SQLite database path")
	fs.StringVar(&cfg.PublicURL, "public-url", cfg.PublicURL, "public URL for generated invite links")
	fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level")
	fs.BoolVar(&cfg.CookieSecure, "cookie-secure", cfg.CookieSecure, "set Secure on cookies")
	fs.StringVar(&cfg.MediaProvider, "media-provider", cfg.MediaProvider, "media-server provider: jellyfin or emby")
	fs.StringVar(&cfg.ServerURL, "server-url", cfg.ServerURL, "media-server URL")
	trustedProxyCIDRs := strings.Join(cfg.TrustedProxyCIDRs, ",")
	fs.StringVar(&trustedProxyCIDRs, "trusted-proxy-cidrs", trustedProxyCIDRs, "comma-separated trusted reverse proxy CIDRs")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	cfg.HTTPAddr = normalizeHTTPAddr(cfg.HTTPAddr)
	cfg.TrustedProxyCIDRs = splitCSV(trustedProxyCIDRs)
	for _, value := range cfg.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(value); err != nil {
			return Config{}, fmt.Errorf("invalid trusted proxy CIDR %q: %w", value, err)
		}
	}
	if !dbPathExplicit {
		cfg.DBPath = filepath.Join(cfg.ConfigDir, "aperture.db")
	}
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if cfg.PublicURL != "" {
		u, err := url.Parse(cfg.PublicURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return Config{}, fmt.Errorf("invalid public URL %q", cfg.PublicURL)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return Config{}, fmt.Errorf("public URL must start with http:// or https://")
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return Config{}, fmt.Errorf("public URL must not include credentials, query strings, or fragments")
		}
		if u.Path != "" && u.Path != "/" {
			return Config{}, fmt.Errorf("public URL must not include a path")
		}
		if u.Scheme == "http" && cfg.CookieSecure {
			return Config{}, fmt.Errorf("APERTURE_COOKIE_SECURE must be false when APERTURE_PUBLIC_URL uses http://")
		}
	}
	provider, ok := mediaserver.ParseProvider(cfg.MediaProvider)
	if !ok {
		return Config{}, fmt.Errorf("APERTURE_MEDIA_PROVIDER must be jellyfin or emby")
	}
	cfg.MediaProvider = string(provider)
	if cfg.ServerURL != "" {
		if len(cfg.ServerURL) > MaxServerURLLength {
			return Config{}, fmt.Errorf("APERTURE_SERVER_URL must be at most %d characters", MaxServerURLLength)
		}
		cfg.ServerURL, err = mediaserver.NormalizeBaseURL(provider, cfg.ServerURL)
		if err != nil {
			return Config{}, fmt.Errorf("invalid APERTURE_SERVER_URL: %w", err)
		}
	}
	if len(cfg.APIKey) > MaxAPIKeyLength {
		return Config{}, fmt.Errorf("APERTURE_API_KEY must be at most %d characters", MaxAPIKeyLength)
	}
	if cfg.LogLevel != "debug" && cfg.LogLevel != "info" && cfg.LogLevel != "warn" && cfg.LogLevel != "error" {
		return Config{}, fmt.Errorf("invalid log level %q", cfg.LogLevel)
	}
	if cfg.EncryptionKey != "" && len(cfg.EncryptionKey) < 32 {
		return Config{}, fmt.Errorf("APERTURE_ENCRYPTION_KEY must be at least 32 characters")
	}
	if cfg.SessionSecret != "" && len(cfg.SessionSecret) < 32 {
		return Config{}, fmt.Errorf("APERTURE_SESSION_SECRET must be at least 32 characters")
	}
	if cfg.InviteSecret != "" && len(cfg.InviteSecret) < 32 {
		return Config{}, fmt.Errorf("APERTURE_INVITE_SECRET must be at least 32 characters")
	}
	return cfg, nil
}

func envBool(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", name, err)
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func normalizeHTTPAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ":8099"
	}
	for _, r := range addr {
		if r < '0' || r > '9' {
			return addr
		}
	}
	return ":" + addr
}

func hasFlag(args []string, name string) bool {
	long := "--" + name
	short := "-" + name
	for _, arg := range args {
		if arg == long || arg == short || strings.HasPrefix(arg, long+"=") || strings.HasPrefix(arg, short+"=") {
			return true
		}
	}
	return false
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
