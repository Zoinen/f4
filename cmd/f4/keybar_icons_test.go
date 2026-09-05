package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestKeyBarIconForActionUsesStableActionIdentity(t *testing.T) {
	tests := map[string]string{
		"Panel.LeftDriveMenu": "hard-drive",
		"File.View":           "eye",
		"File.Copy":           "copy",
		"File.Move":           "folder-input",
		"File.MakeDir":        "folder-plus",
		"Editor.WordWrap":     "text-wrap",
		"Viewer.HexMode":      "binary",
		"Viewer.GoTo":         "locate-fixed",
		"Unknown.SearchThing": "search",
	}
	for action, want := range tests {
		if got := keyBarIconForAction(action); got != want {
			t.Errorf("keyBarIconForAction(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestKeyBarLabelsForAreaTracksRemappedActionIcon(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = &HotkeyManager{Bindings: map[string]map[string]string{
		"Shell": {"F5": "File.Delete"},
	}}
	defer func() { GlobalHotkeysMgr = old }()

	fallbacks := &vtui.KeySet{
		Normal:      vtui.KeyBarLabels{"", "", "", "", "Copy"},
		NormalIcons: vtui.KeyBarIconNames{"", "", "", "", "copy"},
	}
	set := KeyBarLabelsForArea("Shell", fallbacks)
	if set.Normal[4] != plainLabel(mustAction(t, "File.Delete").DisplayLabel()) {
		t.Fatalf("remapped F5 label = %q", set.Normal[4])
	}
	if set.NormalIcons[4] != "trash-2" {
		t.Fatalf("remapped F5 icon = %q, want trash-2", set.NormalIcons[4])
	}
}

func TestPanelsKeyBarExposesNativePresentationToggle(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = nil
	defer func() { GlobalHotkeysMgr = old }()

	set := (&PanelsFrame{}).GetKeyLabels()
	if set.Shift[11] != Msg("KeyBar.ShiftF12") || set.ShiftIcons[11] != "monitor" {
		t.Fatalf("Shift+F12 key-bar hint = %q/%q", set.Shift[11], set.ShiftIcons[11])
	}
}

func mustAction(t *testing.T, name string) Action {
	t.Helper()
	action, ok := GetAction(name)
	if !ok {
		t.Fatalf("missing registered action %q", name)
	}
	return action
}
