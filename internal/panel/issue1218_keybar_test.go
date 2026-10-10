package panel

import (
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
)

// Help is deliberately allowed to inherit the panel key bar.  Its presence
// must not change the captions for actions that still belong to the panel
// frame: before #1218 the global CurrentArea() returned "Other" here, so F2
// and F10 silently changed from action labels to generic fallback strings.
func TestPanelsFrameKeyBarKeepsItsOwnActionLabelsBehindAnotherFrame(t *testing.T) {
	previousArea := CurrentArea
	previousHotkeys := keymap.GlobalHotkeysMgr
	previousLocalize := action.Localize
	t.Cleanup(func() {
		CurrentArea = previousArea
		keymap.GlobalHotkeysMgr = previousHotkeys
		action.Localize = previousLocalize
	})

	restoreActions := action.Snapshot()
	t.Cleanup(restoreActions)
	action.RegisterAction(action.Action{Name: "Panel.UserMenu", Label: "User menu", DefaultKeys: []string{"F2"}})
	action.RegisterAction(action.Action{Name: "App.Quit", Label: "Quit", DefaultKeys: []string{"F10"}})
	action.Localize = func(key string) string { return "{" + key + "}" }
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{Bindings: map[string]map[string]string{
		"Shell":    {"F2": "Panel.UserMenu", "F10": "App.Quit"},
		"Terminal": {"F2": "Panel.UserMenu", "F10": "App.Quit"},
	}}
	// This is what the application reports while Help is on top of the
	// panels.  GetKeyLabels must not use it to choose the panel's bindings.
	CurrentArea = func() string { return "Other" }

	for _, showPanels := range []bool{true, false} {
		pf := &PanelsFrame{ShowPanels: showPanels}
		labels := pf.GetKeyLabels()
		if got := labels.Normal[1]; got != "User menu" {
			t.Errorf("ShowPanels=%v: F2 label = %q, want %q", showPanels, got, "User menu")
		}
		if got := labels.Normal[9]; got != "Quit" {
			t.Errorf("ShowPanels=%v: F10 label = %q, want %q", showPanels, got, "Quit")
		}
	}
}

// TestPanelsFrameTerminalKeyBarFallbacksAreLocalized guards the f4#1218
// follow-up found while re-checking the ticket: the Shift+F6/Shift+F9/
// Ctrl+F12 slots in GetKeyLabels' fallback vtui.KeySet were plain English
// string literals ("Rename", "Save", "Close"), never routed through
// i18n.Msg. File.Rename and Panel.SortMenu only bind Ctrl+F12/Shift+F6 in
// the "Shell" area (no DefaultAreas entry adds "Terminal"), so
// keymap.KeyBarLabelsForArea falls through to these literals whenever the
// panels are hidden (Ctrl+O) -- showing English regardless of the active
// language, in the one GetKeyLabels() caption path the LabelKey-based
// scanners (TestActionLabelKeysResolve, TestCtrlRowActionsHaveLabelKey)
// cannot see, because no Action.LabelKey is involved at all.
func TestPanelsFrameTerminalKeyBarFallbacksAreLocalized(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		i18n.InitLang("", "", "")
	})

	// No bindings anywhere: every resolve() call in KeyBarLabelsForArea must
	// fall back to the literal fallbacks vtui.KeySet carries.
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{Bindings: map[string]map[string]string{}}
	i18n.InitLang("ru", "", "")

	pf := &PanelsFrame{ShowPanels: false} // Terminal area
	labels := pf.GetKeyLabels()

	cases := []struct {
		name    string
		got     string
		english string
	}{
		{"Shift+F6", labels.Shift[5], "Rename"},
		{"Shift+F9", labels.Shift[8], "Save"},
		{"Ctrl+F12", labels.Ctrl[11], "Close"},
	}
	for _, tc := range cases {
		if tc.got == tc.english {
			t.Errorf("%s fallback label = %q, still the untranslated English literal regardless of locale", tc.name, tc.got)
		}
		if tc.got == "" {
			t.Errorf("%s fallback label is empty, want a localized caption", tc.name)
		}
	}
}
