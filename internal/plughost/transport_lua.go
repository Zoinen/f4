//go:build !extralite

package plughost

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/unxed/f4/internal/luaplug"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/ffibridge"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

// LuaPlugin runs a Lua plugin inside the f4 process.
//
// It is the same plugin as the out-of-process one, only without the process:
// the script still registers F4-RPC methods and still calls Host.* methods, so
// a plugin does not know or care which transport is carrying it. What it buys
// is distribution, since the user no longer needs a system Lua and a
// MessagePack rock for a plugin to run at all.
type LuaPlugin struct {
	path          string
	runtime       *luaplug.Runtime
	bridge        *ffibridge.Bridge
	host          map[string]f4rpc.Handler
	registrations *PluginSessionRegistrations
	// identity is who this plugin is to the permission model, taken from
	// the manifest when it came from the catalog.
	identity PluginIdentity

	// mu guards runtime, which restart replaces after a call deadline.
	mu sync.Mutex
	// allowUnsafe is what Init decided about Lua's os and io, kept so that a
	// restarted runtime is built the same way without asking again.
	allowUnsafe bool
	// callTimeout overrides luaplug's default; tests shorten it.
	callTimeout time.Duration
}

// SetPermissionIdentity passes on who the manifest says this plugin is, so
// that a grant is remembered under the id PlugRing installed it under, and
// the dialog can quote the author instead of guessing.
func (p *LuaPlugin) SetPermissionIdentity(identity PluginIdentity) {
	p.identity = identity
}

// permissionIdentity falls back to the path for a plugin registered by hand,
// which has no manifest and therefore no id.
func (p *LuaPlugin) permissionIdentity() PluginIdentity {
	if p.identity.Key == "" {
		return PermissionIdentityForPath(p.path)
	}
	return p.identity
}

// NewLuaPlugin prepares a plugin from a Lua script.
func NewLuaPlugin(path string) *LuaPlugin {
	return &LuaPlugin{path: path}
}

func (p *LuaPlugin) GetName() string {
	return p.path + " (Lua)"
}

func (p *LuaPlugin) Init(api vfs.HostAPI) error {
	gate := newPluginGate(p.permissionIdentity())
	p.bridge = newGatedFFIBridge(gate)

	p.allowUnsafe = p.allowsUnsafeStdlib(gate)
	runtime, err := p.newRuntime()
	if err != nil {
		p.bridge.Close()
		return err
	}
	p.setRuntime(runtime)

	// The host methods must exist before the script body runs: a plugin is
	// free to log or ask for its version while it is still loading.
	p.registrations = &PluginSessionRegistrations{}
	guard := newUIGuard(p)
	p.host = newHostMethods(api, guard, p.path, p.bridge)

	if err := runtime.LoadFile(p.path); err != nil {
		p.Close()
		return fmt.Errorf("loading %s: %w", p.path, err)
	}

	var res PluginInitResponse
	if err := p.Call("Plugin.Init", nil, &res); err != nil {
		p.Close()
		return fmt.Errorf("Plugin.Init failed: %w", err)
	}
	if err := RegisterRPCPluginCommands(api, guard, p.path, res.Commands, p.registrations); err != nil {
		p.Close()
		return err
	}

	for _, drive := range res.Drives {
		driveName := drive
		api.RegisterDrive(driveName, func() vfs.VFS {
			return NewRPCVFS(guard, driveName)
		})
	}
	return nil
}

func (p *LuaPlugin) newRuntime() (*luaplug.Runtime, error) {
	return luaplug.New(luaplug.Options{
		Name:              filepath.Base(p.path),
		Host:              luaplug.HostFunc(p.callHost),
		FFI:               p.bridge,
		AllowUnsafeStdlib: p.allowUnsafe,
		CallTimeout:       p.callTimeout,
	})
}

func (p *LuaPlugin) setRuntime(r *luaplug.Runtime) {
	p.mu.Lock()
	p.runtime = r
	p.mu.Unlock()
}

func (p *LuaPlugin) currentRuntime() *luaplug.Runtime {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.runtime
}

// restart replaces a runtime that hit its call deadline (luaplug.ErrInterrupted)
// with a new one running the same script: the old interpreter was stopped at an
// arbitrary instruction and refuses all work. The plugin's commands and drives
// stay registered with the host and reach the new runtime through the same
// plugin object; the script's own state starts over, as after a restart.
func (p *LuaPlugin) restart(old *luaplug.Runtime) (*luaplug.Runtime, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runtime != old {
		return p.runtime, nil // another call already did it
	}
	vtui.DebugLog("PLUGIN: %s hit its call deadline; restarting its Lua runtime", p.path)
	fresh, err := p.newRuntime()
	if err != nil {
		return nil, err
	}
	if err := fresh.LoadFile(p.path); err != nil {
		_ = fresh.Close()
		return nil, fmt.Errorf("reloading %s: %w", p.path, err)
	}
	// A plugin may register handlers from inside Plugin.Init, so it runs
	// again as it did at startup.
	if _, err := fresh.Dispatch("Plugin.Init", nil); err != nil {
		_ = fresh.Close()
		return nil, fmt.Errorf("Plugin.Init after the restart of %s: %w", p.path, err)
	}
	p.runtime = fresh
	go func() { _ = old.Close() }()
	return fresh, nil
}

// allowsUnsafeStdlib decides whether this plugin gets Lua's os and io.
//
// It is the one permission that cannot be asked for at the moment of use the
// way FFI is: gopher-lua builds a state's globals when the state is created,
// so there is no later point at which os and io could appear. Asking at load
// is the honest version of that constraint. The plugin has not started yet,
// so a refusal costs it nothing it had already begun.
//
// A plugin that never declared the permission is not asked at all. The dialog
// would be a question about something the author never claimed to need, at a
// moment when nothing has happened, and the only answer it could give to
// "why does it want this" is that no reason was given. Such a plugin gets the
// sandbox, which is what it asked for by saying nothing.
//
// A refusal is not fatal. Plugins reach for io on paths they rarely take, and
// failing the load would turn "not now" into "uninstall".
func (p *LuaPlugin) allowsUnsafeStdlib(gate *PermissionGate) bool {
	if _, declared := p.permissionIdentity().Declared[PermissionUnsafeStdlib]; !declared {
		return false
	}
	if err := gate.Allow(PermissionUnsafeStdlib, "load with the os and io libraries available"); err != nil {
		vtui.DebugLog("PERMISSIONS: %s runs without os and io: %v", p.path, err)
		return false
	}
	return true
}

// Call implements PluginTransport: a request from f4 into the plugin.
func (p *LuaPlugin) Call(method string, params any, result any) error {
	runtime := p.currentRuntime()
	if runtime == nil {
		return fmt.Errorf("lua plugin %s is not running", p.path)
	}
	if runtime.Interrupted() {
		var err error
		if runtime, err = p.restart(runtime); err != nil {
			return err
		}
	}
	value, err := runtime.Dispatch(method, params)
	if err != nil {
		return err
	}
	return decodePluginValue(value, result)
}

// callHost is the other direction: the plugin calling f4.
func (p *LuaPlugin) callHost(method string, params any) (any, error) {
	handler, ok := p.host[method]
	if !ok {
		return nil, fmt.Errorf("unknown host method %q", method)
	}

	var raw msgpack.RawMessage
	if params != nil {
		encoded, err := msgpack.Marshal(params)
		if err != nil {
			return nil, err
		}
		raw = encoded
	}
	return handler(raw)
}

func (p *LuaPlugin) Close() error {
	if p.registrations != nil {
		p.registrations.Unregister()
		p.registrations = nil
	}
	if r := p.currentRuntime(); r != nil {
		_ = r.Close()
	}
	if p.bridge != nil {
		p.bridge.Close()
	}
	return nil
}
