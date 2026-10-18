package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func drawControlForHoverTest(t *testing.T, control interface{ Show(*ScreenBuf) }) *ScreenBuf {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(32, 2)
	control.Show(scr)
	return scr
}

func TestRoundedControls_HoverTintPreservesContrast(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	for _, tc := range []struct {
		name   string
		widget interface{ Show(*ScreenBuf) }
	}{
		{"checkbox", NewCheckbox(0, 0, "Check", false)},
		{"radio", NewRadioButton(0, 0, "Radio", false)},
		{"check group", NewCheckGroup(0, 0, 1, []string{"One", "Two"})},
		{"radio group", NewRadioGroup(0, 0, 1, []string{"One", "Two"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := drawControlForHoverTest(t, tc.widget)
			// Use the concrete public hover setter and redraw over the same geometry.
			switch w := tc.widget.(type) {
			case *Checkbox:
				w.SetMouseHovered(true)
			case *RadioButton:
				w.SetMouseHovered(true)
			case *CheckGroup:
				w.SetMouseHovered(true)
				w.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 0, MouseY: 0, MouseEventFlags: vtinput.MouseMoved})
			case *RadioGroup:
				w.SetMouseHovered(true)
				w.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 0, MouseY: 0, MouseEventFlags: vtinput.MouseMoved})
			}
			hovered := NewSilentScreenBuf()
			hovered.AllocBuf(32, 2)
			tc.widget.Show(hovered)
			before, after := base.GetCell(0, 0).Attributes, hovered.GetCell(0, 0).Attributes
			if attrColorRGB(before, true) == attrColorRGB(after, true) {
				t.Fatal("hover did not tint the control indicator")
			}
			if got := contrastRatioRGB(attrColorRGB(after, false), attrColorRGB(after, true)); got < roundedButtonMinContrast {
				t.Errorf("hover contrast = %.2f, want >= %.1f", got, roundedButtonMinContrast)
			}
		})
	}
}

func TestControlGroups_HoverTracksItemAndLeave(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	for _, name := range []string{"check group", "radio group"} {
		t.Run(name, func(t *testing.T) {
			var move func(*vtinput.InputEvent) bool
			var set func(bool)
			var idx func() int
			if name == "check group" {
				g := NewCheckGroup(0, 0, 1, []string{"One", "Two"})
				move, set, idx = g.ProcessMouse, g.SetMouseHovered, func() int { return g.hoverIdx }
			} else {
				g := NewRadioGroup(0, 0, 1, []string{"One", "Two"})
				move, set, idx = g.ProcessMouse, g.SetMouseHovered, func() int { return g.hoverIdx }
			}
			set(true)
			event := func(x, y int16) *vtinput.InputEvent {
				return &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: x, MouseY: y, MouseEventFlags: vtinput.MouseMoved}
			}
			if !move(event(0, 0)) || idx() != 0 {
				t.Fatalf("first option hover index = %d, want 0", idx())
			}
			if !move(event(0, 1)) || idx() != 1 {
				t.Fatalf("moving to second option hover index = %d, want 1", idx())
			}
			if !move(event(30, 1)) || idx() != -1 {
				t.Fatalf("leaving group hover index = %d, want -1", idx())
			}
		})
	}
}

func TestRoundedControlHover_ClassicAndTextTerminalUnchanged(t *testing.T) {
	for _, env := range []struct {
		name, backend string
	}{{"classic graphical", "gogpu"}, {"text terminal", ""}} {
		t.Run(env.name, func(t *testing.T) {
			roundedButtonEnv(t, env.backend, GlyphStyleClassic)
			cb := NewCheckbox(0, 0, "Check", false)
			scr := NewSilentScreenBuf()
			scr.AllocBuf(20, 1)
			cb.Show(scr)
			before := scr.GetCell(0, 0).Attributes
			cb.SetMouseHovered(true)
			afterScreen := NewSilentScreenBuf()
			afterScreen.AllocBuf(20, 1)
			cb.Show(afterScreen)
			after := afterScreen.GetCell(0, 0).Attributes
			if before != after {
				t.Errorf("classic/text hover changed attribute: %016X -> %016X", before, after)
			}
		})
	}
}
