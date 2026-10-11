package vtui

import (
	"bytes"
	"strings"
	"testing"
)

// unxed/f4#107: f4 paints its background with palette index 0 and loads dark
// grey into that entry with OSC 4. PuTTY and KiTTY ignore an OSC 4 that sets a
// colour, so there the background stayed black. Outside the 16-colour profile
// no cell may depend on the terminal having honoured OSC 4.
func TestIndexedColorsDoNotRelyOnTerminalPalette(t *testing.T) {
	pal := XTerm256Palette
	pal[0] = 0x2E3436
	pal[200] = 0x2E3436

	cases := []struct {
		name    string
		bg      bool
		attr    uint64
		profile ColorProfile
		want    string
	}{
		{"redefined low index, true colour", true, SetIndexBack(0, 0), ColorProfileTrueColor, "48;2;46;52;54"},
		{"redefined low index, 256 colours", true, SetIndexBack(0, 0), ColorProfile256, "48;5;236"},
		{"unchanged low index, 256 colours", false, SetIndexFore(0, 7), ColorProfile256, "38;5;250"},
		{"redefined high index, 256 colours", true, SetIndexBack(0, 200), ColorProfile256, "48;5;236"},
		{"standard high index stays an index", true, SetIndexBack(0, 196), ColorProfile256, "48;5;196"},
		{"standard high index stays an index in true colour", true, SetIndexBack(0, 16), ColorProfileTrueColor, "48;5;16"},
		{"16 colours still use the terminal palette", true, SetIndexBack(0, 0), ColorProfile16, "40"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := colorToANSI(tc.bg, tc.attr, &pal, tc.profile, make(map[uint32]uint8)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// An RGB colour equal to a redefined low entry must not be sent as that entry
// either: that is how the editor, drawn with early binding, turned black too.
func TestRGBQuantizationIgnoresRedefinedLowEntries(t *testing.T) {
	pal := XTerm256Palette
	pal[0] = 0x2E3436
	got := colorToANSI(true, SetRGBBack(0, 0x2E3436), &pal, ColorProfile256, make(map[uint32]uint8))
	if want := "48;5;236"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Cells now carry the colours their palette entries resolve to, so a terminal
// no longer recolours them when the palette changes: the renderer must repaint.
func TestPaletteChangeRepaintsIndexedCells(t *testing.T) {
	scr := NewScreenBuf()
	scr.ColorProfile = ColorProfile256
	var out bytes.Buffer
	scr.Writer = &out
	pal := XTerm256Palette
	pal[0] = 0x2E3436
	scr.ThemePalette = &pal
	scr.AllocBuf(3, 1)
	scr.Write(0, 0, []CharInfo{{Char: 'A', Attributes: SetIndexBoth(0, 7, 0)}})

	scr.Flush()
	if !strings.Contains(out.String(), "48;5;236") {
		t.Fatalf("first frame does not paint the dark grey background: %q", out.String())
	}

	out.Reset()
	scr.Flush()
	if strings.Contains(out.String(), "48;") {
		t.Errorf("unchanged frame and palette repainted cells: %q", out.String())
	}

	out.Reset()
	pal[0] = 0x000000
	scr.Flush()
	if !strings.Contains(out.String(), "48;5;16") {
		t.Errorf("palette change did not repaint the cells in the new colour: %q", out.String())
	}
}
