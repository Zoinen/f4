package wincon

import "testing"

// Windows Terminal hosts the console in a pseudoconsole whose window exists
// and is never shown. Drawing over it would put the picture nowhere, and it
// needs no overlay in any case: it renders sixel itself, so the pictures go
// down the wire like they do on any capable terminal.
func TestOnlyAVisibleConsoleIsDrawnOn(t *testing.T) {
	cases := map[Source]bool{
		SourceConsole: true,
		SourceHidden:  false,
		SourceNone:    false,
	}
	for src, want := range cases {
		if got := src.Trusted(); got != want {
			t.Errorf("%v: trusted=%v, want %v", src, got, want)
		}
	}
	if SourceHidden.String() == SourceConsole.String() {
		t.Error("the two must be distinguishable in a log")
	}
}

// A console reports its font size, so a cell rectangle is arithmetic and not
// an estimate: nothing to infer, nothing to round, nothing to be a pixel out
// by. This is the one place the Windows side is simpler than the X side.
func TestCellRect(t *testing.T) {
	got, ok := CellRect(8, 16, 2, 3, 9, 5)
	want := Rect{X: 16, Y: 48, W: 64, H: 48}
	if !ok || got != want {
		t.Errorf("got %+v ok=%v, want %+v", got, ok, want)
	}

	for _, bad := range [][4]int{{0, 16, 0}, {8, 0, 0}} {
		if _, ok := CellRect(bad[0], bad[1], 0, 0, 1, 1); ok {
			t.Errorf("a cell of %dx%d is not a cell", bad[0], bad[1])
		}
	}
	if _, ok := CellRect(8, 16, 5, 0, 2, 1); ok {
		t.Error("a rectangle that ends before it starts is not a rectangle")
	}
}

func TestClipToClient(t *testing.T) {
	// Hanging off the right and the bottom.
	got, ok := ClipToClient(Rect{X: 90, Y: 40, W: 40, H: 40}, 100, 60)
	if !ok || got != (Rect{X: 90, Y: 40, W: 10, H: 20}) {
		t.Errorf("got %+v ok=%v", got, ok)
	}
	// Starting before the corner.
	got, ok = ClipToClient(Rect{X: -10, Y: -5, W: 40, H: 40}, 100, 60)
	if !ok || got != (Rect{X: 0, Y: 0, W: 30, H: 35}) {
		t.Errorf("got %+v ok=%v", got, ok)
	}
	// Entirely outside.
	if _, ok := ClipToClient(Rect{X: 200, Y: 0, W: 10, H: 10}, 100, 60); ok {
		t.Error("nothing of it is on the screen")
	}
}

// One frame of thumbnails goes into one window covering all of them, with the
// gaps cut back out, so the captions between the tiles stay the console's.
func TestUnion(t *testing.T) {
	got, ok := Union([]Rect{
		{X: 10, Y: 10, W: 20, H: 20},
		{X: 50, Y: 5, W: 10, H: 10},
		{X: 0, Y: 0, W: 0, H: 0}, // empty ones are ignored
	})
	if !ok || got != (Rect{X: 10, Y: 5, W: 50, H: 25}) {
		t.Errorf("got %+v ok=%v, want {10 5 50 25}", got, ok)
	}
	if _, ok := Union(nil); ok {
		t.Error("nothing has no union")
	}
	if _, ok := Union([]Rect{{W: 0, H: 0}}); ok {
		t.Error("an empty rectangle has no union either")
	}
}

// A device independent bitmap is bottom-up and its channels are the other way
// round, so the copy has to turn the picture over and swap red with blue. Both
// mistakes look like a picture, which is why this is checked rather than read.
func TestBlitIntoTurnsThePictureOverAndSwapsChannels(t *testing.T) {
	// Two rows of one pixel: red on top, blue underneath.
	src := []byte{
		255, 0, 0, 255,
		0, 0, 255, 255,
	}
	dst := make([]byte, 1*2*4)
	blitInto(dst, 1, 2, src, 1, 2, 4, 0, 0)

	// The last row of the buffer is the top row of the picture, in BGRA.
	bottom := dst[0:4]
	top := dst[4:8]
	if top[0] != 0 || top[1] != 0 || top[2] != 255 {
		t.Errorf("the top row should be red as BGRA: %v", top)
	}
	if bottom[0] != 255 || bottom[1] != 0 || bottom[2] != 0 {
		t.Errorf("the bottom row should be blue as BGRA: %v", bottom)
	}
}

// Anything past the edge is dropped rather than wrapping onto the next row.
func TestBlitIntoClips(t *testing.T) {
	dst := make([]byte, 4*4*4)
	// Two opaque pixels whose channels are their own labels. The alpha has
	// to be a real 255 rather than another label: blitInto composes now, so
	// a pixel with alpha 4 lands as a twentieth of itself over the black
	// buffer, which is nothing, and this test would then be measuring the
	// blend instead of the clipping.
	src := []byte{1, 2, 3, 255, 5, 6, 7, 255}
	blitInto(dst, 4, 4, src, 2, 1, 8, 3, 3)
	// Only the first of the two pixels is inside.
	if dst[0*4+3*4+2] != 1 {
		t.Errorf("the pixel inside must land: %v", dst[0:16])
	}
}

func TestClassifyConsoleWindowDoesNotTrustAPseudoConsole(t *testing.T) {
	// The case this exists for: Windows Terminal's helper window answers
	// "visible" while being 0x0, so visibility alone said "draw here" and
	// every frame went nowhere (WINCON_805_HANDOVER F2, F3).
	if got := ClassifyConsoleWindow("PseudoConsoleWindow", true); got != SourcePseudo {
		t.Fatalf("a visible pseudo console window classified as %v", got)
	}
	if ClassifyConsoleWindow("PseudoConsoleWindow", true).Trusted() {
		t.Fatal("a pseudo console window must never be drawn over")
	}
	if got := ClassifyConsoleWindow("PseudoConsoleWindow", false); got != SourcePseudo {
		t.Fatalf("a hidden pseudo console window classified as %v", got)
	}
}

func TestClassifyConsoleWindowTrustsOnlyAVisibleClassicConsole(t *testing.T) {
	if got := ClassifyConsoleWindow("ConsoleWindowClass", true); got != SourceConsole || !got.Trusted() {
		t.Fatalf("a visible classic console classified as %v", got)
	}
	if got := ClassifyConsoleWindow("ConsoleWindowClass", false); got != SourceHidden || got.Trusted() {
		t.Fatalf("an off-screen classic console classified as %v", got)
	}
}

func TestClassifyConsoleWindowDistrustsWhatItDoesNotKnow(t *testing.T) {
	// An unfamiliar class is not evidence of anything, and a wrong guess here
	// costs a picture drawn where nobody can see it.
	for _, class := range []string{"", "Chrome_WidgetWin_1", "CASCADIA_HOSTING_WINDOW_CLASS"} {
		if got := ClassifyConsoleWindow(class, true); got.Trusted() {
			t.Errorf("class %q was trusted (%v)", class, got)
		}
	}
}
