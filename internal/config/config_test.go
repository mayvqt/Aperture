package config

import (
	"path/filepath"
	"strings"
	"testing"
)

const testSecret = "test-secret-value-with-at-least-32-characters"

func TestLoadAllowsBrowserBootstrapWithoutSecurityEnvironment(t *testing.T) {
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "" || cfg.EncryptionKey != "" || cfg.SessionSecret != "" || cfg.InviteSecret != "" {
		t.Fatalf("bootstrap configuration unexpectedly populated: %#v", cfg)
	}
}

func TestLoadUsesConfigDirForDefaultDBPath(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	cfg, err := Load([]string{"--config-dir", "/tmp/aperture-test"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/tmp/aperture-test", "aperture.db")
	if cfg.DBPath != want {
		t.Fatalf("DBPath = %q, want %q", cfg.DBPath, want)
	}
}

func TestLoadKeepsExplicitDBPathWhenConfigDirChanges(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	cfg, err := Load([]string{"--config-dir", "/tmp/ignored", "--db-path", "/data/aperture.db"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBPath != "/data/aperture.db" {
		t.Fatalf("DBPath = %q, want explicit path", cfg.DBPath)
	}
}

func TestLoadNormalizesBareHTTPPort(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	cfg, err := Load([]string{"--http-addr", "8099"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8099" {
		t.Fatalf("HTTPAddr = %q, want :8099", cfg.HTTPAddr)
	}
}

func TestLoadKeepsHostHTTPAddr(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	cfg, err := Load([]string{"--http-addr", "0.0.0.0:8099"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != "0.0.0.0:8099" {
		t.Fatalf("HTTPAddr = %q, want host address", cfg.HTTPAddr)
	}
}

func TestLoadValidatesPublicURLAndLogLevel(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "not a url")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid public URL to fail")
	}

	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_LOG_LEVEL", "chatty")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid log level to fail")
	}
}

func TestLoadDoesNotIncludeInvalidPublicURLInError(t *testing.T) {
	const secretURL = "https://example.test/\n?api_key=startup-secret"
	t.Setenv("APERTURE_PUBLIC_URL", secretURL)

	_, err := Load(nil)
	if err == nil {
		t.Fatal("expected invalid public URL to fail")
	}
	if strings.Contains(err.Error(), "startup-secret") || strings.Contains(err.Error(), secretURL) {
		t.Fatalf("configuration error exposed URL contents: %q", err)
	}
}

func TestLoadRejectsUnsafePublicURLParts(t *testing.T) {
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)
	for _, value := range []string{
		"ftp://aperture.example",
		"https://user:pass@aperture.example",
		"https://aperture.example?next=/admin",
		"https://aperture.example/#admin",
		"https://aperture.example/aperture",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APERTURE_PUBLIC_URL", value)
			if _, err := Load(nil); err == nil {
				t.Fatal("expected public URL to fail validation")
			}
		})
	}
}

func TestLoadValidatesAndNormalizesJellyfinURL(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	t.Setenv("APERTURE_SERVER_URL", " https://media.example/base/ ")
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServerURL != "https://media.example/base" {
		t.Fatalf("ServerURL = %q, want normalized URL", cfg.ServerURL)
	}

	for _, value := range []string{
		"jellyfin.example",
		"ftp://jellyfin.example",
		"https://user:pass@jellyfin.example",
		"https://jellyfin.example/?api_key=secret",
		"https://jellyfin.example/#fragment",
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APERTURE_SERVER_URL", value)
			if _, err := Load(nil); err == nil {
				t.Fatal("expected invalid Jellyfin URL to fail")
			}
		})
	}
}

func TestLoadNormalizesEmbyURLAndRejectsUnknownProvider(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)
	t.Setenv("APERTURE_MEDIA_PROVIDER", "emby")
	t.Setenv("APERTURE_SERVER_URL", "http://emby:8096")

	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MediaProvider != "emby" || cfg.ServerURL != "http://emby:8096/emby" {
		t.Fatalf("provider = %q, URL = %q", cfg.MediaProvider, cfg.ServerURL)
	}

	t.Setenv("APERTURE_MEDIA_PROVIDER", "plex")
	if _, err := Load(nil); err == nil {
		t.Fatal("unknown media provider was accepted")
	}
}

func TestLoadTrimsPublicURL(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", " https://aperture.example/ ")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)
	cfg, err := Load(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicURL != "https://aperture.example" {
		t.Fatalf("PublicURL = %q", cfg.PublicURL)
	}
}

func TestLoadDerivesSecureCookiesFromFlagPublicURL(t *testing.T) {
	cfg, err := Load([]string{"--public-url", "https://aperture.example"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CookieSecure {
		t.Fatal("CookieSecure = false, want true for an HTTPS public URL")
	}
}

func TestLoadRejectsInvalidSecurityConfiguration(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)

	t.Setenv("APERTURE_COOKIE_SECURE", "sometimes")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid cookie boolean to fail")
	}
	t.Setenv("APERTURE_COOKIE_SECURE", "true")
	t.Setenv("APERTURE_TRUSTED_PROXY_CIDRS", "not-a-network")
	if _, err := Load(nil); err == nil {
		t.Fatal("expected invalid trusted proxy CIDR to fail")
	}

	t.Setenv("APERTURE_TRUSTED_PROXY_CIDRS", "127.0.0.1/32")
	t.Setenv("APERTURE_PUBLIC_URL", "http://aperture.example")
	if _, err := Load(nil); err == nil || !strings.Contains(err.Error(), "APERTURE_COOKIE_SECURE") {
		t.Fatalf("insecure public URL with secure cookies error = %v", err)
	}
	t.Setenv("APERTURE_COOKIE_SECURE", "false")
	if _, err := Load(nil); err != nil {
		t.Fatalf("plain HTTP local configuration failed: %v", err)
	}
}

func TestLoadRejectsOversizedJellyfinConfiguration(t *testing.T) {
	t.Setenv("APERTURE_PUBLIC_URL", "https://aperture.example")
	t.Setenv("APERTURE_ENCRYPTION_KEY", testSecret)
	t.Setenv("APERTURE_SESSION_SECRET", testSecret)
	t.Setenv("APERTURE_INVITE_SECRET", testSecret)
	t.Setenv("APERTURE_API_KEY", strings.Repeat("x", MaxAPIKeyLength+1))

	if _, err := Load(nil); err == nil {
		t.Fatal("expected oversized Jellyfin API key to fail")
	}
}

func TestHasFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "long", args: []string{"--db-path", "/data/app.db"}, want: true},
		{name: "long equals", args: []string{"--db-path=/data/app.db"}, want: true},
		{name: "short", args: []string{"-db-path", "/data/app.db"}, want: true},
		{name: "missing", args: []string{"--config-dir", "/data"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasFlag(tt.args, "db-path"); got != tt.want {
				t.Fatalf("hasFlag() = %v, want %v", got, tt.want)
			}
		})
	}
}
