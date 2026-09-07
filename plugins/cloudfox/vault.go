package cloudfox

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	vaultVersion      = 1
	vaultKDFTime      = 3
	vaultKDFMemoryKiB = 64 * 1024
	vaultKDFThreads   = 4
	vaultSaltSize     = 16
)

// MasterPasswordPrompter performs the UI interaction on the main thread.
// When creating is true it must ask for confirmation itself. The returned
// password is never persisted by CloudFox.
type MasterPasswordPrompter interface {
	PromptMasterPassword(context.Context, bool) (string, error)
}

type MasterPasswordPromptFunc func(context.Context, bool) (string, error)

func (f MasterPasswordPromptFunc) PromptMasterPassword(ctx context.Context, creating bool) (string, error) {
	return f(ctx, creating)
}

type vaultEnvelope struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Time       uint32 `json:"time"`
	MemoryKiB  uint32 `json:"memory_kib"`
	Threads    uint8  `json:"threads"`
	Salt       string `json:"salt"`
	Cipher     string `json:"cipher"`
	Nonce      string `json:"nonce"`
	Ciphertext string `json:"ciphertext"`
}

type vaultPayload struct {
	Version int                     `json:"version"`
	Secrets map[string]SecretValues `json:"secrets"`
}

// VaultStore is a whole-file authenticated portable secret store. One unlock
// prompt is shared by concurrent callers; the derived key exists only in RAM.
type VaultStore struct {
	path   string
	prompt MasterPasswordPrompter

	mu         sync.Mutex
	key        []byte
	salt       []byte
	secrets    map[string]SecretValues
	unlocking  bool
	wait       chan struct{}
	unlockErr  error
	generation uint64
}

func NewVaultStore(path string, prompt MasterPasswordPrompter) *VaultStore {
	return &VaultStore{path: path, prompt: prompt}
}

func (v *VaultStore) Path() string { return v.path }

func (v *VaultStore) Put(ctx context.Context, connectionID string, values SecretValues) (string, error) {
	if err := v.ensureUnlocked(ctx); err != nil {
		return "", err
	}
	rotation, err := newUUID()
	if err != nil {
		return "", err
	}
	key := connectionID + ":" + rotation
	release, err := acquireFileLock(ctx, v.path+".lock")
	if err != nil {
		return "", err
	}
	defer release()
	v.mu.Lock()
	defer v.mu.Unlock()
	latest, err := v.readLatestLocked()
	if err != nil {
		return "", err
	}
	updated := cloneSecretMap(latest)
	updated[key] = values.Clone()
	if err := v.writeLocked(updated, v.key, v.salt); err != nil {
		return "", err
	}
	v.secrets = updated
	return "vault:v1:" + key, nil
}

func (v *VaultStore) Get(ctx context.Context, ref string) (SecretValues, error) {
	key, err := parseSecretRef(ref, "vault")
	if err != nil {
		return nil, err
	}
	if err := v.ensureUnlocked(ctx); err != nil {
		return nil, err
	}
	release, err := acquireFileLock(ctx, v.path+".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	v.mu.Lock()
	defer v.mu.Unlock()
	latest, err := v.readLatestLocked()
	if err != nil {
		return nil, err
	}
	v.secrets = latest
	value, ok := latest[key]
	if !ok {
		return nil, ErrSecretNotFound
	}
	return value.Clone(), nil
}

func (v *VaultStore) Delete(ctx context.Context, ref string) error {
	key, err := parseSecretRef(ref, "vault")
	if err != nil {
		return err
	}
	if err := v.ensureUnlocked(ctx); err != nil {
		return err
	}
	release, err := acquireFileLock(ctx, v.path+".lock")
	if err != nil {
		return err
	}
	defer release()
	v.mu.Lock()
	defer v.mu.Unlock()
	latest, err := v.readLatestLocked()
	if err != nil {
		return err
	}
	if _, ok := latest[key]; !ok {
		v.secrets = latest
		return nil
	}
	updated := cloneSecretMap(latest)
	delete(updated, key)
	if err := v.writeLocked(updated, v.key, v.salt); err != nil {
		return err
	}
	v.secrets = updated
	return nil
}

func (v *VaultStore) Lock() {
	v.mu.Lock()
	v.generation++
	zero(v.key)
	v.key = nil
	v.salt = nil
	v.secrets = nil
	v.unlockErr = nil
	v.mu.Unlock()
}

func (v *VaultStore) IsLocked() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.key) == 0
}

// ChangeMasterPassword rewrites the authenticated envelope with a new random
// salt and key. An empty password is supported but provides no meaningful
// protection; interactive callers must warn the user before accepting it. The
// previous file remains intact if encryption or replacement fails.
func (v *VaultStore) ChangeMasterPassword(ctx context.Context, password string) error {
	if err := v.ensureUnlocked(ctx); err != nil {
		return err
	}
	salt := make([]byte, vaultSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("cloudfox: generate vault salt: %w", err)
	}
	key := deriveVaultKey(password, salt, vaultKDFTime, vaultKDFMemoryKiB, vaultKDFThreads)
	release, err := acquireFileLock(ctx, v.path+".lock")
	if err != nil {
		zero(key)
		return err
	}
	defer release()
	v.mu.Lock()
	defer v.mu.Unlock()
	latest, err := v.readLatestLocked()
	if err != nil {
		zero(key)
		return err
	}
	if err := v.writeLocked(latest, key, salt); err != nil {
		zero(key)
		return err
	}
	zero(v.key)
	v.key = key
	v.salt = salt
	v.secrets = latest
	return nil
}

func (v *VaultStore) ensureUnlocked(ctx context.Context) error {
	v.mu.Lock()
	if len(v.key) != 0 {
		v.mu.Unlock()
		return nil
	}
	if v.unlocking {
		wait := v.wait
		v.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
			v.mu.Lock()
			err := v.unlockErr
			if err == nil && len(v.key) == 0 {
				err = ErrVaultLocked
			}
			v.mu.Unlock()
			return err
		}
	}
	v.unlocking = true
	v.wait = make(chan struct{})
	wait := v.wait
	generation := v.generation
	v.mu.Unlock()

	err := v.unlock(ctx, generation)

	v.mu.Lock()
	v.unlockErr = err
	v.unlocking = false
	close(wait)
	v.mu.Unlock()
	return err
}

func (v *VaultStore) unlock(ctx context.Context, generation uint64) error {
	data, err := os.ReadFile(v.path)
	creating := errors.Is(err, os.ErrNotExist)
	if err != nil && !creating {
		return fmt.Errorf("cloudfox: read portable vault: %w", err)
	}

	if creating {
		if v.prompt == nil {
			return ErrVaultLocked
		}
		password, err := v.prompt.PromptMasterPassword(ctx, true)
		if err != nil {
			return err
		}
		salt := make([]byte, vaultSaltSize)
		if _, err := rand.Read(salt); err != nil {
			return fmt.Errorf("cloudfox: generate vault salt: %w", err)
		}
		key := deriveVaultKey(password, salt, vaultKDFTime, vaultKDFMemoryKiB, vaultKDFThreads)
		v.mu.Lock()
		if v.generation != generation {
			v.mu.Unlock()
			zero(key)
			return ErrVaultLocked
		}
		v.key, v.salt, v.secrets = key, salt, make(map[string]SecretValues)
		v.mu.Unlock()
		return nil
	}

	envelope, salt, nonce, ciphertext, err := parseVaultEnvelope(data)
	if err != nil {
		return err
	}
	// An empty master password provides no access control: every value needed to
	// derive its key is already stored in the vault envelope. Try that known key
	// first so opening a saved connection never displays a meaningless password
	// prompt for an intentionally unprotected vault. Existing protected vaults
	// still fall through to the normal masked prompt.
	key, secrets, err := decryptVaultEnvelope("", envelope, salt, nonce, ciphertext)
	if err == nil {
		return v.installUnlockedVault(generation, key, salt, secrets)
	}
	if !errors.Is(err, ErrWrongMasterPassword) {
		return err
	}
	if v.prompt == nil {
		return ErrVaultLocked
	}
	password, err := v.prompt.PromptMasterPassword(ctx, false)
	if err != nil {
		return err
	}
	key, secrets, err = decryptVaultEnvelope(password, envelope, salt, nonce, ciphertext)
	if err != nil {
		return err
	}
	return v.installUnlockedVault(generation, key, salt, secrets)
}

func decryptVaultEnvelope(password string, envelope vaultEnvelope, salt, nonce, ciphertext []byte) ([]byte, map[string]SecretValues, error) {
	key := deriveVaultKey(password, salt, envelope.Time, envelope.MemoryKiB, envelope.Threads)
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		zero(key)
		return nil, nil, fmt.Errorf("cloudfox: initialize vault cipher: %w", err)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, vaultAAD(envelope))
	if err != nil {
		zero(key)
		return nil, nil, ErrWrongMasterPassword
	}
	defer zero(plain)
	var payload vaultPayload
	if err := json.Unmarshal(plain, &payload); err != nil || payload.Version != vaultVersion || payload.Secrets == nil {
		zero(key)
		return nil, nil, ErrVaultCorrupt
	}
	return key, cloneSecretMap(payload.Secrets), nil
}

func (v *VaultStore) installUnlockedVault(generation uint64, key, salt []byte, secrets map[string]SecretValues) error {
	v.mu.Lock()
	if v.generation != generation {
		v.mu.Unlock()
		zero(key)
		return ErrVaultLocked
	}
	v.key, v.salt, v.secrets = key, salt, secrets
	v.mu.Unlock()
	return nil
}

func parseVaultEnvelope(data []byte) (vaultEnvelope, []byte, []byte, []byte, error) {
	var envelope vaultEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return envelope, nil, nil, nil, fmt.Errorf("%w: %v", ErrVaultCorrupt, err)
	}
	if envelope.Version != vaultVersion || envelope.KDF != "argon2id" || envelope.Cipher != "xchacha20-poly1305" || envelope.Time == 0 || envelope.Time > 10 || envelope.MemoryKiB < 8*1024 || envelope.MemoryKiB > 1024*1024 || envelope.Threads == 0 || envelope.Threads > 32 {
		return envelope, nil, nil, nil, ErrVaultCorrupt
	}
	salt, err := base64.RawStdEncoding.DecodeString(envelope.Salt)
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return envelope, nil, nil, nil, ErrVaultCorrupt
	}
	nonce, err := base64.RawStdEncoding.DecodeString(envelope.Nonce)
	if err != nil || len(nonce) != chacha20poly1305.NonceSizeX {
		return envelope, nil, nil, nil, ErrVaultCorrupt
	}
	ciphertext, err := base64.RawStdEncoding.DecodeString(envelope.Ciphertext)
	if err != nil || len(ciphertext) < chacha20poly1305.Overhead {
		return envelope, nil, nil, nil, ErrVaultCorrupt
	}
	return envelope, salt, nonce, ciphertext, nil
}

// readLatestLocked reloads the last atomically committed payload while the
// caller holds both the interprocess file lock and v.mu. This is what keeps two
// f4 processes from replacing one another's whole-file vault updates.
func (v *VaultStore) readLatestLocked() (map[string]SecretValues, error) {
	data, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return cloneSecretMap(v.secrets), nil
	}
	if err != nil {
		return nil, fmt.Errorf("cloudfox: read portable vault: %w", err)
	}
	envelope, salt, nonce, ciphertext, err := parseVaultEnvelope(data)
	if err != nil {
		return nil, err
	}
	if len(v.salt) == 0 || subtle.ConstantTimeCompare(salt, v.salt) != 1 {
		zero(v.key)
		v.key = nil
		v.salt = nil
		v.secrets = nil
		return nil, ErrVaultLocked
	}
	aead, err := chacha20poly1305.NewX(v.key)
	if err != nil {
		return nil, fmt.Errorf("cloudfox: initialize vault cipher: %w", err)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, vaultAAD(envelope))
	if err != nil {
		return nil, ErrVaultCorrupt
	}
	defer zero(plain)
	var payload vaultPayload
	if err := json.Unmarshal(plain, &payload); err != nil || payload.Version != vaultVersion || payload.Secrets == nil {
		return nil, ErrVaultCorrupt
	}
	return cloneSecretMap(payload.Secrets), nil
}

func (v *VaultStore) writeLocked(secrets map[string]SecretValues, key, salt []byte) error {
	if len(key) != chacha20poly1305.KeySize || len(salt) == 0 {
		return ErrVaultLocked
	}
	payload, err := json.Marshal(vaultPayload{Version: vaultVersion, Secrets: cloneSecretMap(secrets)})
	if err != nil {
		return fmt.Errorf("cloudfox: encode vault payload: %w", err)
	}
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return fmt.Errorf("cloudfox: initialize vault cipher: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("cloudfox: generate vault nonce: %w", err)
	}
	envelope := vaultEnvelope{
		Version: vaultVersion, KDF: "argon2id", Time: vaultKDFTime,
		MemoryKiB: vaultKDFMemoryKiB, Threads: vaultKDFThreads,
		Salt: base64.RawStdEncoding.EncodeToString(salt), Cipher: "xchacha20-poly1305",
		Nonce: base64.RawStdEncoding.EncodeToString(nonce),
	}
	envelope.Ciphertext = base64.RawStdEncoding.EncodeToString(aead.Seal(nil, nonce, payload, vaultAAD(envelope)))
	return writeJSONAtomic(v.path, envelope)
}

func vaultAAD(e vaultEnvelope) []byte {
	return []byte(fmt.Sprintf("cloudfox-vault:%d:%s:%d:%d:%d:%s:%s", e.Version, e.KDF, e.Time, e.MemoryKiB, e.Threads, e.Salt, e.Cipher))
}

func deriveVaultKey(password string, salt []byte, time, memory uint32, threads uint8) []byte {
	return argon2.IDKey([]byte(password), salt, time, memory, threads, chacha20poly1305.KeySize)
}

func cloneSecretMap(values map[string]SecretValues) map[string]SecretValues {
	out := make(map[string]SecretValues, len(values))
	for key, value := range values {
		out[key] = value.Clone()
	}
	return out
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	// Keep the wipe observable to the compiler.
	_ = subtle.ConstantTimeByteEq(0, 0)
}
