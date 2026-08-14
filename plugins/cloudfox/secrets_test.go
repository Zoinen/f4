package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

type memoryKeyring struct {
	mu     sync.Mutex
	values map[string]string
}

type failingSecretStore struct{ err error }

func (s failingSecretStore) Put(context.Context, string, SecretValues) (string, error) {
	return "", s.err
}
func (s failingSecretStore) Get(context.Context, string) (SecretValues, error) {
	return nil, s.err
}
func (s failingSecretStore) Delete(context.Context, string) error { return s.err }

type memorySecretStore struct {
	mu     sync.Mutex
	values map[string]SecretValues
}

func (s *memorySecretStore) Put(_ context.Context, id string, values SecretValues) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]SecretValues)
	}
	ref := "vault:v1:" + id
	s.values[ref] = values.Clone()
	return ref, nil
}
func (s *memorySecretStore) Get(_ context.Context, ref string) (SecretValues, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[ref]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return value.Clone(), nil
}
func (s *memorySecretStore) Delete(_ context.Context, ref string) error {
	s.mu.Lock()
	delete(s.values, ref)
	s.mu.Unlock()
	return nil
}

func (m *memoryKeyring) Set(service, user, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.values == nil {
		m.values = make(map[string]string)
	}
	m.values[service+"\x00"+user] = password
	return nil
}
func (m *memoryKeyring) Get(service, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[service+"\x00"+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}
func (m *memoryKeyring) Delete(service, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := service + "\x00" + user
	if _, ok := m.values[key]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.values, key)
	return nil
}

func TestKeyringStoreUsesOpaqueRefsAndRoundTrips(t *testing.T) {
	t.Parallel()
	api := &memoryKeyring{}
	store := newKeyringStoreWithAPI(api)
	values := SecretValues{"password": "known-secret", "token": "refresh-token"}
	ref, err := store.Put(context.Background(), testConnectionID, values)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref, "keyring:v1:"+testConnectionID+":") || strings.Contains(ref, "known-secret") {
		t.Fatalf("unsafe ref %q", ref)
	}
	got, err := store.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got["password"] != values["password"] || got["token"] != values["token"] {
		t.Fatalf("secret round trip = %#v", got)
	}
	if err := store.Delete(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(context.Background(), ref); !errors.Is(err, ErrSecretNotFound) {
		t.Fatalf("Get after Delete = %v", err)
	}
}

func TestRepositoryRollsBackStagedSecretOnMetadataFailure(t *testing.T) {
	t.Parallel()
	api := &memoryKeyring{}
	secretStore := newKeyringStoreWithAPI(api)
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Keyring: secretStore},
	}
	first := testConnection("Same", ProviderS3)
	values := SecretValues{"secret": "first"}
	if _, err := repo.Save(context.Background(), first, &values, SecretStorageKeyring); err != nil {
		t.Fatal(err)
	}
	second := testConnection("same", ProviderWebDAV)
	other := SecretValues{"secret": "must-be-rolled-back"}
	if _, err := repo.Save(context.Background(), second, &other, SecretStorageKeyring); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("duplicate Save = %v", err)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.values) != 1 {
		t.Fatalf("keyring contains %d entries after rollback, want 1", len(api.values))
	}
}

func TestRepositoryFallsBackFromUnavailableKeyringToVault(t *testing.T) {
	t.Parallel()
	vault := &memorySecretStore{}
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets: SecretStores{
			Keyring: failingSecretStore{err: errors.New("credential manager unavailable")},
			Vault:   vault,
		},
	}
	values := SecretValues{"token": "still encrypted"}
	saved, err := repo.Save(context.Background(), testConnection("Fallback", ProviderYandexDisk), &values, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(saved.SecretRef, "vault:v1:") {
		t.Fatalf("fallback ref = %q", saved.SecretRef)
	}
	got, err := repo.Credentials(context.Background(), saved)
	if err != nil || got["token"] != values["token"] {
		t.Fatalf("fallback credentials = %#v, %v", got, err)
	}
}

func TestRepositoryRejectsStaleMetadataAfterCredentialRotation(t *testing.T) {
	t.Parallel()
	api := &memoryKeyring{}
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Keyring: newKeyringStoreWithAPI(api)},
	}
	initial := SecretValues{"token": "initial"}
	connection, err := repo.Save(context.Background(), testConnection("Profile", ProviderYandexDisk), &initial, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	stale := connection.Clone()
	rotated := SecretValues{"token": "rotated"}
	current, err := repo.Save(context.Background(), connection, &rotated, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	stale.Name = "Stale rename"
	if _, err := repo.Save(context.Background(), stale, nil, ""); !errors.Is(err, ErrConnectionChanged) {
		t.Fatalf("stale Save error = %v", err)
	}
	latest, err := repo.Get(context.Background(), current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != current.Name || latest.SecretRef != current.SecretRef {
		t.Fatalf("stale save changed current profile: %+v", latest)
	}
}

func TestRepositorySecretRotationIsCompareAndSwap(t *testing.T) {
	t.Parallel()
	api := &memoryKeyring{}
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Keyring: newKeyringStoreWithAPI(api)},
	}
	initial := SecretValues{"token": "initial"}
	connection, err := repo.Save(context.Background(), testConnection("Profile", ProviderYandexDisk), &initial, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	firstValues := SecretValues{"token": "first winner"}
	first, swapped, err := repo.RotateSecretsIfCurrent(context.Background(), connection.ID, connection.SecretRef, connection.UpdatedAt, firstValues, SecretStorageKeyring)
	if err != nil || !swapped {
		t.Fatalf("first rotation = %+v, %v, %v", first, swapped, err)
	}
	loserValues := SecretValues{"token": "stale loser"}
	if _, swapped, err := repo.RotateSecretsIfCurrent(context.Background(), connection.ID, connection.SecretRef, connection.UpdatedAt, loserValues, SecretStorageKeyring); err != nil || swapped {
		t.Fatalf("stale rotation swapped=%v err=%v", swapped, err)
	}
	credentials, err := repo.Credentials(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	if credentials["token"] != "first winner" {
		t.Fatalf("winning credentials were overwritten: %#v", credentials)
	}
}

func TestVaultEncryptsAndUnlocksOnceForConcurrentReaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CloudFox.vault")
	var prompts atomic.Int32
	prompt := MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		prompts.Add(1)
		return "master password", nil
	})
	vault := NewVaultStore(path, prompt)
	secret := SecretValues{"password": "known-plaintext-secret"}
	ref, err := vault.Put(context.Background(), testConnectionID, secret)
	if err != nil {
		t.Fatal(err)
	}
	if prompts.Load() != 1 {
		t.Fatalf("creation prompts = %d", prompts.Load())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret["password"]) || strings.Contains(string(data), "master password") {
		t.Fatal("vault contains plaintext")
	}
	var envelope vaultEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.KDF != "argon2id" || envelope.Cipher != "xchacha20-poly1305" {
		t.Fatalf("unexpected vault algorithms: %#v", envelope)
	}

	vault.Lock()
	prompts.Store(0)
	const readers = 8
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value, err := vault.Get(context.Background(), ref)
			if err == nil && value["password"] != secret["password"] {
				err = errors.New("wrong decrypted value")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if prompts.Load() != 1 {
		t.Fatalf("concurrent unlock prompts = %d, want 1", prompts.Load())
	}
}

func TestVaultAllowsEmptyMasterPasswordAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CloudFox.vault")
	emptyPrompt := MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		return "", nil
	})
	vault := NewVaultStore(path, emptyPrompt)
	secret := SecretValues{"token": "empty-password-secret"}
	ref, err := vault.Put(context.Background(), testConnectionID, secret)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret["token"]) {
		t.Fatal("empty-password vault unexpectedly contains literal plaintext")
	}

	var reopenPrompts atomic.Int32
	reopened := NewVaultStore(path, MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		reopenPrompts.Add(1)
		return "", errors.New("empty-password vault must unlock without prompting")
	}))
	got, err := reopened.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if reopenPrompts.Load() != 0 {
		t.Fatalf("empty-password vault prompted %d times, want 0", reopenPrompts.Load())
	}
	if got["token"] != secret["token"] {
		t.Fatalf("reopened credentials = %#v", got)
	}
}

func TestVaultCanChangeMasterPasswordToEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CloudFox.vault")
	vault := NewVaultStore(path, MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		return "initial password", nil
	}))
	ref, err := vault.Put(context.Background(), testConnectionID, SecretValues{"token": "rotated-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.ChangeMasterPassword(context.Background(), ""); err != nil {
		t.Fatal(err)
	}

	reopened := NewVaultStore(path, nil)
	got, err := reopened.Get(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got["token"] != "rotated-secret" {
		t.Fatalf("credentials after empty-password rotation = %#v", got)
	}
}

func TestEmptyMasterPasswordWarningDescribesPlaintextLevelRisk(t *testing.T) {
	for _, required := range []string{
		"empty master password",
		"effectively unprotected",
		"OAuth tokens",
		"access keys",
		"passwords",
		"treated as plaintext",
		"anyone who can read the vault file",
	} {
		if !strings.Contains(emptyMasterPasswordWarning, required) {
			t.Fatalf("empty-password warning does not mention %q: %q", required, emptyMasterPasswordWarning)
		}
	}
}

func TestWrongVaultPasswordDoesNotOverwriteFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CloudFox.vault")
	vault := NewVaultStore(path, MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		return "right password", nil
	}))
	ref, err := vault.Put(context.Background(), testConnectionID, SecretValues{"token": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := NewVaultStore(path, MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		return "wrong password", nil
	}))
	if _, err := wrong.Get(context.Background(), ref); !errors.Is(err, ErrWrongMasterPassword) {
		t.Fatalf("wrong password error = %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("wrong-password attempt changed vault")
	}
}

func TestVaultInterleavedStoresPreserveEachOthersUpdates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "CloudFox.vault")
	prompt := MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		return "shared master password", nil
	})
	first := NewVaultStore(path, prompt)
	firstRef, err := first.Put(context.Background(), "first", SecretValues{"token": "one"})
	if err != nil {
		t.Fatal(err)
	}
	second := NewVaultStore(path, prompt)
	if _, err := second.Get(context.Background(), firstRef); err != nil {
		t.Fatal(err)
	}
	secondRef, err := second.Put(context.Background(), "second", SecretValues{"token": "two"})
	if err != nil {
		t.Fatal(err)
	}
	thirdRef, err := first.Put(context.Background(), "third", SecretValues{"token": "three"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		ref  string
		want string
	}{{firstRef, "one"}, {secondRef, "two"}, {thirdRef, "three"}} {
		got, err := second.Get(context.Background(), tc.ref)
		if err != nil {
			t.Fatalf("Get(%q): %v", tc.ref, err)
		}
		if got["token"] != tc.want {
			t.Fatalf("Get(%q) = %#v, want token %q", tc.ref, got, tc.want)
		}
	}
}

func TestVaultLockInvalidatesInflightUnlock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	vault := NewVaultStore(filepath.Join(t.TempDir(), "CloudFox.vault"), MasterPasswordPromptFunc(func(context.Context, bool) (string, error) {
		close(started)
		<-release
		return "master password", nil
	}))
	done := make(chan error, 1)
	go func() {
		_, err := vault.Put(context.Background(), "profile", SecretValues{"token": "secret"})
		done <- err
	}()
	<-started
	vault.Lock()
	close(release)
	if err := <-done; !errors.Is(err, ErrVaultLocked) {
		t.Fatalf("in-flight unlock error = %v, want ErrVaultLocked", err)
	}
	if !vault.IsLocked() {
		t.Fatal("in-flight unlock repopulated a locked vault")
	}
}
