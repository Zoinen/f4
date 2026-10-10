package vtui

import "testing"

func TestKeyBar_CombinedModifierRows(t *testing.T) {
	kb := NewKeyBar()
	kb.Normal[0] = "n"
	kb.Shift[0] = "s"
	kb.Ctrl[0] = "c"
	kb.Alt[0] = "a"

	// Combined rows not supplied: the single-modifier priority stays.
	kb.LatchModifiers(true, false, true)
	if l, _, name := kb.activeRow(); l[0] != "s" || name != "shift" {
		t.Fatalf("shift+alt without an AltShift row: got %q %q, want the Shift row", l[0], name)
	}

	kb.CtrlShift[0] = "cs"
	kb.AltShift[0] = "as"
	kb.CtrlAlt[0] = "ca"
	kb.AltShiftDisabled[0] = true

	cases := []struct {
		shift, ctrl, alt bool
		label, name      string
	}{
		{false, false, false, "n", "normal"},
		{true, false, false, "s", "shift"},
		{false, true, false, "c", "ctrl"},
		{false, false, true, "a", "alt"},
		{true, true, false, "cs", "ctrl+shift"},
		{true, false, true, "as", "alt+shift"},
		{false, true, true, "ca", "ctrl+alt"},
	}
	for _, c := range cases {
		kb.LatchModifiers(c.shift, c.ctrl, c.alt)
		l, _, name := kb.activeRow()
		if l[0] != c.label || name != c.name {
			t.Errorf("shift=%v ctrl=%v alt=%v: got %q %q, want %q %q", c.shift, c.ctrl, c.alt, l[0], name, c.label, c.name)
		}
	}
	kb.LatchModifiers(true, false, true)
	if _, d, _ := kb.activeRow(); !d[0] {
		t.Error("the disabled flags of the AltShift row were not used")
	}
}
