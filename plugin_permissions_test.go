package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/ffibridge"
)

// fakePrompt answers without a terminal, and records what it was asked.
type fakePrompt struct {
	answer bool
	asked  []PermissionRequest
}

func (p *fakePrompt) Ask(req PermissionRequest) bool {
	p.asked = append(p.asked, req)
	return p.answer
}

func newTestStore(t *testing.T) *PermissionStore {
	t.Helper()
	return LoadPermissionStore(filepath.Join(t.TempDir(), "plugin_permissions.json"))
}

func TestPermissionGateAsksOnceAndRemembersYes(t *testing.T) {
	prompt := &fakePrompt{answer: true}
	store := newTestStore(t)
	gate := NewPermissionGate(PluginIdentity{Key: "notes", Title: "Notes", Declared: map[string]string{PermissionFFI: "to read the clipboard"}}, store, prompt)

	for i := 0; i < 3; i++ {
		if err := gate.Allow(PermissionFFI, "open libc.so.6"); err != nil {
			t.Fatalf("call %d was refused: %v", i, err)
		}
	}
	if len(prompt.asked) != 1 {
		t.Fatalf("the user was asked %d times, want once", len(prompt.asked))
	}
	if prompt.asked[0].Reason != "to read the clipboard" {
		t.Errorf("reason = %q, want the manifest text", prompt.asked[0].Reason)
	}

	// A fresh gate over the same store must not ask again.
	second := &fakePrompt{answer: false}
	again := NewPermissionGate(PluginIdentity{Key: "notes"}, store, second)
	if err := again.Allow(PermissionFFI, "open libc.so.6"); err != nil {
		t.Fatalf("a remembered grant was not honoured: %v", err)
	}
	if len(second.asked) != 0 {
		t.Error("the user was asked again after saying yes")
	}
}

func TestPermissionGateRefusalLastsOnlyForThisRun(t *testing.T) {
	prompt := &fakePrompt{answer: false}
	store := newTestStore(t)
	gate := NewPermissionGate(PluginIdentity{Key: "notes"}, store, prompt)

	if err := gate.Allow(PermissionFFI, "open libc.so.6"); err == nil {
		t.Fatal("a refusal was not enforced")
	}
	if err := gate.Allow(PermissionFFI, "open libc.so.6"); err == nil {
		t.Fatal("the refusal was forgotten within the same run")
	}
	if len(prompt.asked) != 1 {
		t.Errorf("the user was asked %d times after refusing, want once", len(prompt.asked))
	}

	// Nothing was written, so the next start asks again rather than leaving a
	// dead plugin with no way to revive it.
	if _, ok := store.Decision("notes", PermissionFFI); ok {
		t.Error("a refusal was recorded permanently")
	}
}

func TestPermissionGateWithoutAPrompt(t *testing.T) {
	gate := NewPermissionGate(PluginIdentity{Key: "notes"}, newTestStore(t), nil)
	if err := gate.Allow(PermissionFFI, "open libc.so.6"); err == nil {
		t.Fatal("a permission was granted with nobody to ask")
	}
}

func TestPermissionGatePerPlugin(t *testing.T) {
	prompt := &fakePrompt{answer: true}
	store := newTestStore(t)

	if err := NewPermissionGate(PluginIdentity{Key: "notes"}, store, prompt).Allow(PermissionFFI, ""); err != nil {
		t.Fatalf("notes: %v", err)
	}
	if err := NewPermissionGate(PluginIdentity{Key: "other"}, store, prompt).Allow(PermissionFFI, ""); err != nil {
		t.Fatalf("other: %v", err)
	}
	if len(prompt.asked) != 2 {
		t.Errorf("the user was asked %d times, want once per plugin", len(prompt.asked))
	}
}

func TestPermissionStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perms.json")

	store := LoadPermissionStore(path)
	if err := store.Remember("notes", PermissionFFI, PermissionAllow); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	reloaded := LoadPermissionStore(path)
	if decision, ok := reloaded.Decision("notes", PermissionFFI); !ok || decision != PermissionAllow {
		t.Fatalf("reloaded decision = %q, %v", decision, ok)
	}

	// Removing a plugin must not leave its grants behind for whatever is
	// installed under the same id next.
	if err := reloaded.Forget("notes"); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, ok := LoadPermissionStore(path).Decision("notes", PermissionFFI); ok {
		t.Error("a forgotten plugin kept its grants")
	}
}

func TestPermissionStoreSurvivesGarbage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "perms.json")
	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	store := LoadPermissionStore(path)
	if _, ok := store.Decision("notes", PermissionFFI); ok {
		t.Error("a corrupt store produced a decision")
	}
	if err := store.Remember("notes", PermissionFFI, PermissionAllow); err != nil {
		t.Fatalf("a corrupt store could not be written over: %v", err)
	}
}

func TestPermissionRequestText(t *testing.T) {
	withReason := PermissionRequestText(PermissionRequest{
		Plugin: "notes", Permission: PermissionFFI,
		Reason: "to show desktop notifications", Detail: "open libnotify.so.4",
	})
	for _, want := range []string{"notes", "native system libraries", "to show desktop notifications", "libnotify.so.4"} {
		if !strings.Contains(withReason, want) {
			t.Errorf("the dialog text does not mention %q:\n%s", want, withReason)
		}
	}

	// A plugin asking for something it never declared is not blocked, but the
	// silence is worth saying out loud.
	silent := PermissionRequestText(PermissionRequest{Plugin: "notes", Permission: PermissionFFI})
	if !strings.Contains(silent, "did not say why") {
		t.Errorf("the dialog does not report an undeclared permission:\n%s", silent)
	}
}

func TestGatedBridgeRefusesWhenDenied(t *testing.T) {
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}

	gate := NewPermissionGate(PluginIdentity{Key: "notes"}, newTestStore(t), &fakePrompt{answer: false})
	bridge := ffibridge.New(ffibridge.Options{Allow: gate.FFIHook()})
	defer bridge.Close()

	if _, err := bridge.OpenLibC(); err == nil {
		t.Fatal("a denied plugin opened a library")
	}
}

func TestGatedBridgeWorksWhenAllowed(t *testing.T) {
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}

	gate := NewPermissionGate(PluginIdentity{Key: "notes"}, newTestStore(t), &fakePrompt{answer: true})
	bridge := ffibridge.New(ffibridge.Options{Allow: gate.FFIHook()})
	defer bridge.Close()

	if _, err := bridge.OpenLibC(); err != nil {
		t.Skipf("no system C library available: %v", err)
	}
}

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
