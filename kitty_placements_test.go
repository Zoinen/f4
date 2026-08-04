package main

import (
	"testing"

	"github.com/unxed/vtui"
)

// kittyFakeRenderer is a renderer that draws nothing but claims to be able to
// display images, which is what ScreenBuf.SupportsGraphics asks for.
type kittyFakeRenderer struct{}

func (kittyFakeRenderer) Render(buf, shadow []vtui.CharInfo, width, height int, forceRedraw bool) {}
func (kittyFakeRenderer) SetCursor(x, y int, visible bool, shape vtui.CursorShape)                {}
func (kittyFakeRenderer) SetPalette(palette *[256]uint32)                                         {}
func (kittyFakeRenderer) SetWindowTitle(title string)                                             {}
func (kittyFakeRenderer) Flush()                                                                  {}
func (kittyFakeRenderer) RenderGraphics(layer *vtui.GraphicsLayer, buf, shadow []vtui.CharInfo, width, height int, forceRedraw bool) {
}

func kittyGraphicsScreen(w, h int) *vtui.ScreenBuf {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(w, h)
	scr.Renderer = kittyFakeRenderer{}
	scr.Graphics().SetProtocol(vtui.GraphicsNative)
	scr.Graphics().SetCellSize(10, 20)
	return scr
}

// kittySendImage transmits and displays an image of the given pixel size.
func kittySendImage(e *kittyEnv, control string, w, h int) {
	pix := make([]byte, w*h*4)
	for i := range pix {
		pix[i] = byte(i)
	}
	e.send(control, kittyB64(pix))
}

func TestKittyPlacementAtCursorMovesIt(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(3, 5)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=10,r=4", 4, 4)

	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	p := e.tv.images[0]
	if p.Col != 3 || p.Row != 5 {
		t.Errorf("placement position: got %d,%d", p.Col, p.Row)
	}
	if p.Cols != 10 || p.Rows != 4 {
		t.Errorf("placement size: got %dx%d cells", p.Cols, p.Rows)
	}
	if p.SrcW != 4 || p.SrcH != 4 {
		t.Errorf("source rectangle: got %dx%d", p.SrcW, p.SrcH)
	}

	// The cursor has to end up past the picture, otherwise the next line of
	// output would be printed over it.
	if e.tv.CursorX != 13 || e.tv.CursorY != 9 {
		t.Errorf("cursor: expected 13,9 got %d,%d", e.tv.CursorX, e.tv.CursorY)
	}
}

func TestKittyPlacementKeepsCursorWithC1(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(2, 2)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=3,r=3,C=1", 4, 4)

	if e.tv.CursorX != 2 || e.tv.CursorY != 2 {
		t.Errorf("C=1 must leave the cursor alone, got %d,%d", e.tv.CursorX, e.tv.CursorY)
	}
}

func TestKittyPlacementScrollsWithTheText(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(0, 23)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=4,r=4", 4, 4)

	// The screen scrolled by four lines to make room, so the picture moved
	// up by the same amount and the cursor stayed at the bottom.
	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	if got := e.tv.images[0].Row; got != 19 {
		t.Errorf("placement row after scrolling: expected 19, got %d", got)
	}
	if e.tv.CursorY != 23 {
		t.Errorf("cursor row: expected 23, got %d", e.tv.CursorY)
	}

	// Scrolling far enough forgets the picture entirely.
	e.tv.scrollUp(0, 23, 30)
	if len(e.tv.images) != 0 {
		t.Errorf("a picture scrolled out of the buffer must be forgotten")
	}
}

func TestKittyCellSpanKeepsAspect(t *testing.T) {
	e := newKittyEnv(t)
	// Cell metrics are unknown here, so the fallback 8x16 applies:
	// 50 * 10 * 8 / (100 * 16) = 2.5, rounded up to 3 rows.
	kittySendImage(e, "a=T,i=1,f=32,s=100,v=50,c=10", 100, 50)

	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	if got := e.tv.images[0].Rows; got != 3 {
		t.Errorf("rows computed from the aspect ratio: expected 3, got %d", got)
	}
}

func TestKittyRetransmitReplacesPlacement(t *testing.T) {
	e := newKittyEnv(t)
	kittySendImage(e, "a=T,i=7,f=32,s=4,v=4,c=2,r=2", 4, 4)
	kittySendImage(e, "a=T,i=7,f=32,s=4,v=4,c=2,r=2", 4, 4)

	if len(e.tv.images) != 1 {
		t.Errorf("re-transmitting an id must drop its old placements, got %d", len(e.tv.images))
	}
}

func TestKittyDeletePlacements(t *testing.T) {
	e := newKittyEnv(t)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=2,r=2", 4, 4)
	kittySendImage(e, "a=T,i=2,f=32,s=4,v=4,c=2,r=2", 4, 4)
	if len(e.tv.images) != 2 {
		t.Fatalf("expected two placements, got %d", len(e.tv.images))
	}

	e.send("a=d,d=i,i=1", "")
	if len(e.tv.images) != 1 || e.tv.images[0].ImageID != 2 {
		t.Fatalf("d=i must remove only the placements of image 1: %+v", e.tv.images)
	}
	// The lowercase form keeps the pixels around for a later put.
	if e.tv.kitty.Image(1) == nil {
		t.Error("d=i must not free the image data")
	}

	e.send("a=d", "")
	if len(e.tv.images) != 0 {
		t.Errorf("a bare a=d must remove every visible placement, got %d", len(e.tv.images))
	}
}

func TestKittyPlacementsClearedOnErase(t *testing.T) {
	e := newKittyEnv(t)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=2,r=2", 4, 4)
	e.p.Process([]byte("\x1b[2J"))
	if len(e.tv.images) != 0 {
		t.Errorf("clearing the screen must remove its pictures, got %d", len(e.tv.images))
	}
}

func TestKittyDrawsIntoTheGraphicsLayer(t *testing.T) {
	e := newKittyEnv(t)
	// The alternate screen has no visual gravity, which keeps the expected
	// coordinates obvious.
	e.p.Process([]byte("\x1b[?1049h"))
	e.tv.SetCursor(2, 3)
	kittySendImage(e, "a=T,i=1,f=32,s=8,v=8,c=4,r=2,C=1", 8, 8)

	scr := kittyGraphicsScreen(80, 24)
	e.tv.SetPosition(0, 0, 79, 23)
	e.tv.Show(scr)

	list, _ := scr.Graphics().Snapshot(nil)
	if len(list) != 1 {
		t.Fatalf("expected one image in the layer, got %d", len(list))
	}
	if list[0].Col != 2 || list[0].Row != 3 || list[0].Cols != 4 || list[0].Rows != 2 {
		t.Errorf("drawn geometry: %+v", list[0])
	}

	// A picture belonging to the other screen must not be painted.
	e.tv.images[0].Alt = false
	scr2 := kittyGraphicsScreen(80, 24)
	e.tv.Show(scr2)
	if list, _ := scr2.Graphics().Snapshot(nil); len(list) != 0 {
		t.Errorf("the primary screen picture leaked into the alternate one")
	}
}

func TestKittyResizeMovesPlacementsWithTheText(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(2, 5)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=6,r=3,C=1", 4, 4)

	if len(e.tv.images) != 1 || e.tv.images[0].Row != 5 {
		t.Fatalf("the placement did not land on row five: %v", e.tv.images)
	}

	// Four rows fall off the top, and what was on row five is now on row one.
	e.tv.Resize(80, 20)
	if len(e.tv.images) != 1 || e.tv.images[0].Row != 1 {
		t.Fatalf("after shrinking: %v", e.tv.images)
	}

	// Growing pushes everything back down by as much as it gained.
	e.tv.Resize(80, 26)
	if len(e.tv.images) != 1 || e.tv.images[0].Row != 7 {
		t.Fatalf("after growing: %v", e.tv.images)
	}
	if p := e.tv.images[0]; p.Cols != 6 || p.Rows != 3 {
		t.Errorf("a span the client asked for must not change: %dx%d", p.Cols, p.Rows)
	}
}

func TestKittyResizeForgetsWhatScrolledOff(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(0, 2)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=4,r=2,C=1", 4, 4)

	// Five rows go away, and the picture on rows two and three with them.
	e.tv.Resize(80, 19)
	if len(e.tv.images) != 0 {
		t.Fatalf("the placement should be gone: %v", e.tv.images)
	}
}

func TestKittyRecomputesSpanWhenACellChangesSize(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetPosition(0, 0, 79, 23)

	// A cell is ten by twenty pixels to begin with, so a forty by forty
	// picture takes four columns and two rows.
	scr := kittyGraphicsScreen(80, 24)
	e.tv.Show(scr)
	kittySendImage(e, "a=T,i=1,f=32,s=40,v=40,C=1", 40, 40)
	if p := e.tv.images[0]; p.Cols != 4 || p.Rows != 2 {
		t.Fatalf("span on a ten by twenty cell: %dx%d", p.Cols, p.Rows)
	}

	// The font gets smaller and the same picture needs more cells.
	scr.Graphics().SetCellSize(5, 10)
	e.tv.Show(scr)
	if p := e.tv.images[0]; p.Cols != 8 || p.Rows != 4 {
		t.Fatalf("span on a five by ten cell: %dx%d", p.Cols, p.Rows)
	}
}

func TestKittyKeepsTheSpanTheClientAskedFor(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetPosition(0, 0, 79, 23)
	scr := kittyGraphicsScreen(80, 24)
	e.tv.Show(scr)

	// The width is given and the height is left to us. A square picture four
	// columns wide is four cell widths tall, which on a ten by twenty cell
	// comes to two rows.
	kittySendImage(e, "a=T,i=1,f=32,s=40,v=40,c=4,C=1", 40, 40)
	if p := e.tv.images[0]; p.Cols != 4 || p.Rows != 2 {
		t.Fatalf("span: %dx%d", p.Cols, p.Rows)
	}

	// Halving both sides of the cell changes nothing at all: with the width
	// given in columns, the height in rows follows the shape of the cell and
	// not its size.
	scr.Graphics().SetCellSize(5, 10)
	e.tv.Show(scr)
	if p := e.tv.images[0]; p.Cols != 4 || p.Rows != 2 {
		t.Fatalf("a cell of the same shape must leave the span alone: %dx%d", p.Cols, p.Rows)
	}

	// A cell that becomes square makes those same four columns twice as tall.
	scr.Graphics().SetCellSize(10, 10)
	e.tv.Show(scr)
	if p := e.tv.images[0]; p.Cols != 4 || p.Rows != 4 {
		t.Fatalf("the given width must stand and only the height move: %dx%d", p.Cols, p.Rows)
	}
}

func TestKittyAltScreenKeepsAndDropsTheRightPictures(t *testing.T) {
	e := newKittyEnv(t)
	e.tv.SetCursor(0, 3)
	kittySendImage(e, "a=T,i=1,f=32,s=4,v=4,c=4,r=2,C=1", 4, 4)

	e.p.Process([]byte("\x1b[?1049h"))
	if len(e.tv.images) != 1 || e.tv.images[0].Alt {
		t.Fatalf("the picture of the main screen must survive the switch: %v", e.tv.images)
	}

	e.tv.SetCursor(0, 1)
	kittySendImage(e, "a=T,i=2,f=32,s=4,v=4,c=4,r=2,C=1", 4, 4)
	if len(e.tv.images) != 2 {
		t.Fatalf("both screens should hold a picture: %v", e.tv.images)
	}

	e.p.Process([]byte("\x1b[?1049l"))
	if len(e.tv.images) != 1 {
		t.Fatalf("the alternate screen must take its pictures with it: %v", e.tv.images)
	}
	if p := e.tv.images[0]; p.ImageID != 1 || p.Alt {
		t.Errorf("the wrong picture survived: %+v", p)
	}
}

func TestKittyClipPlacement(t *testing.T) {
	// Half of the picture hangs below the bottom of the terminal.
	p := vtui.ImagePlacement{Col: 0, Row: 20, Cols: 4, Rows: 4, SrcW: 80, SrcH: 80}
	if !kittyClipPlacement(&p, 0, 0, 79, 21) {
		t.Fatal("a partially visible placement must survive clipping")
	}
	if p.Rows != 2 || p.SrcH != 40 {
		t.Errorf("bottom clipping: got %d rows, source height %d", p.Rows, p.SrcH)
	}

	// Cut off at the top: the source rectangle moves down with it.
	p = vtui.ImagePlacement{Col: 0, Row: -2, Cols: 4, Rows: 4, SrcW: 80, SrcH: 80}
	if !kittyClipPlacement(&p, 0, 0, 79, 23) {
		t.Fatal("a placement clipped at the top must survive")
	}
	if p.Row != 0 || p.Rows != 2 || p.SrcY != 40 || p.SrcH != 40 {
		t.Errorf("top clipping: %+v", p)
	}

	// Completely outside.
	p = vtui.ImagePlacement{Col: 0, Row: 30, Cols: 4, Rows: 4, SrcW: 80, SrcH: 80}
	if kittyClipPlacement(&p, 0, 0, 79, 23) {
		t.Error("an invisible placement must be dropped")
	}
}
