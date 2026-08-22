package netfox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/vfs"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

type NetFoxConfig struct {
	Type     string            `json:"Type"`
	Host     string            `json:"Host"`
	Port     string            `json:"Port"`
	User     string            `json:"User"`
	Pass     string            `json:"Pass"`
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
	mu   sync.Mutex
	path string
}

func NewNetFoxVFS(dbPath string) *NetFoxVFS {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		os.WriteFile(dbPath, []byte("{}"), 0644)
	}
	return &NetFoxVFS{path: dbPath}
}

func (v *NetFoxVFS) getConfigs() map[string]NetFoxConfig {
	v.mu.Lock()
	defer v.mu.Unlock()
	data, err := os.ReadFile(v.path)
	if err != nil {
		return make(map[string]NetFoxConfig)
	}
	var configs map[string]NetFoxConfig
	json.Unmarshal(data, &configs)
	if configs == nil {
		configs = make(map[string]NetFoxConfig)
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
	return configs
}

func (v *NetFoxVFS) saveConfigs(configs map[string]NetFoxConfig) {
	v.mu.Lock()
	defer v.mu.Unlock()
	os.MkdirAll(filepath.Dir(v.path), 0755)

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

	data, _ := json.MarshalIndent(encodedConfigs, "", "  ")
	os.WriteFile(v.path, data, 0644)
}

func (v *NetFoxVFS) SaveConfig(name string, cfg NetFoxConfig) {
	configs := v.getConfigs()
	configs[name] = cfg
	v.saveConfigs(configs)
}

func (v *NetFoxVFS) IsAtRoot() bool         { return true }
func (v *NetFoxVFS) GetPath() string        { return "net://" }
func (v *NetFoxVFS) IsAbs(p string) bool    { return strings.HasPrefix(p, "net://") }
func (v *NetFoxVFS) SetPath(p string) error { return nil }

func (v *NetFoxVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	configs := v.getConfigs()
	var items []vfs.VFSItem
	items = append(items, vfs.VFSItem{Name: "<Add connection>", IsDir: false, IsExecutable: true})
	for name := range configs {
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
		return vfs.VFSItem{Name: name, IsDir: false, IsExecutable: true}, nil
	}
	configs := v.getConfigs()
	if _, ok := configs[name]; ok {
		return vfs.VFSItem{Name: name, IsDir: false, IsExecutable: true}, nil
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (v *NetFoxVFS) Join(e ...string) string      { return path.Join(e...) }
func (v *NetFoxVFS) Abs(p string) (string, error) { return p, nil }
func (v *NetFoxVFS) Base(p string) string         { return path.Base(p) }
func (v *NetFoxVFS) Dir(p string) string          { return "net://" }

func (v *NetFoxVFS) MkDir(ctx context.Context, p string) error {
	return fmt.Errorf("folders in NetFox are not yet supported")
}

func (v *NetFoxVFS) Remove(ctx context.Context, p string) error {
	name := v.Base(p)
	if name == "<Add connection>" {
		return fmt.Errorf("cannot remove <Add connection>")
	}
	configs := v.getConfigs()
	delete(configs, name)
	v.saveConfigs(configs)
	return nil
}

func (v *NetFoxVFS) Rename(ctx context.Context, old, new string) error {
	oldName := v.Base(old)
	newName := v.Base(new)
	configs := v.getConfigs()
	if cfg, ok := configs[oldName]; ok {
		configs[newName] = cfg
		delete(configs, oldName)
		v.saveConfigs(configs)
	}
	return nil
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
func (b *bufferReadAtCloser) Size() int64 { return int64(b.Reader.Len()) }

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
	json.Unmarshal(w.buf.Bytes(), &cfg)
	configs := w.v.getConfigs()
	configs[w.name] = cfg
	w.v.saveConfigs(configs)
	return nil
}
func (v *NetFoxVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	return &netfoxWriter{v: v, name: v.Base(p)}, nil
}
func (v *NetFoxVFS) ParentVFS() vfs.VFS { return nil }
func (v *NetFoxVFS) Close() error       { return nil }
func (v *NetFoxVFS) IsReadOnly() bool   { return true }
func (v *NetFoxVFS) Clone() vfs.VFS {
	return NewNetFoxVFS(v.path)
}
