package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// luaTestHostAPI is a vfs.HostAPI that records what a plugin does instead of
// touching the real UI.
type luaTestHostAPI struct {
	logs   []string
	drives map[string]func() vfs.VFS
}

func newLuaTestHostAPI() *luaTestHostAPI {
	return &luaTestHostAPI{drives: make(map[string]func() vfs.VFS)}
}

func (h *luaTestHostAPI) GetVersion() string { return "test-version" }
func (h *luaTestHostAPI) Log(msg string)     { h.logs = append(h.logs, msg) }
func (h *luaTestHostAPI) Message(msg string) { h.logs = append(h.logs, msg) }

func (h *luaTestHostAPI) RegisterHighlighter(p vtui.HighlighterProvider) {}
func (h *luaTestHostAPI) RegisterVFSProvider(p vfs.VFSProvider)          {}
func (h *luaTestHostAPI) RegisterURIProvider(p vfs.URIProvider) error    { return nil }

func (h *luaTestHostAPI) RegisterDrive(name string, factory func() vfs.VFS) {
	h.drives[name] = factory
}

func (h *luaTestHostAPI) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app vfs.App)) {
}

func (h *luaTestHostAPI) RegisterPluginMenuItem(label string, handler func(app vfs.App)) {}

func (h *luaTestHostAPI) RunAction(name string) bool {
	h.logs = append(h.logs, "action:"+name)
	return true
}

func writeLuaPlugin(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugin.lua")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatalf("writing plugin: %v", err)
	}
	return path
}

func TestIsLuaEntrypoint(t *testing.T) {
	cases := map[string]bool{
		"plugin.lua":                   true,
		"Plugin.LUA":                   true,
		"scripts/plugin.lua":           true,
		"lua plugin.lua":               false,
		".venv/bin/python main.py":     false,
		"plugin":                       false,
		"plugin.py":                    false,
		"":                             false,
		"/opt/plugins/netfox/main.lua": true,
	}
	for entrypoint, want := range cases {
		if got := IsLuaEntrypoint(entrypoint); got != want {
			t.Errorf("IsLuaEntrypoint(%q) = %v, want %v", entrypoint, got, want)
		}
	}
}

func TestNewPluginForEntrypointPicksTransport(t *testing.T) {
	if _, ok := newPluginForEntrypoint("", "plugin.lua").(*LuaPlugin); !ok {
		t.Error("a bare Lua script did not get the embedded transport")
	}
	if _, ok := newPluginForEntrypoint("", "helper").(*RPCPlugin); !ok {
		t.Error("an executable did not get the process transport")
	}
	if _, ok := newPluginForEntrypoint("/plugins/x", "lua plugin.lua").(*RPCPlugin); !ok {
		t.Error("an entrypoint with arguments did not get the process transport")
	}
}

func TestLuaPluginMountsADrive(t *testing.T) {
	path := writeLuaPlugin(t, `
		local f4rpc = require('f4rpc')

		f4rpc.call("Host.Log", "lua plugin starting")

		f4rpc.register("Plugin.Init", function()
			return { Drives = { "Lua Test Drive" } }
		end)

		f4rpc.register("VFS.ReadDir", function(req)
			if req.Path == "/" or req.Path == "" then
				return {
					{ Name = "readme.txt", Size = 5, IsDir = false },
					{ Name = "sub", Size = 0, IsDir = true },
				}
			end
			return {}
		end)

		f4rpc.register("VFS.Open", function(req)
			return { ID = 7, Size = 5 }
		end)

		f4rpc.register("VFS.ReadAt", function(req)
			return "hello"
		end)

		f4rpc.register("VFS.CloseFile", function(req)
			return nil
		end)

		f4rpc.serve()
	`)

	api := newLuaTestHostAPI()
	plugin := NewLuaPlugin(path)
	if err := plugin.Init(api); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close Lua plugin: %v", err)
		}
	})

	factory, ok := api.drives["Lua Test Drive"]
	if !ok {
		t.Fatalf("plugin did not register its drive, got %v", api.drives)
	}

	var logged bool
	for _, entry := range api.logs {
		if strings.Contains(entry, "lua plugin starting") {
			logged = true
		}
	}
	if !logged {
		t.Errorf("host log did not receive the plugin message, got %v", api.logs)
	}

	fs := factory()
	ctx := context.Background()

	var items []vfs.VFSItem
	if err := fs.ReadDir(ctx, "/", func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ReadDir returned %d items, want 2", len(items))
	}
	if items[0].Name != "readme.txt" || items[0].Size != 5 || items[0].IsDir {
		t.Errorf("first item = %+v", items[0])
	}
	if items[1].Name != "sub" || !items[1].IsDir {
		t.Errorf("second item = %+v", items[1])
	}

	file, err := fs.Open(ctx, "readme.txt")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = file.Close() }()

	if file.Size() != 5 {
		t.Errorf("Size = %d, want 5", file.Size())
	}
	buf := make([]byte, 5)
	n, err := file.ReadAt(ctx, buf, 0)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Errorf("ReadAt returned %q, want \"hello\"", buf[:n])
	}
}

func TestLuaPluginReportsHostVersion(t *testing.T) {
	path := writeLuaPlugin(t, `
		local f4rpc = require('f4rpc')
		local version = f4rpc.call("Host.GetVersion")
		f4rpc.register("Plugin.Init", function()
			return { Drives = { version } }
		end)
	`)

	api := newLuaTestHostAPI()
	plugin := NewLuaPlugin(path)
	if err := plugin.Init(api); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close Lua plugin: %v", err)
		}
	})

	if _, ok := api.drives["test-version"]; !ok {
		t.Fatalf("Host.GetVersion did not reach the plugin, drives = %v", api.drives)
	}
}

func TestLuaPluginWithoutInitFails(t *testing.T) {
	path := writeLuaPlugin(t, `local f4rpc = require('f4rpc')`)

	plugin := NewLuaPlugin(path)
	if err := plugin.Init(newLuaTestHostAPI()); err == nil {
		_ = plugin.Close() // Cleanup is secondary to the unexpected initialization success.
		t.Fatal("a plugin without Plugin.Init was loaded successfully")
	}
}

func TestLuaPluginWithBrokenScriptFails(t *testing.T) {
	path := writeLuaPlugin(t, `this is not lua`)

	plugin := NewLuaPlugin(path)
	if err := plugin.Init(newLuaTestHostAPI()); err == nil {
		_ = plugin.Close() // Cleanup is secondary to the unexpected initialization success.
		t.Fatal("a syntactically invalid plugin was loaded successfully")
	}
}
