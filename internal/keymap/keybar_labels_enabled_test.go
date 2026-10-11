package keymap

import (
	"testing"

	"github.com/unxed/f4/internal/action"
)

// TestKeyBarLabelsForArea_HonoursEnabled checks the key-bar half of the
// action.Action.Enabled mechanism (f4#1356): a bound action's F-key label
// stays on screen either way -- Enabled only dims it -- and the disabled
// flag tracks Enabled() live, the same as Visible does for menus.
func TestKeyBarLabelsForArea_HonoursEnabled(t *testing.T) {
	restore := action.Snapshot()
	defer restore()

	enabled := true
	action.RegisterAction(action.Action{
		Name:        "Test.Enabled.F5",
		Area:        "TestKeyBarEnabledArea",
		Label:       "Test F5",
		DefaultKeys: []string{"F5"},
		Enabled:     func() bool { return enabled },
		Handler:     func() bool { return true },
	})

	oldHm := GlobalHotkeysMgr
	oldLookup := LookupAction
	t.Cleanup(func() {
		GlobalHotkeysMgr = oldHm
		LookupAction = oldLookup
	})
	LookupAction = action.Lookup
	GlobalHotkeysMgr = NewHotkeyManager("")

	set := KeyBarLabelsForArea("TestKeyBarEnabledArea", nil)
	if set.Normal[4] != "Test F5" {
		t.Fatalf("expected F5 label %q, got %q", "Test F5", set.Normal[4])
	}
	if set.NormalDisabled[4] {
		t.Error("an enabled action must not be marked disabled on the keybar")
	}

	enabled = false
	set = KeyBarLabelsForArea("TestKeyBarEnabledArea", nil)
	if !set.NormalDisabled[4] {
		t.Error("a disabled action must be marked disabled on the keybar")
	}
	if set.Normal[4] != "Test F5" {
		t.Error("a disabled action's label must stay on screen, only dimmed -- unlike Visible, Enabled does not hide it")
	}

	// An action without Enabled at all (nil) is never reported disabled.
	action.RegisterAction(action.Action{
		Name:        "Test.Enabled.F6.Unset",
		Area:        "TestKeyBarEnabledArea",
		Label:       "Test F6",
		DefaultKeys: []string{"F6"},
		Handler:     func() bool { return true },
	})
	GlobalHotkeysMgr = NewHotkeyManager("")
	set = KeyBarLabelsForArea("TestKeyBarEnabledArea", nil)
	if set.NormalDisabled[5] {
		t.Error("an action with no Enabled predicate must never be marked disabled")
	}
}

// TestKeyBarLabelsForArea_HonoursKeyBarLabel covers the dynamic caption (f4#1794):
// a bound action whose KeyBarLabel answers puts that answer on the bar, and
// an empty answer leaves the static label.
func TestKeyBarLabelsForArea_HonoursKeyBarLabel(t *testing.T) {
	restore := action.Snapshot()
	defer restore()

	dynamic := ""
	action.RegisterAction(action.Action{
		Name:        "Test.KeyBarLabel.F4",
		Area:        "TestKeyBarLabelArea",
		Label:       "Edit",
		DefaultKeys: []string{"F4"},
		KeyBarLabel: func() string { return dynamic },
		Handler:     func() bool { return true },
	})

	oldHm := GlobalHotkeysMgr
	oldLookup := LookupAction
	t.Cleanup(func() {
		GlobalHotkeysMgr = oldHm
		LookupAction = oldLookup
	})
	LookupAction = action.Lookup
	GlobalHotkeysMgr = NewHotkeyManager("")

	if got := KeyBarLabelsForArea("TestKeyBarLabelArea", nil).Normal[3]; got != "Edit" {
		t.Fatalf("with no dynamic answer the F4 label = %q, want the static %q", got, "Edit")
	}
	dynamic = "Attr"
	if got := KeyBarLabelsForArea("TestKeyBarLabelArea", nil).Normal[3]; got != "Attr" {
		t.Fatalf("with a dynamic answer the F4 label = %q, want %q", got, "Attr")
	}
}
