package keymap

import (
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/vtui"
)

func TestKeyBarMergePreservesIconsDynamicLabelsAndDisabledRows(t *testing.T) {
	restore := action.Snapshot()
	t.Cleanup(restore)
	previousManager, previousLookup := GlobalHotkeysMgr, LookupAction
	t.Cleanup(func() {
		GlobalHotkeysMgr, LookupAction = previousManager, previousLookup
	})
	enabled, label := false, "&Copy now"
	action.RegisterAction(action.Action{
		Name:        "Test.Merge.Copy",
		Area:        "TestMerge",
		Label:       "Copy",
		DefaultKeys: []string{"F5", "ShiftF5", "AltF5", "CtrlF5", "CtrlShiftF5", "AltShiftF5", "CtrlAltF5"},
		KeyBarLabel: func() string { return label },
		Enabled:     func() bool { return enabled },
	})
	LookupAction = action.Lookup
	GlobalHotkeysMgr = NewHotkeyManager("")
	fallbacks := &vtui.KeySet{}
	fallbacks.Normal[0], fallbacks.NormalIcons[0] = "Help fallback", "circle-question-mark"
	fallbacks.CtrlShift[0], fallbacks.CtrlShiftIcons[0] = "Ignored", "ignored"
	set := KeyBarLabelsForArea("TestMerge", fallbacks)
	rows := []struct {
		labels   vtui.KeyBarLabels
		icons    vtui.KeyBarIconNames
		disabled bool
	}{
		{labels: set.Normal, icons: set.NormalIcons, disabled: set.NormalDisabled[4]},
		{labels: set.Shift, icons: set.ShiftIcons, disabled: set.ShiftDisabled[4]},
		{labels: set.Alt, icons: set.AltIcons, disabled: set.AltDisabled[4]},
		{labels: set.Ctrl, icons: set.CtrlIcons, disabled: set.CtrlDisabled[4]},
		{labels: set.CtrlShift, icons: set.CtrlShiftIcons, disabled: set.CtrlShiftDisabled[4]},
		{labels: set.AltShift, icons: set.AltShiftIcons, disabled: set.AltShiftDisabled[4]},
		{labels: set.CtrlAlt, icons: set.CtrlAltIcons, disabled: set.CtrlAltDisabled[4]},
	}
	for i, row := range rows {
		if row.labels[4] != "Copy now" || row.icons[4] != "copy" || !row.disabled {
			t.Errorf("row %d lost label/icon/disabled metadata: %+v", i, row)
		}
	}
	if set.Normal[0] != "Help fallback" || set.NormalIcons[0] != "circle-question-mark" ||
		set.NormalDisabled[0] || set.CtrlShift[0] != "" || set.CtrlShiftIcons[0] != "" {
		t.Fatalf("fallback behavior changed: %+v", set)
	}
	enabled, label = true, ""
	set = KeyBarLabelsForArea("TestMerge", nil)
	if set.Normal[4] != "Copy" || set.NormalIcons[4] != "copy" || set.NormalDisabled[4] {
		t.Fatalf("live label/enabled refresh lost icons: %+v", set)
	}
	set = KeyBarLabelsForAreaExcept("TestMerge", fallbacks, func(name string) bool {
		return name == "Test.Merge.Copy"
	})
	if set.Normal[4] != "" || set.NormalIcons[4] != "" || set.NormalDisabled[4] ||
		set.CtrlAlt[4] != "" || set.CtrlAltIcons[4] != "" || set.CtrlAltDisabled[4] {
		t.Fatalf("filtered binding retained metadata: %+v", set)
	}
	GlobalHotkeysMgr.Bind("TestMerge", "F5", "None")
	set = KeyBarLabelsForArea("TestMerge", fallbacks)
	if set.Normal[4] != "" || set.NormalIcons[4] != "" || set.NormalDisabled[4] {
		t.Fatalf("explicit unbind retained metadata: %+v", set)
	}
}
