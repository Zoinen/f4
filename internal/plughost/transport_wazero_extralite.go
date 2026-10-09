//go:build extralite

package plughost

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// The extra-lite profile (unxed/f4#1671) carries no WebAssembly runtime: a
// wasm plugin is refused with a message instead of being run (and the Observer
// modules that need the same runtime are not offered, see
// plugins_observer_extralite.go). Lua and out-of-process plugins are separate
// transports.

// ErrWasmUnavailable is what a wasm plugin's Init returns in this build.
var ErrWasmUnavailable = errors.New("WASM plugins are not available in the extra-lite build")

// WasmPlugin is a placeholder that only reports ErrWasmUnavailable.
type WasmPlugin struct {
	path     string
	identity PluginIdentity
}

// NewWasmPlugin prepares the placeholder for a module.
func NewWasmPlugin(path string) *WasmPlugin { return &WasmPlugin{path: path} }

// SetPermissionIdentity keeps the identity, as the real transport does.
func (p *WasmPlugin) SetPermissionIdentity(identity PluginIdentity) { p.identity = identity }

func (p *WasmPlugin) GetName() string { return p.path + " (wasm)" }

func (p *WasmPlugin) Init(vfs.HostAPI) error { return ErrWasmUnavailable }

func (p *WasmPlugin) Close() error { return nil }
