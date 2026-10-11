//go:build extralite

package plughost

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// The extra-lite profile (unxed/f4#1671) carries no Lua interpreter: a Lua
// plugin is refused with a message instead of being run. WASM and out-of-process
// plugins are not affected.

// ErrLuaUnavailable is what a Lua plugin's Init returns in this build.
var ErrLuaUnavailable = errors.New("Lua plugins are not available in the extra-lite build")

// LuaPlugin is a placeholder that only reports ErrLuaUnavailable.
type LuaPlugin struct {
	path     string
	identity PluginIdentity
}

// NewLuaPlugin prepares the placeholder for a Lua script.
func NewLuaPlugin(path string) *LuaPlugin { return &LuaPlugin{path: path} }

// SetPermissionIdentity keeps the identity, as the real transport does.
func (p *LuaPlugin) SetPermissionIdentity(identity PluginIdentity) { p.identity = identity }

func (p *LuaPlugin) GetName() string { return p.path + " (Lua)" }

func (p *LuaPlugin) Init(vfs.HostAPI) error { return ErrLuaUnavailable }

func (p *LuaPlugin) Close() error { return nil }
