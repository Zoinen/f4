package netfox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type NetFoxConfig struct {
	Type     string            `json:"Type"`
	Host     string            `json:"Host"`
	Port     string            `json:"Port"`
	User     string            `json:"User"`
	Pass     string            `json:"Pass"`
	KeyPath  string            `json:"KeyPath,omitempty"`
	Timeout  string            `json:"Timeout,omitempty"`
	Codepage string            `json:"Codepage,omitempty"`
	Options  map[string]string `json:"Options,omitempty"`

	// Proxy overrides f4's app-wide proxy for this site alone. ProxyMode 0
	// is netproxy.ModeGlobal, so connections saved before this existed —
	// and new ones the user never touched — simply follow the app setting.
	ProxyMode int    `json:"ProxyMode,omitempty"`
	ProxyHost string `json:"ProxyHost,omitempty"`
	ProxyPort string `json:"ProxyPort,omitempty"`
	ProxyUser string `json:"ProxyUser,omitempty"`
	ProxyPass string `json:"ProxyPass,omitempty"`

	autoSSHProfile bool `json:"-"`
}

// Proxy is the settings this connection dials through: its own when it
// overrides, the app-wide ones otherwise.
func (c NetFoxConfig) Proxy() netproxy.Settings {
	return netproxy.Resolve(netproxy.Settings{
		Mode: c.ProxyMode,
		Host: c.ProxyHost,
		Port: c.ProxyPort,
		User: c.ProxyUser,
		Pass: c.ProxyPass,
	})
}

type NetFoxVFS struct {
	mu           sync.Mutex
	path         string
	settingsPath string
	panelInfo    *netFoxManagerPanelInfoCache
}

const importSSHProfilesSettingID = "netfox.ImportSSHProfiles"

type netFoxPreferences struct {
	ImportSSHProfiles *bool `json:"ImportSSHProfiles,omitempty"`
}

func NewNetFoxVFS(dbPath string) *NetFoxVFS {
	_ = os.MkdirAll(filepath.Dir(dbPath), 0700)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		_ = writeNetFoxFile(dbPath, []byte("{}\n"))
	}
	return &NetFoxVFS{
		path:         dbPath,
		settingsPath: filepath.Join(filepath.Dir(dbPath), "NetFoxSettings.json"),
		panelInfo:    newNetFoxManagerPanelInfoCache(),
	}
}

func (v *NetFoxVFS) readConfigsLocked() (map[string]NetFoxConfig, error) {
	data, err := os.ReadFile(v.path)
	if errors.Is(err, os.ErrNotExist) {
		return make(map[string]NetFoxConfig), nil
	}
	if err != nil {
		return nil, fmt.Errorf("netfox: read connections: %w", err)
	}
	var configs map[string]NetFoxConfig
	if err := json.Unmarshal(data, &configs); err != nil {
		return nil, fmt.Errorf("netfox: damaged connections file: %w", err)
	}
	if configs == nil {
		return nil, errors.New("netfox: connections file is not a JSON object")
	}

	// Transparently decrypt passwords
	for k, cfg := range configs {
		if cfg.Pass != "" {
			cfg.Pass = deobfuscate(cfg.Pass)
		}
		if cfg.ProxyPass != "" {
			cfg.ProxyPass = deobfuscate(cfg.ProxyPass)
		}
		configs[k] = cfg
	}
	for k, cfg := range configs {
		if cfg.Codepage == "" {
			cfg.Codepage = "65001"
			configs[k] = cfg
		}
	}
	return configs, nil
}

func (v *NetFoxVFS) getConfigs() map[string]NetFoxConfig {
	v.mu.Lock()
	defer v.mu.Unlock()
	configs, err := v.readConfigsLocked()
	if err != nil {
		return make(map[string]NetFoxConfig)
	}
	if v.importSSHProfilesLocked() {
		v.mergeSSHProfilesLocked(configs)
	}
	if v.panelInfo != nil {
		v.panelInfo.replace(configs)
	}
	return configs
}

func (v *NetFoxVFS) settingsFilePath() string {
	if v.settingsPath != "" {
		return v.settingsPath
	}
	return filepath.Join(filepath.Dir(v.path), "NetFoxSettings.json")
}

func (v *NetFoxVFS) readPreferencesLocked() (netFoxPreferences, error) {
	preferences := netFoxPreferences{}
	data, err := os.ReadFile(v.settingsFilePath())
	if errors.Is(err, os.ErrNotExist) {
		return preferences, nil
	}
	if err != nil {
		return preferences, fmt.Errorf("netfox: read settings: %w", err)
	}
	if err := json.Unmarshal(data, &preferences); err != nil {
		return preferences, fmt.Errorf("netfox: decode settings: %w", err)
	}
	return preferences, nil
}

func (v *NetFoxVFS) importSSHProfilesLocked() bool {
	preferences, err := v.readPreferencesLocked()
	if err != nil || preferences.ImportSSHProfiles == nil {
		return true
	}
	return *preferences.ImportSSHProfiles
}

func (v *NetFoxVFS) savePreferences(preferences netFoxPreferences) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	value := true
	if preferences.ImportSSHProfiles != nil {
		value = *preferences.ImportSSHProfiles
	}
	data, err := json.MarshalIndent(netFoxPreferences{ImportSSHProfiles: &value}, "", "  ")
	if err != nil {
		return fmt.Errorf("netfox: encode settings: %w", err)
	}
	return writeNetFoxFile(v.settingsFilePath(), append(data, '\n'))
}

func (v *NetFoxVFS) mergeSSHProfilesLocked(configs map[string]NetFoxConfig) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	profiles, err := loadSSHProfiles(home)
	if err != nil {
		vtui.DebugLog("[FIX:netfox-ssh-config] cannot load profiles: %v", err)
		return
	}
	logSSHProfileLoad(len(profiles), filepath.Join(home, ".ssh", "config"))
	for name, profile := range profiles {
		if cfg, ok := configs[name]; ok {
			configs[name] = mergeSSHProfile(name, cfg, profile)
			continue
		}
		configs[name] = sshProfileConfig(profile)
	}
}

func (v *NetFoxVFS) isSSHProfile(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.isSSHProfileLocked(name)
}

func (v *NetFoxVFS) isSSHProfileLocked(name string) bool {
	if !v.importSSHProfilesLocked() {
		return false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return false
	}
	profiles, err := loadSSHProfiles(home)
	if err != nil {
		return false
	}
	_, ok := profiles[name]
	return ok
}

func (v *NetFoxVFS) isAutoSSHProfile(name string) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	configs, err := v.readConfigsLocked()
	if err != nil {
		return false
	}
	if _, stored := configs[name]; stored {
		return false
	}
	return v.isSSHProfileLocked(name)
}

func saveNetFoxConfigs(configs map[string]NetFoxConfig) ([]byte, error) {
	// Encrypt passwords before saving
	encodedConfigs := make(map[string]NetFoxConfig)
	for k, cfg := range configs {
		if cfg.Pass != "" {
			cfg.Pass = obfuscate(cfg.Pass)
		}
		if cfg.ProxyPass != "" {
			cfg.ProxyPass = obfuscate(cfg.ProxyPass)
		}
		encodedConfigs[k] = cfg
	}

	// Password fields have already been obfuscated above; this is the
	// persistence boundary for the encoded representation.
	data, err := json.MarshalIndent(encodedConfigs, "", "  ") // #nosec G117 -- secrets are obfuscated before serialization.
	if err != nil {
		return nil, fmt.Errorf("netfox: encode connections: %w", err)
	}
	return append(data, '\n'), nil
}

func writeNetFoxFile(path string, data []byte) (returnErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("netfox: create connections directory: %w", err)
	}
	f, err := os.CreateTemp(dir, ".netfox-*.tmp")
	if err != nil {
		return fmt.Errorf("netfox: create temporary connections file: %w", err)
	}
	tmpPath := f.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := f.Close(); returnErr == nil && closeErr != nil {
				returnErr = closeErr
			}
		}
		if returnErr != nil {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	for len(data) > 0 {
		written, err := f.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return nil
}

func (v *NetFoxVFS) updateConfigs(mutate func(map[string]NetFoxConfig) error) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	configs, err := v.readConfigsLocked()
	if err != nil {
		return err
	}
	if err := mutate(configs); err != nil {
		return err
	}
	data, err := saveNetFoxConfigs(configs)
	if err != nil {
		return err
	}
	return writeNetFoxFile(v.path, data)
}

func (v *NetFoxVFS) SaveConfig(name string, cfg NetFoxConfig) error {
	return v.updateConfigs(func(configs map[string]NetFoxConfig) error {
		configs[name] = cfg
		return nil
	})
}

func (v *NetFoxVFS) IsAtRoot() bool         { return true }
func (v *NetFoxVFS) GetPath() string        { return "net://" }
func (v *NetFoxVFS) IsAbs(p string) bool    { return strings.HasPrefix(p, "net://") }
func (v *NetFoxVFS) SetPath(p string) error { return nil }

// PanelTitle keeps the transport URI out of the user-facing root path
// control. Child connection paths remain fully qualified as net:// URIs.
func (v *NetFoxVFS) PanelTitle(path string) string {
	if path == "net://" {
		vtui.DebugLog("[FIX:netfox-path] pretty root title path=%q title=%q", path, "Network")
		return "Network"
	}
	return path
}

func netFoxSelectedConnection(req vfs.PanelInfoRequest) string {
	name := strings.TrimSpace(req.SelectedName)
	if name == "" {
		name = strings.TrimSpace(req.Path)
		if strings.HasPrefix(strings.ToLower(name), "net://") {
			_, device, remote, err := vfs.ParseDevicePath(name)
			if err == nil && remote == "/" {
				name = device
			}
		}
	}
	if name == "<Add connection>" {
		return ""
	}
	return name
}

// PanelInfoKey follows the selected connection row. The manager itself stays
// at net://, so using the panel path alone would never invalidate the cached
// facts when the cursor moves between saved connections.
func (v *NetFoxVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	name := netFoxSelectedConnection(req)
	if name == "" {
		return "netfox-manager"
	}
	return "netfox-manager:" + name
}

func (v *NetFoxVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	name := netFoxSelectedConnection(req)
	if name == "" {
		return vfs.PanelInfoSnapshot{Authoritative: true}, true
	}
	if snapshot, ok := v.panelInfo.snapshot(name); ok {
		vtui.DebugLog("[FIX:netfox-cache] manager cache hit connection=%q", name)
		return snapshot, true
	}
	vtui.DebugLog("[FIX:netfox-cache] manager cache miss connection=%q", name)
	return vfs.PanelInfoSnapshot{Authoritative: true}, false
}

func (v *NetFoxVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	// getConfigs refreshes the same in-memory snapshot used by ReadDir, so the
	// information panel never performs a second config-file read after a list.
	_ = v.getConfigs()
	snapshot, _ := v.CachedPanelInfo(req)
	return snapshot, nil
}

func (v *NetFoxVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	configs := v.getConfigs()
	var items []vfs.VFSItem
	items = append(items, vfs.VFSItem{Name: "<Add connection>", IconKey: "plus", NoExtension: true, IsExecutable: true})
	names := make([]string, 0, len(configs))
	for name := range configs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		items = append(items, vfs.VFSItem{Name: name, IsDir: false, IsExecutable: true})
	}
	if len(items) > 0 {
		onChunk(items)
	}
	return nil
}

func (v *NetFoxVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	name := v.Base(p)
	if name == "<Add connection>" {
		return vfs.VFSItem{Name: name, IconKey: "plus", NoExtension: true, IsExecutable: true}, nil
	}
	configs := v.getConfigs()
	if _, ok := configs[name]; ok {
		return vfs.VFSItem{Name: name, IsDir: false, IsExecutable: true}, nil
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (v *NetFoxVFS) Join(e ...string) string {
	if len(e) == 0 {
		return ""
	}
	if (e[0] == "net://" || e[0] == "net:/") && len(e) > 1 {
		device := vfs.DevicePath{Scheme: "net", Device: e[1]}
		if len(e) == 2 {
			return strings.TrimSuffix(device.Root(), "/")
		}
		return device.Public(path.Join(e[2:]...))
	}
	return path.Join(e...)
}
func (v *NetFoxVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.GetPath(), nil
	}
	return p, nil
}
func (v *NetFoxVFS) Base(p string) string {
	if vfs.IsURIPath(p) {
		if scheme, device, remote, err := vfs.ParseDevicePath(p); err == nil && strings.EqualFold(scheme, "net") {
			if remote == "/" {
				return device
			}
			return path.Base(remote)
		}
	}
	return path.Base(p)
}
func (v *NetFoxVFS) Dir(p string) string { return "net://" }

func (v *NetFoxVFS) MkDir(ctx context.Context, p string) error {
	return fmt.Errorf("folders in NetFox are not yet supported")
}

func (v *NetFoxVFS) Remove(ctx context.Context, p string) error {
	name := v.Base(p)
	if name == "<Add connection>" {
		return fmt.Errorf("cannot remove <Add connection>")
	}
	if v.isAutoSSHProfile(name) {
		return fmt.Errorf("cannot remove SSH profile %q from NetFox; edit ~/.ssh/config or disable SSH profile import", name)
	}
	return v.updateConfigs(func(configs map[string]NetFoxConfig) error {
		if _, ok := configs[name]; !ok {
			return os.ErrNotExist
		}
		delete(configs, name)
		return nil
	})
}

func (v *NetFoxVFS) Rename(ctx context.Context, old, new string) error {
	oldName := v.Base(old)
	newName := v.Base(new)
	if v.isAutoSSHProfile(oldName) {
		return fmt.Errorf("cannot rename SSH profile %q from NetFox; edit ~/.ssh/config or save a separate connection", oldName)
	}
	return v.updateConfigs(func(configs map[string]NetFoxConfig) error {
		cfg, ok := configs[oldName]
		if !ok {
			return os.ErrNotExist
		}
		configs[newName] = cfg
		delete(configs, oldName)
		return nil
	})
}

func (v *NetFoxVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	return os.ErrPermission
}

func (v *NetFoxVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: false, ReadAccess: vfs.ReadAccessMaterializeOnce, StorageClass: vfs.StorageClassNetwork}
}
func (v *NetFoxVFS) Search(ctx context.Context, p, pat string) (chan int64, error) { return nil, nil }

type bufferReadAtCloser struct{ *bytes.Reader }

func (b *bufferReadAtCloser) Close() error { return nil }
func (b *bufferReadAtCloser) Read(ctx context.Context, p []byte) (int, error) {
	return b.Reader.Read(p)
}
func (b *bufferReadAtCloser) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return b.Reader.ReadAt(p, off)
}
func (b *bufferReadAtCloser) Size() int64 { return int64(b.Len()) }

func (v *NetFoxVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	name := v.Base(p)
	if name == "<Add connection>" {
		return nil, os.ErrNotExist
	}
	configs := v.getConfigs()
	cfg, ok := configs[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	// #nosec G117 -- this user-opened virtual connection file intentionally exposes the owning user's editable connection fields.
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return &bufferReadAtCloser{Reader: bytes.NewReader(data)}, nil
}

type netfoxWriter struct {
	v    *NetFoxVFS
	name string
	buf  bytes.Buffer
}

func (w *netfoxWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *netfoxWriter) Close() error {
	var cfg NetFoxConfig
	if err := json.Unmarshal(w.buf.Bytes(), &cfg); err != nil {
		return fmt.Errorf("netfox: invalid connection JSON: %w", err)
	}
	return w.v.updateConfigs(func(configs map[string]NetFoxConfig) error {
		configs[w.name] = cfg
		return nil
	})
}
func (v *NetFoxVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	return &netfoxWriter{v: v, name: v.Base(p)}, nil
}
func (v *NetFoxVFS) ParentVFS() vfs.VFS { return nil }
func (v *NetFoxVFS) Close() error       { return nil }
func (v *NetFoxVFS) IsReadOnly() bool   { return true }
func (v *NetFoxVFS) Clone() vfs.VFS {
	clone := NewNetFoxVFS(v.path)
	clone.panelInfo = v.panelInfo
	return clone
}

func (*NetFoxVFS) PanelIcon() string { return "network" }

var (
	_ vfs.PanelInfoProvider  = (*NetFoxVFS)(nil)
	_ vfs.PanelTitleProvider = (*NetFoxVFS)(nil)
)
