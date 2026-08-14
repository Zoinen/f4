package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"golang.org/x/oauth2"
)

const (
	cloudFoxAddConnectionCommandID    = "cloudfox.add-connection"
	cloudFoxEditConnectionCommandID   = "cloudfox.edit-connection"
	cloudFoxDeleteConnectionCommandID = "cloudfox.delete-connection"
)

// Options configures CloudFox without performing network I/O.
type Options struct {
	ConfigDir      string
	MetadataPath   string
	VaultPath      string
	Portable       bool
	Factories      []BackendFactory
	Keyring        SecretStore
	Vault          SecretStore
	PasswordPrompt MasterPasswordPrompter
	Editor         ProfileEditor
	Strings        ManagerStrings
}

func DefaultOptions() Options {
	configDir := vfs.CustomConfigDir
	if configDir == "" {
		if userDir, err := os.UserConfigDir(); err == nil {
			configDir = filepath.Join(userDir, "f4")
		} else {
			configDir = "."
		}
	}
	return Options{
		ConfigDir: configDir,
		Strings:   defaultManagerStrings(),
		Factories: []BackendFactory{
			&GoogleDriveFactory{},
			&YandexDiskFactory{},
			&S3Factory{},
			&WebDAVFactory{},
		},
	}
}

// Plugin owns shared stores, factories and remote sessions.
type Plugin struct {
	repo     *Repository
	vault    *VaultStore
	pool     *sessionPool
	editor   ProfileEditor
	strings  ManagerStrings
	portable bool

	mu        sync.RWMutex
	factories map[ProviderType]BackendFactory

	registrations []vfs.Registration
}

// NewPlugin accepts zero or one Options value. The variadic form preserves a
// convenient built-in plugin constructor while allowing fully injected tests.
func NewPlugin(values ...Options) *Plugin {
	opts := DefaultOptions()
	if len(values) != 0 {
		provided := values[0]
		if provided.ConfigDir != "" {
			opts.ConfigDir = provided.ConfigDir
		}
		if provided.MetadataPath != "" {
			opts.MetadataPath = provided.MetadataPath
		}
		if provided.VaultPath != "" {
			opts.VaultPath = provided.VaultPath
		}
		opts.Portable = provided.Portable
		if provided.Factories != nil {
			opts.Factories = provided.Factories
		}
		opts.Keyring = provided.Keyring
		opts.Vault = provided.Vault
		opts.PasswordPrompt = provided.PasswordPrompt
		opts.Editor = provided.Editor
		if provided.Strings.AddConnection != "" {
			opts.Strings = provided.Strings
		}
	}
	if opts.MetadataPath == "" {
		opts.MetadataPath = filepath.Join(opts.ConfigDir, "CloudFox.json")
	}
	if opts.VaultPath == "" {
		opts.VaultPath = filepath.Join(opts.ConfigDir, "CloudFox.vault")
	}
	if opts.Strings.AddConnection == "" {
		opts.Strings = defaultManagerStrings()
	}
	if opts.PasswordPrompt == nil {
		opts.PasswordPrompt = vtuiMasterPasswordPrompter{}
	}
	keyringStore := opts.Keyring
	if keyringStore == nil {
		// Portable mode controls where CloudFox writes new credentials (the
		// profile dialog forces its local vault), but existing metadata can still
		// contain a keyring:v1 reference after a profile-mode switch or copy.
		// Retain access to the OS keyring so such a connection remains usable on
		// the same machine; new portable credentials are still saved to the vault.
		keyringStore = NewKeyringStore()
	}
	var vaultStore SecretStore = opts.Vault
	var concreteVault *VaultStore
	if vaultStore == nil {
		concreteVault = NewVaultStore(opts.VaultPath, opts.PasswordPrompt)
		vaultStore = concreteVault
	} else {
		concreteVault, _ = vaultStore.(*VaultStore)
	}
	p := &Plugin{
		repo:  &Repository{Connections: NewConnectionStore(opts.MetadataPath), Secrets: SecretStores{Keyring: keyringStore, Vault: vaultStore}},
		vault: concreteVault, pool: newSessionPool(), editor: opts.Editor, strings: opts.Strings, portable: opts.Portable,
		factories: make(map[ProviderType]BackendFactory),
	}
	if p.editor == nil {
		p.editor = &simpleProfileEditor{plugin: p}
	}
	for _, factory := range opts.Factories {
		registered := factory
		if googleFactory, ok := factory.(*GoogleDriveFactory); ok {
			// Options are caller-owned and may be reused for multiple plugin
			// instances. Each plugin needs its own token callback/repository, so
			// never mutate or register the shared factory pointer in place.
			clone := *googleFactory
			if clone.TokenUpdate == nil {
				clone.TokenUpdate = p.persistGoogleToken
			}
			registered = &clone
		}
		_ = p.RegisterFactory(registered)
	}
	return p
}

func (p *Plugin) persistGoogleToken(ctx context.Context, connection Connection, token *oauth2.Token) (Connection, error) {
	if token == nil || connection.ID == "" || connection.SecretRef == "" {
		return Connection{}, nil
	}
	latest, err := p.repo.Get(ctx, connection.ID)
	if err != nil {
		return Connection{}, err
	}
	// A credential rotation means this live session belongs to an older
	// authorization (possibly another account). Never mix its refreshed token
	// into the newly committed credential bundle.
	if latest.SecretRef != connection.SecretRef || !sameGoogleOAuthAudience(connection, latest) {
		return Connection{}, nil
	}
	values, err := p.repo.Credentials(ctx, latest)
	if err != nil {
		return Connection{}, err
	}
	values["access_token"] = token.AccessToken
	if token.RefreshToken != "" {
		values["refresh_token"] = token.RefreshToken
	}
	if !token.Expiry.IsZero() {
		values["expires_at"] = token.Expiry.UTC().Format(time.RFC3339Nano)
	}
	storage := SecretStorageKeyring
	if strings.HasPrefix(latest.SecretRef, "vault:") {
		storage = SecretStorageVault
	}
	saved, swapped, err := p.repo.RotateSecretsIfCurrent(ctx, latest.ID, latest.SecretRef, latest.UpdatedAt, values, storage)
	clearSecrets(values)
	if err != nil || !swapped {
		return Connection{}, err
	}
	return saved, nil
}

func sameGoogleOAuthAudience(first, second Connection) bool {
	factory := GoogleDriveFactory{}
	firstSettings, firstErr := factory.settings(first)
	secondSettings, secondErr := factory.settings(second)
	return firstErr == nil && secondErr == nil &&
		strings.TrimSpace(firstSettings.ClientID) == strings.TrimSpace(secondSettings.ClientID)
}

func (p *Plugin) Repository() *Repository { return p.repo }

func (p *Plugin) RegisterFactory(factory BackendFactory) error {
	if factory == nil {
		return errors.New("cloudfox: cannot register a nil backend factory")
	}
	provider := factory.Provider()
	if !provider.Valid() {
		return fmt.Errorf("cloudfox: invalid backend provider %q", provider)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.factories[provider]; exists {
		return fmt.Errorf("cloudfox: backend factory for %q is already registered", provider)
	}
	p.factories[provider] = factory
	return nil
}

func (p *Plugin) Factory(provider ProviderType) (BackendFactory, bool) {
	p.mu.RLock()
	factory, ok := p.factories[provider]
	p.mu.RUnlock()
	return factory, ok
}

func (p *Plugin) Providers() []ProviderType {
	p.mu.RLock()
	providers := make([]ProviderType, 0, len(p.factories))
	for provider := range p.factories {
		providers = append(providers, provider)
	}
	p.mu.RUnlock()
	sort.Slice(providers, func(i, j int) bool { return providers[i] < providers[j] })
	return providers
}

func (p *Plugin) manager() *ManagerVFS {
	return newManagerVFS(p, p.repo, p.editor, p.strings)
}

func activeCloudFoxManager(app vfs.App) (*ManagerVFS, bool) {
	if app == nil {
		return nil, false
	}
	manager, ok := app.GetActivePanelVFS().(*ManagerVFS)
	return manager, ok && manager != nil
}

func cloudFoxManagerPaths(app vfs.App, manager *ManagerVFS) []string {
	if app == nil || manager == nil {
		return nil
	}
	names := app.GetSelectedNames()
	paths := make([]string, 0, len(names))
	for _, name := range names {
		if name == "" || name == ".." || name == manager.strings.AddConnection {
			continue
		}
		paths = append(paths, manager.Join(manager.GetPath(), name))
	}
	return paths
}

func cloudFoxAddConnectionVisible(app vfs.App) bool {
	_, ok := activeCloudFoxManager(app)
	return ok
}

func cloudFoxEditConnectionVisible(app vfs.App) bool {
	manager, ok := activeCloudFoxManager(app)
	return ok && len(cloudFoxManagerPaths(app, manager)) == 1
}

func cloudFoxDeleteConnectionVisible(app vfs.App) bool {
	manager, ok := activeCloudFoxManager(app)
	return ok && len(cloudFoxManagerPaths(app, manager)) != 0
}

func addCloudFoxConnection(app vfs.App) {
	manager, ok := activeCloudFoxManager(app)
	if !ok || manager.editor == nil {
		return
	}
	manager.editor.EditProfile(app, manager, nil)
}

func editCloudFoxConnection(app vfs.App) {
	manager, ok := activeCloudFoxManager(app)
	if !ok {
		return
	}
	paths := cloudFoxManagerPaths(app, manager)
	if len(paths) == 1 {
		manager.HandlePanelAction(app, vfs.PanelActionEdit, paths)
	}
}

func deleteCloudFoxConnections(app vfs.App) {
	manager, ok := activeCloudFoxManager(app)
	if !ok {
		return
	}
	if paths := cloudFoxManagerPaths(app, manager); len(paths) != 0 {
		manager.HandlePanelAction(app, vfs.PanelActionDelete, paths)
	}
}

func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("cloudfox: host API is nil")
	}
	registrations := make([]vfs.Registration, 0, 3)
	rollback := func(cause error) error {
		for index := len(registrations) - 1; index >= 0; index-- {
			registrations[index].Unregister()
		}
		return cause
	}
	if contributions, ok := api.(vfs.ContributionHost); ok {
		commands := []vfs.PluginCommand{
			{
				ID:             cloudFoxAddConnectionCommandID,
				Location:       vfs.PluginCommandPanel,
				Label:          "&Add cloud connection",
				LabelKey:       "CloudFox.Command.AddConnection",
				Description:    "Create a CloudFox storage connection",
				DescriptionKey: "CloudFox.Command.AddConnection.Desc",
				SearchTerms:    []string{"cloud storage", "Google Drive", "Yandex Disk", "S3", "WebDAV"},
				Shortcut:       "Shift+F4",
				Visible:        cloudFoxAddConnectionVisible,
				Run:            addCloudFoxConnection,
			},
			{
				ID:             cloudFoxEditConnectionCommandID,
				Location:       vfs.PluginCommandPanel,
				Label:          "&Edit cloud connection",
				LabelKey:       "CloudFox.Command.EditConnection",
				Description:    "Edit the selected CloudFox storage connection",
				DescriptionKey: "CloudFox.Command.EditConnection.Desc",
				SearchTerms:    []string{"cloud storage connection"},
				Shortcut:       "F4",
				Visible:        cloudFoxEditConnectionVisible,
				Run:            editCloudFoxConnection,
			},
			{
				ID:             cloudFoxDeleteConnectionCommandID,
				Location:       vfs.PluginCommandPanel,
				Label:          "&Delete cloud connection",
				LabelKey:       "CloudFox.Command.DeleteConnection",
				Description:    "Delete the selected CloudFox storage connections",
				DescriptionKey: "CloudFox.Command.DeleteConnection.Desc",
				SearchTerms:    []string{"cloud storage connection"},
				Shortcut:       "F8",
				Visible:        cloudFoxDeleteConnectionVisible,
				Run:            deleteCloudFoxConnections,
			},
		}
		for _, command := range commands {
			registration, err := contributions.RegisterPluginCommand(command)
			if err != nil {
				return rollback(fmt.Errorf("cloudfox: register command %q: %w", command.ID, err))
			}
			registrations = append(registrations, registration)
		}
	}
	if err := api.RegisterURIProvider(&cloudURIProvider{plugin: p}); err != nil {
		return rollback(err)
	}
	// URI registration is the last fallible step. Keep the irreversible VFS
	// and drive registrations after it so a duplicate scheme cannot leave a
	// half-installed panel provider; rich commands above can still roll back.
	api.RegisterVFSProvider(&connectionProvider{plugin: p})
	api.RegisterDrive(DriveName, func() vfs.VFS { return p.manager() })
	p.mu.Lock()
	p.registrations = append(p.registrations, registrations...)
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registrations := append([]vfs.Registration(nil), p.registrations...)
	p.registrations = nil
	p.mu.Unlock()
	for index := len(registrations) - 1; index >= 0; index-- {
		registrations[index].Unregister()
	}
	err := p.pool.close()
	if p.vault != nil {
		p.vault.Lock()
	}
	return err
}

func (p *Plugin) GetName() string { return DriveName }

func (p *Plugin) openConnection(ctx context.Context, manager *ManagerVFS, connection Connection, location string, validateLocation bool) (vfs.VFS, error) {
	factory, ok := p.Factory(connection.Provider)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrFactoryNotRegistered, connection.Provider)
	}
	if err := factory.Validate(connection.Clone()); err != nil {
		return nil, err
	}
	secrets, err := p.repo.Credentials(ctx, connection)
	if err != nil {
		return nil, err
	}
	session, err := p.pool.acquire(ctx, connection, func(openCtx context.Context) (Backend, error) {
		defer clearSecrets(secrets)
		return factory.Open(openCtx, connection.Clone(), secrets.Clone())
	})
	if err != nil {
		return nil, err
	}
	backend := session.backendSnapshot()
	if backend == nil {
		p.pool.release(session)
		return nil, os.ErrClosed
	}
	if location == "" {
		location = backend.Root()
	}
	cloud, err := newCloudVFS(connection, manager, session, location)
	if err != nil {
		p.pool.release(session)
		return nil, err
	}
	if validateLocation && !backend.IsRoot(cloud.currentLocation()) {
		statCtx, finishStat := providerOperationContext(ctx, p.pool.lifetime())
		entry, statErr := backend.Stat(statCtx, cloud.currentLocation())
		finishStat()
		if statErr != nil {
			_ = cloud.Close()
			return nil, statErr
		}
		if !entry.IsDir {
			_ = cloud.Close()
			return nil, fmt.Errorf("cloudfox: restored location is not a directory: %w", os.ErrInvalid)
		}
		// Stat may teach an opaque-ID backend a stable identity (for example by
		// resolving a pre-create Google Drive g:new token) and hydrate its display
		// hierarchy. Adopt that identity before exposing GetPath to panel history.
		canonical := cloud.rememberCanonicalLocation(backend, cloud.currentLocation())
		if entry.Location != "" {
			canonical = entry.Location
		}
		cloud.mu.Lock()
		cloud.location = canonical
		if entry.Location != "" {
			cloud.reverse[entry.Location] = entry.Name
			cloud.entries[entry.Location] = entry
		}
		cloud.mu.Unlock()
	}
	return wrapCloudVFS(cloud), nil
}

func clearSecrets(values SecretValues) {
	for key := range values {
		values[key] = ""
		delete(values, key)
	}
}

type connectionProvider struct{ plugin *Plugin }

func (*connectionProvider) Name() string                  { return "CloudFox-connection" }
func (*connectionProvider) Priority() int                 { return 220 }
func (*connectionProvider) OpensVirtualDirectories() bool { return true }
func (*connectionProvider) OpensStandalonePaths() bool    { return true }
func (p *connectionProvider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	// Navigation inside an already mounted CloudVFS (including "..") belongs
	// to that VFS. Treating its visual absolute path as a standalone restore
	// remounts the connection and loses the child-name cursor target.
	if cloud := unwrapCloudVFS(parent); cloud != nil {
		if _, owned := cloud.visualAbsoluteParts(path); owned {
			return false
		}
	}
	manager, ok := parent.(*ManagerVFS)
	if ok && manager.plugin == p.plugin {
		connection, found := manager.connectionForPath(ctx, path)
		if found {
			_, found = p.plugin.Factory(connection.Provider)
			return found
		}
		// A history/bookmark entry may point below a saved connection while the
		// panel currently shows the CloudFox manager. connectionForPath resolves
		// only a direct manager row (it intentionally uses Base), so let the
		// visual-path resolver recognize Account:\Folder instead of declaring the
		// valid entry unavailable and making history skip over it.
	}
	if path == managerVisualRoot() {
		return true
	}
	connection, _, ok := p.plugin.connectionForVisualPath(ctx, path)
	if !ok {
		return false
	}
	_, ok = p.plugin.Factory(connection.Provider)
	return ok
}
func (p *connectionProvider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	manager, ok := parent.(*ManagerVFS)
	if ok && manager.plugin == p.plugin {
		connection, found := manager.connectionForPath(ctx, path)
		if found {
			return p.plugin.openConnection(ctx, manager, connection, "", false)
		}
		// Fall through for a nested visual path. Keep this manager as ParentVFS,
		// but resolve the connection and its child components below.
	}
	if path == managerVisualRoot() {
		return p.plugin.manager(), nil
	}
	connection, parts, ok := p.plugin.connectionForVisualPath(ctx, path)
	if !ok {
		return nil, ErrConnectionNotFound
	}
	if manager == nil || manager.plugin != p.plugin {
		manager = p.plugin.manager()
	}
	opened, err := p.plugin.openConnection(ctx, manager, connection, "", false)
	if err != nil {
		return nil, err
	}
	cloud := unwrapCloudVFS(opened)
	if cloud == nil {
		_ = opened.Close()
		return nil, errors.New("cloudfox: opened connection did not return CloudVFS")
	}
	if err := cloud.restoreVisualPath(ctx, parts); err != nil {
		_ = opened.Close()
		return nil, err
	}
	return opened, nil
}

func (p *Plugin) connectionForVisualPath(ctx context.Context, raw string) (Connection, []string, bool) {
	connections, err := p.repo.List(ctx)
	if err != nil {
		return Connection{}, nil, false
	}
	best := -1
	var selected Connection
	for _, connection := range connections {
		prefix := connection.Name + ":"
		if !strings.HasPrefix(raw, prefix) {
			continue
		}
		rest := raw[len(prefix):]
		if rest != "" && rest[0] != '/' && rest[0] != '\\' {
			continue
		}
		if len(prefix) > best {
			best = len(prefix)
			selected = connection.Clone()
		}
	}
	if best < 0 {
		return Connection{}, nil, false
	}
	return selected, visualPathParts(raw[best:]), true
}

func unwrapCloudVFS(filesystem vfs.VFS) *CloudVFS {
	switch value := filesystem.(type) {
	case *CloudVFS:
		return value
	case *trashCloudVFS:
		return value.CloudVFS
	default:
		return nil
	}
}

type cloudURIProvider struct{ plugin *Plugin }

func (*cloudURIProvider) Scheme() string { return Scheme }
func (p *cloudURIProvider) OpenURI(ctx context.Context, _ vfs.VFS, rawURI string) (vfs.VFS, error) {
	u, err := ParseURI(rawURI)
	if err != nil {
		return nil, err
	}
	manager := p.plugin.manager()
	if u.Provider == "" {
		return manager, nil
	}
	connection, err := p.plugin.repo.Get(ctx, u.ConnectionID)
	if err != nil {
		return nil, err
	}
	if connection.Provider != u.Provider {
		return nil, fmt.Errorf("cloudfox: URI provider does not match profile")
	}
	return p.plugin.openConnection(ctx, manager, connection, u.Location, true)
}

var (
	_ vfs.VFSProvider              = (*connectionProvider)(nil)
	_ vfs.VirtualDirectoryProvider = (*connectionProvider)(nil)
	_ vfs.StandalonePathProvider   = (*connectionProvider)(nil)
	_ vfs.URIProvider              = (*cloudURIProvider)(nil)
)
