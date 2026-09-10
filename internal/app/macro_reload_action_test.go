package app

import (
	"github.com/unxed/f4/internal/keymap"
	"testing"
)

// The action registration stayed with the registry and the hotkey manager; the
// reload itself is tested in internal/macro.

func TestMacroReloadActionRegistration(t *testing.T) {
	action, ok := GetAction("Macro.Reload")
	if !ok {
		t.Fatal("Macro.Reload is not registered")
	}
	if action.Area != "Common" || len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "CtrlAltShiftM" {
		t.Fatalf("action metadata = %+v", action)
	}
	if got := keymap.NewHotkeyManager("").GetAction("Shell", "CtrlAltShiftM"); got != "Macro.Reload" {
		t.Fatalf("default hotkey = %q, want Macro.Reload", got)
	}
}
