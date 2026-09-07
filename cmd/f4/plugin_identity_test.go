package main

import (
	"path/filepath"
	"testing"
)

// TestPlugRingPluginIsIdentifiedByItsCatalogID pins the key a grant is stored
// under. Keying on the plugin's file path looked harmless and was not:
// PlugRing removes a plugin by id, so the Forget on removal dropped a key
// nothing had ever written, and the grant outlived the plugin that earned it.
func TestPlugRingPluginIsIdentifiedByItsCatalogID(t *testing.T) {
	item := PlugRingItem{
		ID:          "notes",
		Name:        "Notes",
		Entrypoint:  "plugin.lua",
		Permissions: map[string]string{PermissionFFI: "to read the clipboard"},
	}

	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	identity := plugin.permissionIdentity()
	if identity.Key != item.ID {
		t.Errorf("grants would be stored under %q, want the catalog id %q", identity.Key, item.ID)
	}
	if identity.Name() != "Notes" {
		t.Errorf("the dialog would call the plugin %q, want the name from the manifest", identity.Name())
	}
	if identity.Declared[PermissionFFI] != "to read the clipboard" {
		t.Errorf("the author's reason did not reach the gate: %q", identity.Declared[PermissionFFI])
	}
}

// TestWasmPluginCarriesTheSameIdentity checks the other embedded transport,
// which keeps its own copy of this plumbing.
func TestWasmPluginCarriesTheSameIdentity(t *testing.T) {
	item := PlugRingItem{ID: "notes", Name: "Notes", Entrypoint: "plugin.wasm"}

	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*WasmPlugin)
	if !ok {
		t.Fatal("a bare .wasm entrypoint did not produce an embedded wasm plugin")
	}
	if key := plugin.permissionIdentity().Key; key != item.ID {
		t.Errorf("grants would be stored under %q, want the catalog id %q", key, item.ID)
	}
}

// TestIdentityIsSetWithoutDeclaredPermissions guards the case that produced
// the bug. A manifest declaring nothing still has to name its plugin, because
// the gate asks about permissions a plugin never declared as well.
func TestIdentityIsSetWithoutDeclaredPermissions(t *testing.T) {
	item := PlugRingItem{ID: "notes", Name: "Notes", Entrypoint: "plugin.lua"}

	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}
	if key := plugin.permissionIdentity().Key; key != item.ID {
		t.Errorf("a plugin declaring no permissions was identified as %q, want %q", key, item.ID)
	}
}

// TestRegisteredPluginFallsBackToItsPath covers the plugin that never went
// through a manifest: there is no id to key on, and where it lives is the only
// stable thing about it.
func TestRegisteredPluginFallsBackToItsPath(t *testing.T) {
	plugin, ok := newPluginForEntrypoint("", "/opt/f4/notes.lua").(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	identity := plugin.permissionIdentity()
	if identity.Key != "/opt/f4/notes.lua" {
		t.Errorf("identity.Key = %q, want the path", identity.Key)
	}
	if identity.Name() != "notes.lua" {
		t.Errorf("identity.Name() = %q, want the file name", identity.Name())
	}
}

// TestRemovingAPluginDropsTheGrantItEarned joins the two halves that used to
// disagree: the gate writes under the plugin's identity, and removal forgets
// the catalog id.
func TestRemovingAPluginDropsTheGrantItEarned(t *testing.T) {
	item := PlugRingItem{ID: "notes", Name: "Notes", Entrypoint: "plugin.lua"}
	store := LoadPermissionStore(filepath.Join(t.TempDir(), "perms.json"))

	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	gate := NewPermissionGate(plugin.permissionIdentity(), store, &fakePrompt{answer: true})
	if err := gate.Allow(PermissionFFI, "open libc.so.6"); err != nil {
		t.Fatalf("the user said yes and the gate refused anyway: %v", err)
	}

	// What actionRemovePlugRingItem does when the user confirms.
	if err := store.Forget(item.ID); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := store.Decision(item.ID, PermissionFFI); ok {
		t.Error("removing the plugin left its grant behind for whatever is installed next")
	}
}
