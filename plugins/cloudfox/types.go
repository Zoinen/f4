// Package cloudfox exposes network object stores as a top-level f4 drive.
//
// This package owns connection metadata, secret storage, canonical cloud URIs,
// session sharing and the common VFS adapter. Provider-specific HTTP clients
// implement BackendFactory and Backend and are deliberately kept separate.
package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	DriveName       = "CloudFox"
	Scheme          = "cloud"
	ManagerRoot     = "cloud://"
	AddConnectionID = "<cloudfox:add>"
	// AddConnectionLabel is also reserved as a profile name because it is the
	// visible manager row used by the built-in UI.
	AddConnectionLabel = "<Add connection>"
)

var (
	ErrManagerReadOnly         = errors.New("cloudfox: connection manager contents cannot be opened as files")
	ErrReservedName            = errors.New("cloudfox: reserved connection name")
	ErrDuplicateName           = errors.New("cloudfox: connection name already exists")
	ErrConnectionNotFound      = errors.New("cloudfox: connection not found")
	ErrConnectionChanged       = errors.New("cloudfox: connection changed in another task or process")
	ErrCredentialScopeUnbound  = errors.New("cloudfox: legacy credentials are not bound to this connection; re-enter or explicitly confirm them and save the profile")
	ErrCredentialScopeMismatch = errors.New("cloudfox: stored credentials are bound to a different connection scope")
	ErrFactoryNotRegistered    = errors.New("cloudfox: backend factory is not registered")
	ErrSecretNotFound          = errors.New("cloudfox: secret not found")
	ErrVaultLocked             = errors.New("cloudfox: portable vault is locked")
	ErrWrongMasterPassword     = errors.New("cloudfox: wrong master password or damaged vault")
	ErrVaultCorrupt            = errors.New("cloudfox: portable vault is damaged")
	ErrShareLinksUnsupported   = errors.New("cloudfox: this provider cannot create a share link for this item")
)

// ProviderType is the stable identifier serialized in profiles and cloud URIs.
type ProviderType string

const (
	ProviderGoogleDrive ProviderType = "gdrive"
	ProviderYandexDisk  ProviderType = "yandex"
	ProviderS3          ProviderType = "s3"
	ProviderWebDAV      ProviderType = "webdav"
)

func (p ProviderType) Valid() bool {
	switch p {
	case ProviderGoogleDrive, ProviderYandexDisk, ProviderS3, ProviderWebDAV:
		return true
	default:
		return false
	}
}

// Connection contains only non-secret profile data. Settings is owned by the
// selected BackendFactory and must never contain credentials or OAuth tokens.
type Connection struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Provider  ProviderType    `json:"provider"`
	Settings  json.RawMessage `json:"settings,omitempty"`
	SecretRef string          `json:"secret_ref,omitempty"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
}

func (c Connection) Clone() Connection {
	c.Settings = append(json.RawMessage(nil), c.Settings...)
	return c
}

// SecretValues is an extensible credential set. Provider implementations use
// stable field names (for example refresh_token or access_key_id).
type SecretValues map[string]string

func (s SecretValues) Clone() SecretValues {
	if s == nil {
		return nil
	}
	out := make(SecretValues, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

// RemoteEntry adds provider identity to the ordinary panel item. Location is
// an opaque, canonical path in the Backend's namespace; it is persisted in a
// cloud URI and may therefore be an immutable remote ID rather than a name.
type RemoteEntry struct {
	vfs.VFSItem
	Location     string
	TransferName string
	// SizeKnown distinguishes a reported zero-byte object from a provider
	// response which omitted its size. Backends that need a second request can
	// use it to validate that a representation was not truncated in transit.
	SizeKnown bool
	// Revision is a provider precondition token (for example a strong ETag).
	// It is kept out of persisted URIs and panel metadata and is used only to
	// prevent a multi-request reader from mixing object generations.
	Revision string
}

// BackendFactory validates profile settings and opens one authenticated
// provider session. Open must honor ctx and must not retain or mutate secrets.
type BackendFactory interface {
	Provider() ProviderType
	Validate(Connection) error
	Open(context.Context, Connection, SecretValues) (Backend, error)
}

// Backend is the common filesystem contract implemented by every provider.
// Its path helpers operate on opaque canonical locations. ReadDir entries must
// return a non-empty Location so CloudVFS can preserve identity across renames.
type Backend interface {
	io.Closer
	Root() string
	Normalize(string) (string, error)
	Join(string, ...string) string
	Base(string) string
	Dir(string) string
	IsRoot(string) bool
	ReadDir(context.Context, string, func([]RemoteEntry)) error
	Stat(context.Context, string) (RemoteEntry, error)
	MkDir(context.Context, string) error
	Remove(context.Context, string) error
	Rename(context.Context, string, string) error
	Open(context.Context, string) (vfs.ReadAtCloser, error)
	Create(context.Context, string) (io.WriteCloser, error)
	SetAttributes(context.Context, string, vfs.VFSItem) error
	Capabilities() vfs.VFSCapabilities
}

// BackendCopier marks server-side copies which do not transfer bytes through
// f4. CloudVFS exposes vfs.ServerSideCopier only through this capability.
type BackendCopier interface {
	Copy(context.Context, string, string) error
}

// BackendTrasher is implemented only when the remote service has a recoverable
// trash. Remove always means permanent deletion.
type BackendTrasher interface {
	MoveToTrash(context.Context, string) error
}

// BackendTransferNamer removes display-only disambiguators when a file is
// copied out of CloudFox.
type BackendTransferNamer interface {
	TransferName(string) string
}

// BackendIntraSessionNamer may preserve a provider-native name when a copy or
// move stays in the same authenticated session. Google Workspace objects, for
// example, need an export extension only when bytes leave Google Drive.
type BackendIntraSessionNamer interface {
	IntraSessionTransferName(string) string
}

// BackendCanonicalizer resolves a provider destination token to the stable
// identity learned after a successful mutation. Some APIs (notably Google
// Drive) assign an immutable object ID only when a create request commits, so
// the pre-create name token cannot itself be a durable canonical location.
// Implementations must be local, non-blocking, and safe for concurrent calls.
type BackendCanonicalizer interface {
	CanonicalLocation(string) string
}

// BackendPanelInfo supplies optional account/quota/bucket information.
type BackendPanelInfo interface {
	PanelInfoKey(vfs.PanelInfoRequest) string
	CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool)
	RefreshPanelInfo(context.Context, vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error)
}

// BackendShareLinker is the optional provider capability behind the common
// Files > Share dialog. It stays separate from Backend so read-only test
// backends and providers without link sharing do not need placeholder methods.
// URLs returned by these methods may be bearer credentials and must never be
// written to logs or wrapped into error strings.
type BackendShareLinker interface {
	ShareLinkInfo(context.Context, string) (vfs.ShareLinkInfo, error)
	CreateShareLink(context.Context, string, vfs.ShareLinkRequest) (vfs.ShareLink, error)
	RevokeShareLink(context.Context, string) error
}

// SecretStorage selects where newly supplied credentials are stored.
type SecretStorage string

const (
	SecretStorageKeyring SecretStorage = "keyring"
	SecretStorageVault   SecretStorage = "vault"
)

func validateConnection(c Connection) error {
	if c.ID != "" && !validUUID(c.ID) {
		return fmt.Errorf("cloudfox: invalid connection id %q", c.ID)
	}
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return errors.New("cloudfox: connection name cannot be empty")
	}
	// Connection names are also virtual drive names ("Name:\\" on Windows).
	// Keep the manager namespace and native single-letter drive namespace
	// unambiguous. Reserving ASCII drive letters on every platform also keeps a
	// profile created elsewhere safe to open on Windows later.
	if strings.EqualFold(name, AddConnectionID) ||
		strings.EqualFold(name, AddConnectionLabel) ||
		strings.EqualFold(name, DriveName) ||
		name == "." || name == ".." || isWindowsDriveLetter(name) {
		return ErrReservedName
	}
	if strings.ContainsAny(name, ":/\\\x00") {
		return errors.New("cloudfox: connection name contains a colon, path separator, or NUL")
	}
	if !c.Provider.Valid() {
		return fmt.Errorf("cloudfox: unsupported provider %q", c.Provider)
	}
	if len(c.Settings) != 0 && !json.Valid(c.Settings) {
		return errors.New("cloudfox: provider settings are not valid JSON")
	}
	return nil
}

func isWindowsDriveLetter(name string) bool {
	return len(name) == 1 && ((name[0] >= 'A' && name[0] <= 'Z') || (name[0] >= 'a' && name[0] <= 'z'))
}
