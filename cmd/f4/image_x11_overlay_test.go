package main

import (
	"os"
	"testing"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

func TestOverlayCellRect(t *testing.T) {
	// A window at 100,200 that is 800x600 and holds 80x25 cells: a cell is
	// ten by twenty four.
	term := ttyx.Rect{X: 100, Y: 200, W: 800, H: 600}

	got, ok := overlayCellRect(term, 80, 25, 0, 0, 79, 24)
	if !ok || got != (ttyx.Rect{X: 100, Y: 200, W: 800, H: 600}) {
		t.Errorf("whole grid: got %+v ok=%v", got, ok)
	}

	got, ok = overlayCellRect(term, 80, 25, 4, 2, 5, 3)
	if !ok || got != (ttyx.Rect{X: 140, Y: 248, W: 20, H: 48}) {
		t.Errorf("two by two cells at 4,2: got %+v ok=%v", got, ok)
	}
}

// A frame that reaches past the grid is clamped rather than sent off the side
// of the terminal window.
func TestOverlayCellRectClamps(t *testing.T) {
	term := ttyx.Rect{X: 0, Y: 0, W: 800, H: 600}
	got, ok := overlayCellRect(term, 80, 25, -3, -1, 200, 900)
	if !ok || got != (ttyx.Rect{X: 0, Y: 0, W: 800, H: 600}) {
		t.Errorf("clamped: got %+v ok=%v", got, ok)
	}
}

func TestOverlayCellRectRefusesNonsense(t *testing.T) {
	term := ttyx.Rect{X: 0, Y: 0, W: 800, H: 600}
	cases := []struct {
		name           string
		term           ttyx.Rect
		cols, rows     int
		x1, y1, x2, y2 int
	}{
		{"no cells", term, 0, 25, 0, 0, 1, 1},
		{"no window", ttyx.Rect{}, 80, 25, 0, 0, 1, 1},
		{"inverted", term, 80, 25, 10, 10, 4, 4},
		// A grid finer than the window has cells smaller than a pixel.
		{"cell under a pixel", ttyx.Rect{W: 40, H: 10}, 80, 25, 0, 0, 1, 1},
	}
	for _, c := range cases {
		if _, ok := overlayCellRect(c.term, c.cols, c.rows, c.x1, c.y1, c.x2, c.y2); ok {
			t.Errorf("%s: should have been refused", c.name)
		}
	}
}

// The frame key has to move when anything that changes the picture on screen
// moves, and to stay put when nothing does.
func TestOverlayFrameKey(t *testing.T) {
	surf := vtui.NewImageSurface(4, 4)
	surf.SetPixel(0, 0, 1, 2, 3, 255)
	other := vtui.NewImageSurface(4, 4)
	other.SetPixel(0, 0, 9, 9, 9, 255)

	base := []vtui.ImagePlacement{{Surface: surf, Col: 1, Row: 2, Cols: 3, Rows: 4}}
	rect := ttyx.Rect{X: 1, Y: 2, W: 30, H: 40}
	key := overlayFrameKey(base, rect)
	if key != overlayFrameKey(base, rect) {
		t.Fatal("the same frame in the same place must key the same")
	}

	moved := []vtui.ImagePlacement{{Surface: surf, Col: 5, Row: 2, Cols: 3, Rows: 4}}
	swapped := []vtui.ImagePlacement{{Surface: other, Col: 1, Row: 2, Cols: 3, Rows: 4}}
	grew := append(append([]vtui.ImagePlacement(nil), base...), base[0])

	for name, variant := range map[string][]vtui.ImagePlacement{
		"moved": moved, "different picture": swapped, "one more picture": grew,
	} {
		if overlayFrameKey(variant, rect) == key {
			t.Errorf("%s must key differently", name)
		}
	}
	if overlayFrameKey(base, ttyx.Rect{X: 9, Y: 2, W: 30, H: 40}) == key {
		t.Error("a window somewhere else must key differently")
	}
}

// A grid of thumbnails goes into one window with gaps cut out of it, so every
// picture has to land at its own offset inside the frame buffer.
func TestBlitIntoPlacesAndClips(t *testing.T) {
	dst := make([]byte, 4*4*4)
	// Two opaque pixels whose channels are their own labels. The alpha has
	// to be a real 255 rather than another label: blitInto composes now, so
	// a pixel with alpha 4 lands as a twentieth of itself over the empty
	// buffer, which is nothing, and this test would then be measuring the
	// blend instead of the offsets.
	src := []byte{1, 2, 3, 255, 5, 6, 7, 255}

	blitInto(dst, 4, 4, src, 2, 1, 8, 1, 1)
	if dst[(1*4+1)*4] != 1 || dst[(1*4+2)*4] != 5 {
		t.Errorf("the pixels did not land at the offset: %v", dst[16:32])
	}

	// Anything past the edge is dropped rather than wrapping onto the next
	// row or running off the end of the buffer.
	blitInto(dst, 4, 4, src, 2, 1, 8, 3, 3)
	if dst[(3*4+3)*4] != 1 {
		t.Error("the pixel inside the buffer must still land")
	}
}

// Every method has to survive being called on a nil overlay, because that is
// what "no X here" looks like to the viewer.
func TestOverlayNilIsSafe(t *testing.T) {
	var x *x11ImageOverlay
	x.hide()
	x.close()
	if err := x.show(80, 25, vtui.ImagePlacement{}); err == nil {
		t.Error("a nil overlay shows nothing")
	}
}

// The window is not the grid. A terminal with a menu bar on top and a scroll
// bar on the right hands back a text area smaller than its window, and the
// grid sits against the bottom left of it — which is what stops a picture
// landing a row and a bit too high.
func TestHostGridRect(t *testing.T) {
	win := ttyx.Rect{X: 100, Y: 200, W: 800, H: 630}

	// Thirty pixels of menu bar at the top, ten of scroll bar on the right.
	got := hostGridRect(win, 790, 600, true)
	want := ttyx.Rect{X: 100, Y: 230, W: 790, H: 600}
	if got != want {
		t.Errorf("measured: got %+v, want %+v", got, want)
	}

	// Nothing measured: the grid is the whole window, which is what this
	// did before it could measure anything.
	if got := hostGridRect(win, 0, 0, false); got != win {
		t.Errorf("unmeasured: got %+v, want %+v", got, win)
	}

	// A text area larger than the window is nonsense and is clamped rather
	// than trusted.
	if got := hostGridRect(win, 9000, 9000, true); got != win {
		t.Errorf("clamped: got %+v, want %+v", got, win)
	}
}

func TestParseXTWinOps(t *testing.T) {
	// CSI 14 t comes back as the text area, CSI 16 t as one cell.
	w, h, ok := parseXTWinOps("\x1b[4;600;790t", "\x1b[4;")
	if !ok || w != 790 || h != 600 {
		t.Errorf("text area: got %dx%d ok=%v, want 790x600", w, h, ok)
	}
	cw, ch, ok := parseXTWinOps("\x1b[6;20;10t", "\x1b[6;")
	if !ok || cw != 10 || ch != 20 {
		t.Errorf("cell: got %dx%d ok=%v, want 10x20", cw, ch, ok)
	}
	// An answer to the other question must not be read as this one.
	if _, _, ok := parseXTWinOps("\x1b[6;20;10t", "\x1b[4;"); ok {
		t.Error("a cell answer is not a text area answer")
	}
	for _, s := range []string{"", "\x1b[4;t", "garbage"} {
		if _, _, ok := parseXTWinOps(s, "\x1b[4;"); ok {
			t.Errorf("%q must not parse", s)
		}
	}
}

// The cell is preferred over the reported text area, because multiplied by the
// grid it is the text area exactly and owes nothing to what the terminal
// chooses to call padding.
func TestHostTextSizePrefersTheCell(t *testing.T) {
	saveCell := func() func() {
		hostTextMu.Lock()
		cw, chh, ck := hostCellW, hostCellH, hostCellKnown
		tw, th, tk := hostTextW, hostTextH, hostTextKnown
		hostTextMu.Unlock()
		return func() {
			hostTextMu.Lock()
			hostCellW, hostCellH, hostCellKnown = cw, chh, ck
			hostTextW, hostTextH, hostTextKnown = tw, th, tk
			hostTextMu.Unlock()
		}
	}()
	defer saveCell()

	hostTextMu.Lock()
	hostCellW, hostCellH, hostCellKnown = 10, 20, true
	hostTextW, hostTextH, hostTextKnown = 999, 999, true
	hostTextMu.Unlock()
	if w, h, ok := hostTextSize(80, 25); !ok || w != 800 || h != 500 {
		t.Errorf("with a cell: got %dx%d ok=%v, want 800x500", w, h, ok)
	}

	// Without a cell the reported text area is all there is.
	hostTextMu.Lock()
	hostCellKnown = false
	hostTextMu.Unlock()
	if w, h, ok := hostTextSize(80, 25); !ok || w != 999 || h != 999 {
		t.Errorf("without a cell: got %dx%d ok=%v", w, h, ok)
	}

	// And without either, nothing is known.
	hostTextMu.Lock()
	hostTextKnown = false
	hostTextMu.Unlock()
	if _, _, ok := hostTextSize(80, 25); ok {
		t.Error("nothing was measured, so nothing is known")
	}
}

// The window size ioctl is where the pixel size of the text area comes from
// on a real terminal. A pipe is not a terminal and must say so rather than
// answering with zeros that look like an answer.
func TestHostPixelsFromIoctlOnAPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Skipf("no pipe: %v", err)
	}
	defer r.Close()
	defer w.Close()

	if _, _, ok := hostPixelsFromIoctl(r); ok {
		t.Error("a pipe has no text area")
	}
	if _, _, ok := hostPixelsFromIoctl(nil); ok {
		t.Error("neither has nothing at all")
	}
}

// The answer is only complete once its terminating t has arrived, so a reply
// that is still being read is not mistaken for one that is short.
func TestAnswerComplete(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"\x1b[4;600;790t", true},
		{"\x1b[4;600;79", false},
		{"\x1b[6;20;10t", false},
		{"", false},
	}
	for _, c := range cases {
		if got := answerComplete(c.in, "\x1b[4;"); got != c.want {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
		}
	}
}

// The terminal answers in logical pixels and the X server reports the window
// in device pixels, and on a display at double scale those are not the same
// number. These are the figures out of the debug log that showed it: a text
// area of 640x408 inside a window of 1312x868, which put the picture at half
// size in the bottom left corner.
func TestHostScaleFindsAScaledDisplay(t *testing.T) {
	win := ttyx.Rect{X: 862, Y: 337, W: 1312, H: 868}
	if got := hostScale(win, 640, 408); got != 2 {
		t.Errorf("scale: got %d, want 2", got)
	}

	grid := hostGridRect(win, 640, 408, true)
	want := ttyx.Rect{X: 862, Y: 337 + 868 - 816, W: 1280, H: 816}
	if grid != want {
		t.Errorf("grid: got %+v, want %+v", grid, want)
	}
	// Sixteen by thirty four, which is what the terminal really draws with.
	if cw, ch := grid.W/80, grid.H/24; cw != 16 || ch != 34 {
		t.Errorf("cell: got %dx%d, want 16x34", cw, ch)
	}
}

// An unscaled display must be left alone: the text area is nearly the window
// already, and doubling it would not fit.
func TestHostScaleLeavesAnUnscaledDisplayAlone(t *testing.T) {
	win := ttyx.Rect{X: 0, Y: 0, W: 1312, H: 868}
	if got := hostScale(win, 1280, 816); got != 1 {
		t.Errorf("scale: got %d, want 1", got)
	}
	if got := hostGridRect(win, 1280, 816, true); got.W != 1280 || got.H != 816 {
		t.Errorf("grid: got %+v", got)
	}
}

// Nonsense must not be scaled into worse nonsense.
func TestHostScaleRefusesNonsense(t *testing.T) {
	win := ttyx.Rect{W: 800, H: 600}
	if got := hostScale(win, 0, 0); got != 1 {
		t.Errorf("nothing measured: got %d", got)
	}
	if got := hostScale(win, 9000, 9000); got != 1 {
		t.Errorf("larger than the window: got %d", got)
	}
}

// The nudge is the only way anyone can supply a number nothing else knows:
// the padding a terminal keeps between its widget and its grid is reported
// nowhere. Zero by default, so it changes nothing unless it is set.
func TestOverlayOffsetIsAppliedAndDefaultsToNothing(t *testing.T) {
	if AppConfig.ImageX11OffsetX != 0 || AppConfig.ImageX11OffsetY != 0 {
		t.Errorf("the nudge must default to nothing: %d,%d",
			AppConfig.ImageX11OffsetX, AppConfig.ImageX11OffsetY)
	}

	saveX, saveY := AppConfig.ImageX11OffsetX, AppConfig.ImageX11OffsetY
	defer func() { AppConfig.ImageX11OffsetX, AppConfig.ImageX11OffsetY = saveX, saveY }()

	// A nil overlay has no session to ask, so this exercises the arithmetic
	// on its own: the grid moves by the nudge and by nothing else.
	AppConfig.ImageX11OffsetX, AppConfig.ImageX11OffsetY = 3, -2
	grid := ttyx.Rect{X: 100, Y: 200, W: 800, H: 600}
	want := ttyx.Rect{X: 103, Y: 198, W: 800, H: 600}
	if got := nudgeGrid(grid); got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
