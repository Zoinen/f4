package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func scopedTestConnection(t *testing.T, name string, provider ProviderType, settings any) Connection {
	t.Helper()
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	return Connection{Name: name, Provider: provider, Settings: raw}
}

func scopedTestRepository(t *testing.T) (*Repository, *memorySecretStore) {
	t.Helper()
	store := &memorySecretStore{}
	return &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Vault: store},
	}, store
}

func TestRepositoryBindsCredentialsAndRejectsRetargetedMetadata(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider ProviderType
		settings any
		retarget any
		secrets  SecretValues
	}{
		{
			name:     "S3",
			provider: ProviderS3,
			settings: S3Settings{Bucket: "documents", Region: "eu-west-1", Endpoint: "https://objects.example", Auth: "static"},
			retarget: S3Settings{Bucket: "documents", Region: "eu-west-1", Endpoint: "https://attacker.example", Auth: "static"},
			secrets:  SecretValues{"access_key_id": "access", "secret_access_key": "secret"},
		},
		{
			name:     "WebDAV",
			provider: ProviderWebDAV,
			settings: WebDAVSettings{BaseURL: "https://dav.example/remote.php/dav", Root: "/files", Auth: "basic", Username: "alice"},
			retarget: WebDAVSettings{BaseURL: "https://attacker.example/dav", Root: "/files", Auth: "basic", Username: "alice"},
			secrets:  SecretValues{"password": "secret"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo, store := scopedTestRepository(t)
			connection := scopedTestConnection(t, tt.name, tt.provider, tt.settings)
			saved, err := repo.Save(context.Background(), connection, &tt.secrets, SecretStorageVault)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := store.Get(context.Background(), saved.SecretRef)
			if err != nil {
				t.Fatal(err)
			}
			if raw[credentialScopeSecretKey] == "" {
				t.Fatal("stored credential bundle has no scope binding")
			}
			if _, err := repo.Credentials(context.Background(), saved); err != nil {
				t.Fatalf("Credentials with original metadata = %v", err)
			}

			retargeted := saved.Clone()
			retargeted.Settings = scopedTestConnection(t, "retargeted", tt.provider, tt.retarget).Settings
			if _, err := repo.Credentials(context.Background(), retargeted); !errors.Is(err, ErrCredentialScopeMismatch) {
				t.Fatalf("Credentials with retargeted metadata = %v", err)
			}
			if _, err := repo.Save(context.Background(), retargeted, nil, ""); !errors.Is(err, ErrCredentialScopeMismatch) {
				t.Fatalf("metadata-only retarget Save = %v", err)
			}
		})
	}
}

func TestCredentialScopeCanonicalizationAndSecurityBoundaries(t *testing.T) {
	t.Parallel()
	webDAVOne := scopedTestConnection(t, "one", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://DAV.Example:443/remote.php/dav", Root: "/alice/first", Auth: "BASIC", Username: "alice",
	})
	webDAVTwo := scopedTestConnection(t, "two", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/another/base", Root: "/alice/second", Auth: "basic", Username: "alice",
	})
	first, required, err := credentialScope(webDAVOne)
	if err != nil || !required {
		t.Fatalf("first WebDAV scope = %q, %v, %v", first, required, err)
	}
	second, _, err := credentialScope(webDAVTwo)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("WebDAV subtree did not change credential scope")
	}
	webDAVTwo.Settings = scopedTestConnection(t, "two", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example:443/remote.php/dav/", Root: "/alice/first/../first", Auth: "basic", Username: "alice",
	}).Settings
	second, _, _ = credentialScope(webDAVTwo)
	if first != second {
		t.Fatal("equivalent WebDAV subtree produced a different credential scope")
	}
	webDAVTwo.Settings = scopedTestConnection(t, "two", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/another/base", Auth: "basic", Username: "bob",
	}).Settings
	second, _, _ = credentialScope(webDAVTwo)
	if first == second {
		t.Fatal("WebDAV username did not change credential scope")
	}
	webDAVTwo.Settings = scopedTestConnection(t, "two", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/dav", Auth: "digest", Username: "alice",
	}).Settings
	second, _, _ = credentialScope(webDAVTwo)
	if first == second {
		t.Fatal("WebDAV authentication mode did not change credential scope")
	}

	s3One := scopedTestConnection(t, "one", ProviderS3, S3Settings{
		Bucket: "documents", Region: "EU-WEST-1", Endpoint: "https://S3.Example:443/api/", Auth: "profile", Profile: " team ",
	})
	s3Two := scopedTestConnection(t, "two", ProviderS3, S3Settings{
		Bucket: "documents", Region: "eu-west-1", Endpoint: "https://s3.example/api", Auth: "PROFILE", Profile: "team",
	})
	first, required, err = credentialScope(s3One)
	if err != nil || !required {
		t.Fatalf("first S3 scope = %q, %v, %v", first, required, err)
	}
	second, _, err = credentialScope(s3Two)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent S3 endpoint/region/profile settings produced different scopes")
	}
	variants := map[string]S3Settings{
		"bucket":     {Bucket: "other", Region: "eu-west-1", Endpoint: "https://s3.example/api", Auth: "profile", Profile: "team"},
		"region":     {Bucket: "documents", Region: "us-east-1", Endpoint: "https://s3.example/api", Auth: "profile", Profile: "team"},
		"endpoint":   {Bucket: "documents", Region: "eu-west-1", Endpoint: "https://other.example/api", Auth: "profile", Profile: "team"},
		"profile":    {Bucket: "documents", Region: "eu-west-1", Endpoint: "https://s3.example/api", Auth: "profile", Profile: "other"},
		"auth mode":  {Bucket: "documents", Region: "eu-west-1", Endpoint: "https://s3.example/api", Auth: "default", Profile: "team"},
		"path style": {Bucket: "documents", Region: "eu-west-1", Endpoint: "https://s3.example/api", Auth: "profile", Profile: "team", UsePathStyle: true},
	}
	for name, settings := range variants {
		s3Two.Settings = scopedTestConnection(t, "two", ProviderS3, settings).Settings
		second, _, _ = credentialScope(s3Two)
		if first == second {
			t.Fatalf("S3 %s did not change credential scope", name)
		}
	}
}

func TestCredentialScopeBindsCustomCAContentsAndFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first-ca.pem")
	secondPath := filepath.Join(dir, "second-ca.pem")
	missingPath := filepath.Join(dir, "missing-ca.pem")
	initial := []byte("-----BEGIN CERTIFICATE-----\ninitial\n-----END CERTIFICATE-----\n")
	changed := []byte("-----BEGIN CERTIFICATE-----\nchanged\n-----END CERTIFICATE-----\n")
	if err := os.WriteFile(firstPath, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, initial, 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		provider    ProviderType
		settings    func(string) any
		credentials SecretValues
	}{
		{
			name:     "S3",
			provider: ProviderS3,
			settings: func(customCA string) any {
				return S3Settings{Bucket: "documents", Region: "us-east-1", Endpoint: "https://s3.example", Auth: "static", CustomCA: customCA}
			},
			credentials: SecretValues{"access_key_id": "access", "secret_access_key": "secret"},
		},
		{
			name:     "WebDAV",
			provider: ProviderWebDAV,
			settings: func(customCA string) any {
				return WebDAVSettings{BaseURL: "https://dav.example/dav", Auth: "basic", Username: "alice", CustomCA: customCA}
			},
			credentials: SecretValues{"password": "secret"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := os.WriteFile(firstPath, initial, 0o600); err != nil {
				t.Fatal(err)
			}
			repo, _ := scopedTestRepository(t)
			connection := scopedTestConnection(t, tt.name, tt.provider, tt.settings(firstPath))
			saved, err := repo.Save(context.Background(), connection, &tt.credentials, SecretStorageVault)
			if err != nil {
				t.Fatal(err)
			}

			sameContents := saved.Clone()
			sameContents.Settings = scopedTestConnection(t, tt.name, tt.provider, tt.settings(secondPath)).Settings
			if _, err := repo.Credentials(context.Background(), sameContents); err != nil {
				t.Fatalf("same CA contents at another path = %v", err)
			}

			if err := os.WriteFile(firstPath, changed, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.Credentials(context.Background(), saved); !errors.Is(err, ErrCredentialScopeMismatch) {
				t.Fatalf("credentials after CA content change = %v, want scope mismatch", err)
			}

			missing := saved.Clone()
			missing.Settings = scopedTestConnection(t, tt.name, tt.provider, tt.settings(missingPath)).Settings
			if _, err := repo.Credentials(context.Background(), missing); err == nil {
				t.Fatal("missing custom CA did not fail closed")
			}
		})
	}
}

func TestWebDAVFactoryRechecksExactCustomCABytesAfterCredentialLookup(t *testing.T) {
	t.Parallel()
	caPath := filepath.Join(t.TempDir(), "custom-ca.pem")
	if err := os.WriteFile(caPath, []byte("trusted-at-lookup"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo, _ := scopedTestRepository(t)
	connection := scopedTestConnection(t, "WebDAV CA swap", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/dav", Auth: "basic", Username: "alice", CustomCA: caPath,
	})
	credentials := SecretValues{"password": "secret"}
	saved, err := repo.Save(context.Background(), connection, &credentials, SecretStorageVault)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := repo.Credentials(context.Background(), saved)
	if err != nil {
		t.Fatal(err)
	}
	defer clearSecrets(verified)
	if err := os.WriteFile(caPath, []byte("swapped-after-lookup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (&WebDAVFactory{}).Open(context.Background(), saved, verified); !errors.Is(err, ErrCredentialScopeMismatch) {
		t.Fatalf("factory after custom CA swap = %v, want credential scope mismatch", err)
	}
}

func TestLegacyCredentialScopeMigrationIsExplicit(t *testing.T) {
	t.Parallel()
	repo, store := scopedTestRepository(t)
	legacy := scopedTestConnection(t, "Legacy WebDAV", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/dav", Auth: "basic", Username: "alice",
	})
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	legacy.ID = id
	legacy.SecretRef, err = store.Put(context.Background(), id, SecretValues{"password": "old-secret"})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err = repo.Connections.Create(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Credentials(context.Background(), legacy); !errors.Is(err, ErrCredentialScopeUnbound) {
		t.Fatalf("legacy Credentials = %v", err)
	}

	// Supplying a fresh value on an explicit Save migrates the profile and
	// never reads/reuses the unbound legacy password.
	fresh := SecretValues{"password": "fresh-secret"}
	migrated, err := repo.Save(context.Background(), legacy, &fresh, SecretStorageVault)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := repo.Credentials(context.Background(), migrated)
	if err != nil {
		t.Fatal(err)
	}
	if credentials["password"] != "fresh-secret" {
		t.Fatalf("migrated credentials = %#v", credentials)
	}

	defaultS3 := scopedTestConnection(t, "Legacy default S3", ProviderS3, S3Settings{
		Bucket: "documents", Region: "us-east-1", Auth: "default",
	})
	if _, err := repo.Save(context.Background(), defaultS3, nil, ""); !errors.Is(err, ErrCredentialScopeUnbound) {
		t.Fatalf("new unbound S3 Save = %v", err)
	}
	empty := SecretValues{}
	if _, err := repo.Save(context.Background(), defaultS3, &empty, SecretStorageVault); err != nil {
		t.Fatalf("explicitly confirmed S3 Save = %v", err)
	}

	anonymous := scopedTestConnection(t, "Anonymous S3", ProviderS3, S3Settings{
		Bucket: "public", Region: "us-east-1", Auth: "anonymous",
	})
	if _, err := repo.Save(context.Background(), anonymous, nil, ""); err != nil {
		t.Fatalf("anonymous Save = %v", err)
	}
}

func TestProfileScopeChangeRequiresFreshInlineCredentials(t *testing.T) {
	t.Parallel()
	original := scopedTestConnection(t, "WebDAV", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://dav.example/dav", Root: "/one", Auth: "basic", Username: "alice",
	})
	sameOrigin := scopedTestConnection(t, "WebDAV", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://DAV.EXAMPLE:443/other", Root: "/two", Auth: "basic", Username: "alice",
	})
	changed, err := validateCredentialScopeChange(&original, sameOrigin, nil)
	if !changed || err == nil {
		t.Fatalf("same-origin subtree retarget without password = %v, %v", changed, err)
	}
	if changed, err := validateCredentialScopeChange(&original, sameOrigin, SecretValues{"password": "fresh"}); !changed || err != nil {
		t.Fatalf("same-origin subtree retarget with password = %v, %v", changed, err)
	}

	retargeted := scopedTestConnection(t, "WebDAV", ProviderWebDAV, WebDAVSettings{
		BaseURL: "https://other.example/dav", Root: "/two", Auth: "basic", Username: "alice",
	})
	if changed, err := validateCredentialScopeChange(&original, retargeted, nil); !changed || err == nil {
		t.Fatalf("WebDAV retarget without password = %v, %v", changed, err)
	}
	if changed, err := validateCredentialScopeChange(&original, retargeted, SecretValues{"password": "fresh"}); !changed || err != nil {
		t.Fatalf("WebDAV retarget with fresh password = %v, %v", changed, err)
	}

	s3Original := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "one", Region: "us-east-1", Endpoint: "https://s3.example", Auth: "static",
	})
	s3Retargeted := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "two", Region: "us-east-1", Endpoint: "https://s3.example", Auth: "static",
	})
	if changed, err := validateCredentialScopeChange(&s3Original, s3Retargeted, SecretValues{"access_key_id": "access"}); !changed || err == nil {
		t.Fatalf("S3 retarget with partial static credentials = %v, %v", changed, err)
	}
	static := SecretValues{"access_key_id": "access", "secret_access_key": "fresh"}
	if changed, err := validateCredentialScopeChange(&s3Original, s3Retargeted, static); !changed || err != nil {
		t.Fatalf("S3 retarget with fresh static credentials = %v, %v", changed, err)
	}

	s3Profile := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "two", Region: "us-east-1", Endpoint: "https://s3.example", Auth: "profile", Profile: "team",
	})
	if changed, err := validateCredentialScopeChange(&s3Original, s3Profile, nil); !changed || err != nil {
		t.Fatalf("S3 external credential scope confirmation = %v, %v", changed, err)
	}
}
