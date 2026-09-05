package cloudfox

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func googleDialogTestConnection(t *testing.T, clientID string) Connection {
	t.Helper()
	settings, err := json.Marshal(GoogleDriveSettings{ClientID: clientID})
	if err != nil {
		t.Fatal(err)
	}
	return Connection{Name: "Google", Provider: ProviderGoogleDrive, Settings: settings}
}

func TestGoogleProfileAuthorizationSecretsDoNotCrossOAuthAudience(t *testing.T) {
	store := &memorySecretStore{}
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Factories: []BackendFactory{&GoogleDriveFactory{}},
		Keyring:   store,
		Vault:     store,
	})
	t.Cleanup(func() { _ = plugin.Close() })

	oldValues := SecretValues{ // #nosec G101 -- synthetic OAuth credentials verify that secrets do not cross an edited client-ID boundary.
		"client_secret": "old-client-secret",
		"access_token":  "old-access-token",
		"refresh_token": "old-refresh-token",
		"expires_at":    "2099-01-01T00:00:00Z",
	}
	original, err := plugin.repo.Save(context.Background(), googleDialogTestConnection(t, "old-client-id"), &oldValues, SecretStorageVault)
	if err != nil {
		t.Fatal(err)
	}
	dialog := &cloudProfileDialog{plugin: plugin, provider: ProviderGoogleDrive, original: &original}

	sameAudience, err := dialog.googleSecretsForAuthorization(context.Background(), original, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sameAudience, SecretValues{"client_secret": "old-client-secret"}) { // #nosec G101 -- this is the synthetic secret expected from the fixture above.
		t.Fatalf("same-audience authorization secrets = %#v", sameAudience)
	}

	updated := original.Clone()
	updated.Settings = googleDialogTestConnection(t, "new-client-id").Settings
	newAudience, err := dialog.googleSecretsForAuthorization(context.Background(), updated, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(newAudience) != 0 {
		t.Fatalf("new-audience authorization inherited old credentials: %#v", newAudience)
	}

	typed := SecretValues{"client_secret": "new-client-secret"} // #nosec G101 -- synthetic user input verifies that only the newly typed secret is retained.
	newAudience, err = dialog.googleSecretsForAuthorization(context.Background(), updated, typed)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(newAudience, typed) {
		t.Fatalf("new-audience authorization secrets = %#v, want only typed secret", newAudience)
	}
}

func TestGoogleStagedTokensDoNotSurviveClientIDEdit(t *testing.T) {
	staged := SecretValues{
		"client_secret": "first-secret",
		"access_token":  "first-access",
		"refresh_token": "first-refresh",
	}

	matching, err := googleStagedSecretsForConnection(googleDialogTestConnection(t, "first-client"), staged, "first-client")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(matching, staged) {
		t.Fatalf("matching staged credentials = %#v", matching)
	}

	changed, err := googleStagedSecretsForConnection(googleDialogTestConnection(t, "second-client"), staged, "first-client")
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("edited client ID retained staged credentials: %#v", changed)
	}
}

func TestGoogleOAuthConfigUsesBundledSecretOnlyForBundledClient(t *testing.T) {
	oldClientID, oldClientSecret := DefaultGoogleClientID, DefaultGoogleClientSecret
	DefaultGoogleClientID = "bundled-client"
	DefaultGoogleClientSecret = "bundled-secret"
	t.Cleanup(func() {
		DefaultGoogleClientID = oldClientID
		DefaultGoogleClientSecret = oldClientSecret
	})

	bundled := googleOAuthConfig(GoogleDriveSettings{ClientID: "bundled-client"}, nil, "http://127.0.0.1/callback")
	if bundled.ClientSecret != "bundled-secret" {
		t.Fatalf("bundled client secret = %q", bundled.ClientSecret)
	}
	custom := googleOAuthConfig(GoogleDriveSettings{ClientID: "custom-public-client"}, nil, "http://127.0.0.1/callback")
	if custom.ClientSecret != "" {
		t.Fatalf("custom public client inherited bundled secret %q", custom.ClientSecret)
	}
	explicit := googleOAuthConfig(GoogleDriveSettings{ClientID: "custom-confidential-client"}, SecretValues{"client_secret": "explicit-secret"}, "http://127.0.0.1/callback")
	if explicit.ClientSecret != "explicit-secret" {
		t.Fatalf("explicit custom client secret = %q", explicit.ClientSecret)
	}
}
