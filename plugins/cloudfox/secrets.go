package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	keyring "github.com/zalando/go-keyring"
)

const keyringService = "f4-cloudfox"

// SecretStore persists credentials separately from connection metadata.
// Put must create a new immutable ref so Repository can rotate credentials
// without making a crash lose the previously committed value.
type SecretStore interface {
	Put(context.Context, string, SecretValues) (string, error)
	Get(context.Context, string) (SecretValues, error)
	Delete(context.Context, string) error
}

type keyringAPI interface {
	Set(service, user, password string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type systemKeyring struct{}

func (systemKeyring) Set(service, user, password string) error {
	return keyring.Set(service, user, password)
}
func (systemKeyring) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}
func (systemKeyring) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

// KeyringStore stores one JSON credential bundle in the operating-system
// credential manager. Its opaque user key is random and never a display name.
type KeyringStore struct {
	api keyringAPI
}

func NewKeyringStore() *KeyringStore { return &KeyringStore{api: systemKeyring{}} }

func newKeyringStoreWithAPI(api keyringAPI) *KeyringStore { return &KeyringStore{api: api} }

func (s *KeyringStore) Put(ctx context.Context, connectionID string, values SecretValues) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	rotation, err := newUUID()
	if err != nil {
		return "", err
	}
	key := strings.ToLower(connectionID) + ":" + rotation
	data, err := json.Marshal(values.Clone())
	if err != nil {
		return "", fmt.Errorf("cloudfox: encode keyring secret: %w", err)
	}
	if err := s.api.Set(keyringService, key, string(data)); err != nil {
		return "", fmt.Errorf("cloudfox: write OS keyring: %w", err)
	}
	return "keyring:v1:" + key, nil
}

func (s *KeyringStore) Get(ctx context.Context, ref string) (SecretValues, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, err := parseSecretRef(ref, "keyring")
	if err != nil {
		return nil, err
	}
	data, err := s.api.Get(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("cloudfox: read OS keyring: %w", err)
	}
	var values SecretValues
	if err := json.Unmarshal([]byte(data), &values); err != nil {
		return nil, fmt.Errorf("cloudfox: damaged OS keyring entry: %w", err)
	}
	return values.Clone(), nil
}

func (s *KeyringStore) Delete(ctx context.Context, ref string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key, err := parseSecretRef(ref, "keyring")
	if err != nil {
		return err
	}
	err = s.api.Delete(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cloudfox: delete OS keyring entry: %w", err)
	}
	return nil
}

func parseSecretRef(ref, kind string) (string, error) {
	prefix := kind + ":v1:"
	if !strings.HasPrefix(ref, prefix) || strings.TrimPrefix(ref, prefix) == "" {
		return "", fmt.Errorf("cloudfox: invalid %s secret reference", kind)
	}
	return strings.TrimPrefix(ref, prefix), nil
}

// SecretStores dispatches opaque refs without ever trying another store on an
// error. This prevents a damaged vault/keyring from becoming a plaintext or
// alternate-storage fallback.
type SecretStores struct {
	Keyring SecretStore
	Vault   SecretStore
}

func (s SecretStores) storeForRef(ref string) (SecretStore, error) {
	switch {
	case strings.HasPrefix(ref, "keyring:v1:"):
		if s.Keyring != nil {
			return s.Keyring, nil
		}
	case strings.HasPrefix(ref, "vault:v1:"):
		if s.Vault != nil {
			return s.Vault, nil
		}
	}
	return nil, fmt.Errorf("cloudfox: no secret store for reference %q", ref)
}

func (s SecretStores) storeForKind(kind SecretStorage) (SecretStore, error) {
	switch kind {
	case SecretStorageKeyring:
		if s.Keyring != nil {
			return s.Keyring, nil
		}
	case SecretStorageVault:
		if s.Vault != nil {
			return s.Vault, nil
		}
	}
	return nil, fmt.Errorf("cloudfox: secret storage %q is unavailable", kind)
}

func (s SecretStores) Get(ctx context.Context, ref string) (SecretValues, error) {
	if ref == "" {
		return SecretValues{}, nil
	}
	store, err := s.storeForRef(ref)
	if err != nil {
		return nil, err
	}
	return store.Get(ctx, ref)
}

func (s SecretStores) Delete(ctx context.Context, ref string) error {
	if ref == "" {
		return nil
	}
	store, err := s.storeForRef(ref)
	if err != nil {
		return err
	}
	return store.Delete(ctx, ref)
}

// Repository commits metadata and secret rotations in a crash-safe order.
type Repository struct {
	Connections *ConnectionStore
	Secrets     SecretStores
}

func (r *Repository) List(ctx context.Context) ([]Connection, error) {
	return r.Connections.List(ctx)
}

func (r *Repository) Get(ctx context.Context, id string) (Connection, error) {
	return r.Connections.Get(ctx, id)
}

func (r *Repository) Credentials(ctx context.Context, c Connection) (SecretValues, error) {
	values, err := r.Secrets.Get(ctx, c.SecretRef)
	if err != nil {
		return nil, err
	}
	if err := verifyCredentialScope(c, values, true); err != nil {
		clearSecrets(values)
		return nil, err
	}
	return values, nil
}

func (r *Repository) putSecrets(ctx context.Context, connectionID string, values SecretValues, storage SecretStorage) (string, error) {
	store, err := r.Secrets.storeForKind(storage)
	if err != nil {
		return "", err
	}
	newRef, err := store.Put(ctx, connectionID, values.Clone())
	// An unavailable desktop keyring falls back only to the authenticated
	// portable vault. The original error is retained if the vault cannot be
	// unlocked; there is deliberately no plaintext fallback.
	if err != nil && storage == SecretStorageKeyring && r.Secrets.Vault != nil {
		keyringErr := err
		newRef, err = r.Secrets.Vault.Put(ctx, connectionID, values.Clone())
		if err != nil {
			return "", errors.Join(keyringErr, err)
		}
	}
	return newRef, err
}

// Save creates or updates c. A nil values pointer preserves the existing
// secret reference. A non-nil pointer rotates it, including to an empty bundle.
func (r *Repository) Save(ctx context.Context, c Connection, values *SecretValues, storage SecretStorage) (Connection, error) {
	if r == nil || r.Connections == nil {
		return Connection{}, errors.New("cloudfox: repository is not configured")
	}
	isNew := c.ID == ""
	if isNew {
		id, err := newUUID()
		if err != nil {
			return Connection{}, err
		}
		c.ID = id
	}

	expectedUpdatedAt := c.UpdatedAt
	oldRef := c.SecretRef
	newRef := ""
	if values != nil {
		bound, err := bindCredentialScope(c, values.Clone())
		if err != nil {
			return Connection{}, err
		}
		newRef, err = r.putSecrets(ctx, c.ID, bound, storage)
		clearSecrets(bound)
		if err != nil {
			return Connection{}, err
		}
		c.SecretRef = newRef
	} else if !isNew && oldRef != "" {
		// A metadata-only update may rename a profile, but it must not silently
		// retarget an already-bound credential bundle.
		current, err := r.Connections.Get(ctx, c.ID)
		if err != nil {
			return Connection{}, err
		}
		if current.SecretRef != oldRef || !current.UpdatedAt.Equal(expectedUpdatedAt) {
			return Connection{}, ErrConnectionChanged
		}
		_, required, err := credentialScope(c)
		if err != nil {
			return Connection{}, err
		}
		if required {
			verified, err := r.Credentials(ctx, c)
			if err != nil {
				return Connection{}, err
			}
			clearSecrets(verified)
		}
	} else if isNew && oldRef == "" {
		_, required, err := credentialScope(c)
		if err != nil {
			return Connection{}, err
		}
		if required {
			return Connection{}, ErrCredentialScopeUnbound
		}
	}

	var (
		saved Connection
		err   error
	)
	if isNew {
		saved, err = r.Connections.Create(ctx, c)
	} else {
		saved, err = r.Connections.UpdateIfCurrent(ctx, c, expectedUpdatedAt, oldRef)
	}
	if err != nil {
		if newRef != "" {
			_ = r.Secrets.Delete(context.Background(), newRef)
		}
		return Connection{}, err
	}
	if newRef != "" && oldRef != "" && oldRef != newRef {
		_ = r.Secrets.Delete(context.Background(), oldRef)
	}
	return saved, nil
}

// RotateSecretsIfCurrent stages immutable credentials and commits their
// reference only while expectedRef is still current. It is the safe path for
// background OAuth refreshes racing with a user reauthorization.
func (r *Repository) RotateSecretsIfCurrent(ctx context.Context, id, expectedRef string, expectedUpdatedAt time.Time, values SecretValues, storage SecretStorage) (Connection, bool, error) {
	if r == nil || r.Connections == nil {
		return Connection{}, false, errors.New("cloudfox: repository is not configured")
	}
	if id == "" || expectedRef == "" {
		return Connection{}, false, errors.New("cloudfox: connection id and current secret reference are required")
	}
	current, err := r.Connections.Get(ctx, id)
	if err != nil {
		return Connection{}, false, err
	}
	if current.SecretRef != expectedRef || !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return Connection{}, false, nil
	}
	verified, err := r.Credentials(ctx, current)
	if err != nil {
		return Connection{}, false, err
	}
	clearSecrets(verified)
	bound, err := bindCredentialScope(current, values.Clone())
	if err != nil {
		return Connection{}, false, err
	}
	newRef, err := r.putSecrets(ctx, id, bound, storage)
	clearSecrets(bound)
	if err != nil {
		return Connection{}, false, err
	}
	saved, swapped, err := r.Connections.ReplaceSecretRefIfCurrent(ctx, id, expectedRef, expectedUpdatedAt, newRef)
	if err != nil || !swapped {
		_ = r.Secrets.Delete(context.Background(), newRef)
		return Connection{}, false, err
	}
	if expectedRef != newRef {
		_ = r.Secrets.Delete(context.Background(), expectedRef)
	}
	return saved, true, nil
}

// Delete commits removal of public metadata first, then performs best-effort
// credential cleanup. The returned error is only a metadata failure; an orphan
// keyring entry is safer than restoring a deleted profile after a crash.
func (r *Repository) Delete(ctx context.Context, id string) error {
	deleted, err := r.Connections.Delete(ctx, id)
	if err != nil {
		return err
	}
	if deleted.SecretRef != "" {
		_ = r.Secrets.Delete(context.Background(), deleted.SecretRef)
	}
	return nil
}
