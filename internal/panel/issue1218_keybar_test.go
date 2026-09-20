package panel

import (
	"testing"

	"github.com/unxed/f4/internal/action"
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
