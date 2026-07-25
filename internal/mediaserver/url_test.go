package mediaserver

import "testing"

func TestNormalizeBaseURLByProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		input    string
		want     string
	}{
		{"jellyfin", ProviderJellyfin, "http://jellyfin:8096/base/", "http://jellyfin:8096/base"},
		{"emby root", ProviderEmby, "http://emby:8096", "http://emby:8096/emby"},
		{"emby existing path", ProviderEmby, "http://emby:8096/emby/", "http://emby:8096/emby"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.provider, tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeBaseURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeBaseURLRejectsUnsafeParts(t *testing.T) {
	for _, value := range []string{"ftp://media", "https://user:pass@media", "https://media/?key=value", "https:///missing-host"} {
		t.Run(value, func(t *testing.T) {
			if _, err := NormalizeBaseURL(ProviderJellyfin, value); err == nil {
				t.Fatal("unsafe URL was accepted")
			}
		})
	}
}

func TestNormalizeBaseURLRejectsUnsupportedProvider(t *testing.T) {
	if _, err := NormalizeBaseURL("plex", "http://media:8096"); err == nil {
		t.Fatal("unsupported provider was accepted")
	}
}
