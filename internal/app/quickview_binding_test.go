package app

import (
	keymap "github.com/unxed/f4/internal/keymap"
	testing "testing"
)

func TestQuickViewSemanticQMLContract_CtrlQBindingIsShellOnly(t *testing.T) {
	hotkeys := keymap.NewHotkeyManager("")
	if action := hotkeys.GetAction("Shell", "CtrlQ"); action != "Panel.QuickView" {
		t.Fatalf("Shell/CtrlQ = %q, want Panel.QuickView", action)
	}
	for _, area := range []string{"Viewer", "Editor", "Terminal"} {
		if action := hotkeys.GetAction(area, "CtrlQ"); action != "" {
			t.Fatalf("%s/CtrlQ leaked to %q; F3/F4/terminal must retain key ownership", area, action)
		}
	}
}
