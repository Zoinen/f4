package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// emptyWasmModule is the eight byte header of a valid, entirely empty
// WebAssembly module: it compiles and instantiates but never says anything.
var emptyWasmModule = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

func writeWasmModule(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writing module: %v", err)
	}
	return path
}

func TestIsWasmEntrypoint(t *testing.T) {
	cases := map[string]bool{
		"plugin.wasm":           true,
		"Plugin.WASM":           true,
		"build/plugin.wasm":     true,
		"wasmtime plugin.wasm":  false,
		"plugin.lua":            false,
		"plugin":                false,
		"":                      false,
		"/opt/plugins/net.wasm": true,
		"plugin.wasm --verbose": false,
	}
	for entrypoint, want := range cases {
		if got := IsWasmEntrypoint(entrypoint); got != want {
			t.Errorf("IsWasmEntrypoint(%q) = %v, want %v", entrypoint, got, want)
		}
	}
}

func TestNewPluginForEntrypointPicksWasm(t *testing.T) {
	if _, ok := newPluginForEntrypoint("", "plugin.wasm").(*WasmPlugin); !ok {
		t.Error("a bare wasm module did not get the wasm transport")
	}
	if _, ok := newPluginForEntrypoint("/plugins/x", "wasmtime plugin.wasm").(*RPCPlugin); !ok {
		t.Error("an entrypoint with arguments did not get the process transport")
	}
}

func TestNewPluginForEntrypointResolvesAgainstDir(t *testing.T) {
	plugin, ok := newPluginForEntrypoint("/plugins/netfox", "plugin.wasm").(*WasmPlugin)
	if !ok {
		t.Fatal("expected a wasm plugin")
	}
	if plugin.path != filepath.Join("/plugins/netfox", "plugin.wasm") {
		t.Errorf("path = %q, want it resolved against the plugin directory", plugin.path)
	}
}

func TestWasmPluginMissingFile(t *testing.T) {
	plugin := NewWasmPlugin(filepath.Join(t.TempDir(), "absent.wasm"))
	if err := plugin.Init(newLuaTestHostAPI()); err == nil {
		_ = plugin.Close() // Cleanup is secondary to the unexpected initialization success.
		t.Fatal("a missing module was loaded successfully")
	}
}

func TestWasmPluginRejectsGarbage(t *testing.T) {
	path := writeWasmModule(t, "broken.wasm", []byte("this is not a wasm module"))

	plugin := NewWasmPlugin(path)
	err := plugin.Init(newLuaTestHostAPI())
	if err == nil {
		_ = plugin.Close() // Cleanup is secondary to the unexpected initialization success.
		t.Fatal("an invalid module was loaded successfully")
	}
	if !strings.Contains(err.Error(), "compiling") {
		t.Errorf("error = %v, want it to name the compile step", err)
	}
}

func TestWasmPluginSilentModuleDoesNotHang(t *testing.T) {
	path := writeWasmModule(t, "silent.wasm", emptyWasmModule)

	restore := pluginInitTimeout
	pluginInitTimeout = 200 * time.Millisecond
	defer func() { pluginInitTimeout = restore }()

	plugin := NewWasmPlugin(path)
	start := time.Now()
	err := plugin.Init(newLuaTestHostAPI())
	elapsed := time.Since(start)

	if err == nil {
		_ = plugin.Close() // Cleanup is secondary to the unexpected initialization success.
		t.Fatal("a module that never answers was accepted")
	}
	// A module that exits without ever speaking either trips the handshake
	// timeout or breaks its pipe first, depending on which wins the race.
	// Either is fine; what matters is that startup is never left waiting.
	if elapsed > 5*time.Second {
		t.Errorf("Init took %v, nothing bounded the handshake", elapsed)
	}
}

func TestWasmPluginCloseIsIdempotent(t *testing.T) {
	path := writeWasmModule(t, "silent.wasm", emptyWasmModule)

	restore := pluginInitTimeout
	pluginInitTimeout = 200 * time.Millisecond
	defer func() { pluginInitTimeout = restore }()

	plugin := NewWasmPlugin(path)
	_ = plugin.Init(newLuaTestHostAPI())

	if err := plugin.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestWasmPluginName(t *testing.T) {
	plugin := NewWasmPlugin("/plugins/netfox/plugin.wasm")
	if !strings.Contains(plugin.GetName(), "wasm") {
		t.Errorf("GetName = %q, want it to identify the transport", plugin.GetName())
	}
}
