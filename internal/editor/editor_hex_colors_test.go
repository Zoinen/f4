package editor

import (
	"testing"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// The offsets down the left of the hex view are drawn in the viewer's offset
// colour on the background the bytes are on, and the column separator in the
// bytes' own colour. In the status line's colour they showed as a strip next
// to the bytes (#1232).
func TestEditor_HexOffsetsSitOnTheTextBackground(t *testing.T) {
	ev, scr := occurrenceEditor(t, "0123456789abcdefXYZ")
	ev.HexMode = true
	ev.Show(scr)

	text := ev.colorerBaseAttr()
	_, textBg := theme.GetColorRGBBoth(text)
	arrowsFg, _ := theme.GetColorRGBBoth(vtui.Palette[theme.ColViewerArrows])
	for x := 0; x < 11; x++ { // "0000000000:"
		fg, bg := theme.GetColorRGBBoth(scr.GetCell(x, 1).Attributes)
		if fg != arrowsFg || bg != textBg {
			t.Errorf("offset cell %d is #%06x on #%06x, want #%06x on the text's #%06x", x, fg, bg, arrowsFg, textBg)
		}
	}
	sep := scr.GetCell(60, 1)
	if sep.Char != uint64('│') || sep.Attributes != text {
		t.Errorf("separator %q attr %#x, want '│' in the text colour %#x", rune(sep.Char), sep.Attributes, text)
	}
}
