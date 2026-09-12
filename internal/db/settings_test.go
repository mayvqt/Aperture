package db

import (
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestSecretSettingsAreEncryptedAtRest(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.SetSetting(ctx, "api_key", "plain-secret", true); err != nil {
		t.Fatal(err)
	}
	got, err := store.Setting(ctx, "api_key")
	if err != nil {
		t.Fatal(err)
	}
	if got != "plain-secret" {
		t.Fatalf("decrypted setting = %q, want plain-secret", got)
	}
	var raw string
	if err := store.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'api_key'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw == "plain-secret" {
		t.Fatal("secret setting stored in plaintext")
	}
}

func TestSettingsCacheSupportsConcurrentReadsAndInvalidation(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.SetSetting(ctx, "server_url", "http://media-0:8096", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Settings(ctx); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 33)
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 50 {
				if _, err := store.Settings(ctx); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 1; i <= 20; i++ {
			if err := store.SetSetting(ctx, "server_url", fmt.Sprintf("http://media-%d:8096", i), false); err != nil {
				errs <- err
				return
			}
		}
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ServerURL != "http://media-20:8096" {
		t.Fatalf("final cached server URL = %q, want final update", settings.ServerURL)
	}
}

func TestSettingsCacheInvalidation(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.SetSetting(ctx, "media_provider", "jellyfin", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, "server_url", "http://media:8096", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, "public_url", "https://aperture.example", false); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(ctx, "api_key", "api-key", true); err != nil {
		t.Fatal(err)
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Provider != "jellyfin" || settings.ServerURL != "http://media:8096" || settings.APIKey != "api-key" {
		t.Fatalf("Settings() = %#v", settings)
	}
	if err := store.SetSetting(ctx, "server_url", "http://updated-media:8096", false); err != nil {
		t.Fatal(err)
	}
	settings, err = store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.ServerURL != "http://updated-media:8096" {
		t.Fatalf("Settings() after update = %#v; cache was not invalidated", settings)
	}
}

func TestUpdateApplicationSettingsWritesAtomically(t *testing.T) {
	ctx, store := testStore(t)
	provider := "emby"
	publicURL := "https://join.example.com"
	url := "http://media:8096/emby"
	apiKey := "api-key"
	if _, err := store.PublishMediaConnection(ctx, ConnectionUpdate{ExpectedGeneration: 1, Origin: MediaBinding{Provider: provider, BaseURL: url, ServerID: "emby-server"}, Provider: &provider, PublicURL: &publicURL, ServerURL: &url, APIKey: &apiKey}); err != nil {
		t.Fatal(err)
	}
	settings, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Provider != provider || settings.PublicURL != publicURL || settings.ServerURL != url || settings.APIKey != apiKey {
		t.Fatalf("settings = %#v, want application settings updated together", settings)
	}
	var rawAPIKey string
	if err := store.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'api_key'`).Scan(&rawAPIKey); err != nil {
		t.Fatal(err)
	}
	if rawAPIKey == apiKey {
		t.Fatal("atomic media settings update stored API key in plaintext")
	}
}

func TestUpdateApplicationSettingsRollsBackPartialWrite(t *testing.T) {
	ctx, store := testStore(t)
	if _, err := store.db.ExecContext(ctx, `
		CREATE TRIGGER fail_api_key
		BEFORE INSERT ON settings
		WHEN NEW.key = 'api_key'
		BEGIN
			SELECT RAISE(ABORT, 'injected API key failure');
		END;
	`); err != nil {
		t.Fatal(err)
	}
	url := "http://must-not-persist:8096"
	apiKey := "api-key"
	if _, err := store.PublishMediaConnection(ctx, ConnectionUpdate{ExpectedGeneration: 1, Origin: MediaBinding{Provider: "jellyfin", BaseURL: url, ServerID: "other-server"}, ServerURL: &url, APIKey: &apiKey}); err == nil {
		t.Fatal("atomic settings update unexpectedly succeeded")
	}
	if _, err := store.Setting(ctx, "server_url"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("server URL survived rolled-back update: %v", err)
	}
}

func TestEnsureRuntimeSecretsGeneratesAndReusesSecrets(t *testing.T) {
	ctx, store := testStore(t)
	if err := store.EnsureRuntimeSecrets(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	first, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.SessionSecret) < 32 || len(first.InviteSecret) < 32 || first.SessionSecret == first.InviteSecret {
		t.Fatalf("generated secrets are invalid: %#v", first)
	}
	if err := store.EnsureRuntimeSecrets(ctx, "", ""); err != nil {
		t.Fatal(err)
	}
	second, err := store.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionSecret != first.SessionSecret || second.InviteSecret != first.InviteSecret {
		t.Fatal("generated runtime secrets changed between startups")
	}
}
