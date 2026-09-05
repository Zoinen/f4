package vtui

import (
	"bytes"
	"strings"
	"testing"
)

// The workspace tab bar owns cell row 0. A sixel placement whose Row is 1
// (the first content row) must emit its cursor move for 1-based row 2, never
// row 1, otherwise Windows Terminal paints the picture over the tabs.
func TestSixelCursorStaysBelowTabRow(t *testing.T) {
	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(80, 25)
	scr.Graphics().SetProtocol(GraphicsSixel)
	scr.Graphics().SetCellSize(8, 16)

	surf := NewImageSurface(10, 10)
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			surf.SetPixel(x, y, 255, 0, 0, 255)
		}
	}

	scr.Graphics().BeginFrame()
	scr.Graphics().DrawImage("img", ImagePlacement{
		Surface: surf,
		Col:     17, Row: 1, Cols: 46, Rows: 23,
	})
	scr.Graphics().EndFrame()

	scr.Flush()

	raw := out.String()
	idx := strings.Index(raw, "\x1bP0;1;8q")
	if idx < 0 {
		t.Fatalf("no sixel DCS emitted:\n%q", raw)
	}
	// The cursor move is written immediately before the DCS.
	before := raw[:idx]
	if !strings.HasSuffix(before, "\x1b[2;18H") {
		t.Fatalf("sixel cursor move not at row 2 col 18; tail=%q", before[max(0, len(before)-40):])
	}
}
