package macro

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/keymap"
)

func TestMacroManagerReloadLuaMacros(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.lua")
	const first = `Macro { area = "Shell"; key = "CtrlJ"; action = function() end }`
	if err := os.WriteFile(path, []byte(first), 0o600); err != nil {
		t.Fatal(err)
	}

	mgr := &MacroManager{}
	t.Cleanup(func() {
		if mgr.Lua != nil {
			_ = mgr.Lua.Close()
		}
	})

	count, err := mgr.ReloadLuaMacros(newFakeMacroHost(), dir)
	if err != nil || count != 1 {
		t.Fatalf("first reload = (%d, %v), want (1, nil)", count, err)
	}
	if mgr.Lua.Find("Shell", "CtrlJ") == nil {
		t.Fatal("first macro was not loaded")
	}

	// LoadFile uses os.ReadFile and must not keep the script open. This is the
	// Windows-sensitive part of the user report: a generated script can be
	// renamed immediately after it has been loaded.
	renamed := filepath.Join(dir, "renamed.lua")
	if err := os.Rename(path, renamed); err != nil {
		t.Fatalf("rename loaded script: %v", err)
	}
	const second = `Macro { area = "Shell"; key = "CtrlK"; action = function() end }`
	if err := os.WriteFile(renamed, []byte(second), 0o600); err != nil {
		t.Fatal(err)
	}

	count, err = mgr.ReloadLuaMacros(newFakeMacroHost(), dir)
	if err != nil || count != 1 {
		t.Fatalf("second reload = (%d, %v), want (1, nil)", count, err)
	}
	if mgr.Lua.Find("Shell", "CtrlJ") != nil {
		t.Fatal("stale macro survived the reload")
	}
	if mgr.Lua.Find("Shell", "CtrlK") == nil {
		t.Fatal("reloaded macro was not installed")
	}
}

// A macro that never returns hits the call deadline and leaves its interpreter
// unusable; the next key rebuilds the engine from disk, so the other macros
// keep working instead of going silent until f4 restarts.
func TestRefreshInterruptedLuaRebuildsTheEngine(t *testing.T) {
	old := macroCallTimeout
	macroCallTimeout = 100 * time.Millisecond
	t.Cleanup(func() { macroCallTimeout = old })

	dir := t.TempDir()
	const source = `
Macro { area = "Shell"; key = "CtrlJ"; action = function() while true do end end }
Macro { area = "Shell"; key = "CtrlK"; action = function() end }`
	if err := os.WriteFile(filepath.Join(dir, "m.lua"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := &MacroManager{}
	t.Cleanup(func() {
		if mgr.Lua != nil {
			_ = mgr.Lua.Close()
		}
	})
	host := newFakeMacroHost()
	if count, err := mgr.ReloadLuaMacros(host, dir); err != nil || count != 2 {
		t.Fatalf("reload = (%d, %v), want (2, nil)", count, err)
	}
	if mgr.RefreshInterruptedLua() {
		t.Fatal("a healthy engine must not be rebuilt")
	}

	first := mgr.Lua
	if !first.Trigger("Shell", keymap.ParseFarKey("CtrlJ")) || !first.WaitIdle(5*time.Second) {
		t.Fatal("the runaway macro was not triggered or never stopped")
	}
	if !first.Interrupted() {
		t.Fatal("the engine does not report the interrupted interpreter")
	}

	if !mgr.RefreshInterruptedLua() {
		t.Fatal("the interrupted engine was not rebuilt")
	}
	if mgr.Lua == first || mgr.Lua.Interrupted() || mgr.Lua.Find("Shell", "CtrlK") == nil {
		t.Fatal("the rebuilt engine is not a fresh one with the macros loaded")
	}
	if !fireMacro(t, mgr.Lua, "CtrlK") {
		t.Fatal("the rebuilt engine does not run its macros")
	}
}
