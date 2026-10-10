package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestDeclaredHotkeyStringUsesTheHotkeyManagersSpelling(t *testing.T) {
	for declared, want := range map[string]string{
		"Shift+F1": "ShiftF1",
		"F4":       "F4",
		"Ctrl+F9":  "CtrlF9",
		"":         "",
	} {
		if got := declaredHotkeyString(declared); got != want {
			t.Errorf("declaredHotkeyString(%q) = %q, want %q", declared, got, want)
		}
	}
}

// A default hotkey a plugin brings with it can be removed with Del in the plugin
// menu (unxed/f4#918): the menu stops showing it, and the list is what dispatch
// reads to stop running it.
func TestPluginDefaultHotkeyCanBeSwitchedOff(t *testing.T) {
	old := config.App.PluginDefaultHotkeysOff
	t.Cleanup(func() { config.App.PluginDefaultHotkeysOff = old })
	config.App.PluginDefaultHotkeysOff = ""

	if PluginDefaultKeyOff("ShiftF1") {
		t.Fatal("nothing is switched off by default")
	}
	entry := PluginMenuEntry{Label: "Add to archive", ActionName: "Plugin.Command.x", Declared: "Shift+F1"}
	entry.applyBinding()
	if entry.Chord != "Shift+F1" {
		t.Fatalf("chord before = %q, want the declared Shift+F1", entry.Chord)
	}

	config.App.PluginDefaultHotkeysOff = "ShiftF1;ShiftF2"
	if !PluginDefaultKeyOff("shiftf1") || !PluginDefaultKeyOff("ShiftF2") || PluginDefaultKeyOff("ShiftF3") {
		t.Fatal("the switched-off list is not read correctly")
	}
	entry.applyBinding()
	if entry.Chord != "" || entry.Hotkey != "" {
		t.Fatalf("after removal the entry shows chord %q hotkey %q, want none", entry.Chord, entry.Hotkey)
	}
}
