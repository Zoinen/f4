package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMacroReloadActionRegistration(t *testing.T) {
	action, ok := GetAction("Macro.Reload")
	if !ok {
		t.Fatal("Macro.Reload is not registered")
	}
	if action.Area != "Common" || len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "CtrlAltShiftM" {
		t.Fatalf("action metadata = %+v", action)
	}
	if got := NewHotkeyManager("").GetAction("Shell", "CtrlAltShiftM"); got != "Macro.Reload" {
		t.Fatalf("default hotkey = %q, want Macro.Reload", got)
	}
}

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

	count, err := mgr.ReloadLuaMacros(dir)
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

	count, err = mgr.ReloadLuaMacros(dir)
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
