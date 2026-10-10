package frameborder

import (
	"testing"

	"github.com/unxed/vtui"
)

// borderAttr stands for the colour a frame drew its border with; an
// arbitrary value so the test notices a zero cell as well as a wrong one.
const borderAttr uint64 = 0x00C0C00000000300

// screenAttr stands for whatever sits behind the frame.
const screenAttr uint64 = 0x00A0A0000000A000

// paintFrame fills the part of a frame the buffer can show, the way a frame
// paints itself clipped to the screen: everything outside is never drawn.
func paintFrame(scr *vtui.ScreenBuf, x1, y1, x2, y2 int) {
	w, h := scr.Width(), scr.Height()
	if x2 < 0 || y2 < 0 || x1 >= w || y1 >= h {
		return // nothing of the frame is on screen
	}
	scr.FillRect(max(x1, 0), max(y1, 0), min(x2, w-1), min(y2, h-1), '#', borderAttr)
}

func newScreen(t *testing.T) *vtui.ScreenBuf {
	t.Helper()
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(40, 20)
	scr.FillRect(0, 0, 39, 19, '.', screenAttr)
	return scr
}

// A frame whose border the buffer can see must answer with the colour that
// border was drawn in, wherever the frame sits: dragged left, up, into a
// corner, or past the right and bottom edges.
func TestAttrAnswersWithTheBorderColour(t *testing.T) {
	for _, tc := range []struct {
		name   string
		x1     int
		y1     int
		x2     int
		y2     int
		onCell [2]int // a painted cell of this frame, for a second opinion
	}{
		{name: "inside", x1: 5, y1: 5, x2: 20, y2: 12, onCell: [2]int{5, 5}},
		{name: "dragged left", x1: -8, y1: 5, x2: 20, y2: 12, onCell: [2]int{0, 5}},
		{name: "dragged up", x1: 5, y1: -6, x2: 20, y2: 12, onCell: [2]int{5, 0}},
		{name: "dragged to the corner", x1: -8, y1: -6, x2: 20, y2: 12, onCell: [2]int{0, 0}},
		{name: "dragged right", x1: 25, y1: 5, x2: 60, y2: 12, onCell: [2]int{39, 5}},
		{name: "dragged down", x1: 5, y1: 15, x2: 20, y2: 40, onCell: [2]int{5, 19}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scr := newScreen(t)
			paintFrame(scr, tc.x1, tc.y1, tc.x2, tc.y2)

			got, ok := Attr(scr, tc.x1, tc.y1, tc.x2, tc.y2)
			if !ok {
				t.Fatal("Attr found no border colour for a frame whose border is on screen")
			}
			if got != borderAttr {
				t.Fatalf("border attr = %#x, want %#x", got, borderAttr)
			}
			if want := scr.GetCell(tc.onCell[0], tc.onCell[1]).Attributes; want != got {
				t.Fatalf("Attr = %#x, but cell (%d,%d) of the same frame reads %#x",
					got, tc.onCell[0], tc.onCell[1], want)
			}
		})
	}
}

// A frame with no border cell left on screen has no border colour of its own
// to read, and the probe must say so instead of answering with the colour
// behind the frame or with a zero cell.
func TestAttrSaysNoWhenTheBorderIsOffScreen(t *testing.T) {
	for _, tc := range []struct {
		name string
		x1   int
		y1   int
		x2   int
		y2   int
	}{
		{name: "left of the buffer", x1: -30, y1: 5, x2: -5, y2: 12},
		{name: "above the buffer", x1: 5, y1: -30, x2: 20, y2: -5},
		{name: "right of the buffer", x1: 45, y1: 5, x2: 60, y2: 12},
		{name: "below the buffer", x1: 5, y1: 25, x2: 20, y2: 40},
		{name: "bigger than the buffer", x1: -3, y1: -3, x2: 60, y2: 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scr := newScreen(t)
			paintFrame(scr, tc.x1, tc.y1, tc.x2, tc.y2)

			got, ok := Attr(scr, tc.x1, tc.y1, tc.x2, tc.y2)
			if ok {
				t.Fatalf("Attr = %#x for a frame with no border on screen, want no answer", got)
			}
			if got != 0 {
				t.Fatalf("Attr = %#x beside its false answer; %#x is the colour behind the frame, %#x the zero cell",
					got, screenAttr, uint64(0))
			}
		})
	}
}
