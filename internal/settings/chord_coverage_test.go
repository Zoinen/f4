package settings

import "testing"

func TestSettingsChordConstructionAndLayout(t *testing.T) {
	changed := ""
	chord := newSettingsChord("Ctrl+K", func(value string) { changed = value })
	if chord.edit.GetText() != "Ctrl+K" || chord.capture.GetCaption() == "" {
		t.Fatalf("chord initial state text=%q caption=%q", chord.edit.GetText(), chord.capture.GetCaption())
	}
	chord.SetPosition(2, 3, 30, 3)
	x1, y1, x2, y2 := chord.GetPosition()
	if x1 != 2 || y1 != 3 || x2 != 30 || y2 != 3 {
		t.Fatalf("chord position=%d,%d,%d,%d", x1, y1, x2, y2)
	}
	chord.edit.SetText("Alt+K")
	chord.edit.OnTextChange(chord.edit.GetText())
	if changed != "Alt+K" {
		t.Fatalf("text-change callback received %q", changed)
	}
}
