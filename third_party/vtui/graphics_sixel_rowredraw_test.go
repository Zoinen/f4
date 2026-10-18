package vtui

import (
	"regexp"
	"strings"
	"testing"
)

var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")

// f4#1885 (sogonov, Windows Terminal): a row with a character outside ASCII
// and the box drawing set is written again in full. Under an open menu with a
// check mark next to a picture that wrote the cells of the picture too, the
// text background covered it row by row, and the whole picture was sent
// again. The cells a picture shows in stay unwritten while they are unchanged.
func TestFullRowRedrawLeavesTheCellsOfAPicture(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	scr.Graphics().SetProtocol(GraphicsSixel)
	surf := NewImageSurface(4, 4)
	fillOpaque(surf, 30, 120, 200)
	pass := func(mark string) {
		g := scr.Graphics()
		g.BeginFrame()
		scr.FillRect(0, 0, 19, 9, '.', 0)
		g.DrawImage("panel", ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 6, Rows: 4})
		scr.Write(10, 2, StringToCharInfo(mark+" Medium", 0)) // a menu row beside the picture
		g.EndFrame()
	}

	pass("√")
	scr.Flush()
	out.Reset()

	pass(" ") // the check mark moves away: the row has changed, the picture has not
	scr.Flush()
	frame := out.String()
	if strings.Contains(frame, "\x1bP") {
		t.Errorf("the picture was sent again for a change beside it: %q", frame)
	}
	// Columns 2..7 of the third row are the picture's; the row is written
	// left of it and goes on right of it.
	start, resume := strings.Index(frame, "\x1b[3;1H"), strings.Index(frame, "\x1b[3;8H")
	if start < 0 || resume < start {
		t.Fatalf("the row is not written around the picture: %q", frame)
	}
	if left := sgrPattern.ReplaceAllString(frame[start+len("\x1b[3;1H"):resume], ""); left != "." {
		t.Errorf("the row was written over the picture: %q", left)
	}
}
