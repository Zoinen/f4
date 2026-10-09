package plughost

import (
	"path/filepath"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

// IsLuaEntrypoint reports whether an entrypoint is a bare Lua script that the
// embedded interpreter can run.
func IsLuaEntrypoint(entrypoint string) bool {
	return isBareEntrypointWithExt(entrypoint, ".lua")
}

// IsWasmEntrypoint reports whether an entrypoint is a bare WebAssembly module.
func IsWasmEntrypoint(entrypoint string) bool {
	return isBareEntrypointWithExt(entrypoint, ".wasm")
}

// isBareEntrypointWithExt reports whether an entrypoint is a single file with
// the given extension. An entrypoint with arguments, such as "lua plugin.lua"
// or ".venv/bin/python main.py", asks for a process and gets one.
func isBareEntrypointWithExt(entrypoint, ext string) bool {
	fields := strings.Fields(entrypoint)
	if len(fields) != 1 {
		return false
	}
	return strings.EqualFold(filepath.Ext(fields[0]), ext)
}

// resolvePluginPath turns an entrypoint into a path, relative to the plugin's
// own directory when it has one.
func resolvePluginPath(dir, entrypoint string) string {
	path := strings.TrimSpace(entrypoint)
	if dir != "" && !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path
}

// newPluginForEntrypoint picks the transport an entrypoint asks for. dir is the
// plugin's own directory, empty for a plain registered path.
func newPluginForEntrypoint(dir, entrypoint string) Plugin {
	if IsLuaEntrypoint(entrypoint) {
		return NewLuaPlugin(resolvePluginPath(dir, entrypoint))
	}
	if IsWasmEntrypoint(entrypoint) {
		return NewWasmPlugin(resolvePluginPath(dir, entrypoint))
	}
	if dir == "" {
		return NewRPCPlugin(entrypoint)
	}
	return NewRPCPlugRing(dir, entrypoint)
}

// carriesPermissionIdentity is implemented by the transports that can be
// gated.
type carriesPermissionIdentity interface {
	SetPermissionIdentity(PluginIdentity)
}

// newPluginForPlugRingItem is newPluginForEntrypoint with the manifest in
// hand, which is the only place the declared permissions come from.
func newPluginForPlugRingItem(dir string, item PlugRingItem) Plugin {
	plugin := newPluginForEntrypoint(dir, item.Entrypoint)
	// Unconditionally: an identity is needed even when the manifest declares
	// no permissions, because the gate also asks about permissions a plugin
	// never declared.
	if aware, ok := plugin.(carriesPermissionIdentity); ok {
		aware.SetPermissionIdentity(PermissionIdentityForPlugRingItem(item))
	}
	return plugin
}

// decodePluginValue moves a value the interpreter produced into the typed
// struct the core expects. Routing it through MessagePack costs a round trip
// but guarantees that both transports agree on field naming, which is exactly
// where the older Far plugin APIs drifted apart.
func decodePluginValue(value any, result any) error {
	if value == nil || result == nil {
		return nil
	}
	encoded, err := msgpack.Marshal(value)
	if err != nil {
		return err
	}
	return msgpack.Unmarshal(encoded, result)
}
