//go:build !extralite

package plughost

import "testing"

// TestUnsafeStdlibIsNotAskedForWhenUndeclared pins the one place the model
// deliberately stays quiet. The decision is taken before the interpreter
// exists, so the question would arrive before the plugin had done anything,
// about a permission its author never claimed to need.
func TestUnsafeStdlibIsNotAskedForWhenUndeclared(t *testing.T) {
	item := PlugRingItem{ID: "notes", Name: "Notes", Entrypoint: "plugin.lua"}
	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	prompt := &fakePrompt{answer: true}
	gate := NewPermissionGate(plugin.permissionIdentity(), newTestStore(t), prompt)

	if plugin.allowsUnsafeStdlib(gate) {
		t.Error("a plugin that never asked for os and io was given them")
	}
	if len(prompt.asked) != 0 {
		t.Errorf("the user was asked %d times about a permission nobody declared", len(prompt.asked))
	}
}

func TestUnsafeStdlibIsGrantedWhenDeclaredAndAllowed(t *testing.T) {
	item := PlugRingItem{
		ID: "notes", Name: "Notes", Entrypoint: "plugin.lua",
		Permissions: map[string]string{PermissionUnsafeStdlib: "to write its notes to disk"},
	}
	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	store := newTestStore(t)
	prompt := &fakePrompt{answer: true}
	if !plugin.allowsUnsafeStdlib(NewPermissionGate(plugin.permissionIdentity(), store, prompt)) {
		t.Fatal("a declared and allowed permission did not reach the interpreter")
	}
	if len(prompt.asked) != 1 {
		t.Fatalf("the user was asked %d times, want once", len(prompt.asked))
	}
	if prompt.asked[0].Reason != "to write its notes to disk" {
		t.Errorf("the dialog did not quote the manifest: %q", prompt.asked[0].Reason)
	}

	// Remembered under the catalog id, so the next start does not ask again.
	if decision, ok := store.Decision(item.ID, PermissionUnsafeStdlib); !ok || decision != PermissionAllow {
		t.Errorf("the grant was not remembered: %q, %v", decision, ok)
	}
}

func TestUnsafeStdlibRefusalLeavesThePluginSandboxed(t *testing.T) {
	item := PlugRingItem{
		ID: "notes", Name: "Notes", Entrypoint: "plugin.lua",
		Permissions: map[string]string{PermissionUnsafeStdlib: "to write its notes to disk"},
	}
	plugin, ok := newPluginForPlugRingItem("/plugins/notes", item).(*LuaPlugin)
	if !ok {
		t.Fatal("a bare .lua entrypoint did not produce an embedded Lua plugin")
	}

	gate := NewPermissionGate(plugin.permissionIdentity(), newTestStore(t), &fakePrompt{answer: false})
	if plugin.allowsUnsafeStdlib(gate) {
		t.Error("a refused plugin was given os and io anyway")
	}
}
