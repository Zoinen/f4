package terminal

import (
	"testing"
)

// kittySizePty records the geometry the terminal pushes to the child.
type kittySizePty struct {
	mockPty
	cols, rows, xpixel, ypixel int
}

func (p *kittySizePty) SetSizePixels(cols, rows, xpixel, ypixel int) {
	p.cols, p.rows, p.xpixel, p.ypixel = cols, rows, xpixel, ypixel
}

func TestTerminalPushesPixelSizeToTheChild(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &kittySizePty{}
	tv.Pty = Pty

	// Cells are ten by twenty pixels on that screen.
	scr := kittyGraphicsScreen(80, 24)
	tv.SetPosition(0, 0, 79, 23)
	tv.Show(scr)

	if Pty.cols != 80 || Pty.rows != 24 {
		t.Errorf("size in cells: got %dx%d", Pty.cols, Pty.rows)
	}
	if Pty.xpixel != 800 || Pty.ypixel != 480 {
		t.Errorf("size in pixels: got %dx%d", Pty.xpixel, Pty.ypixel)
	}
}

func TestSetPtySizeNeedsAKnownCell(t *testing.T) {
	sized := &kittySizePty{}
	SetPtySize(sized, 80, 24, 10, 20)
	if sized.xpixel != 800 || sized.ypixel != 480 {
		t.Errorf("pixels: got %dx%d", sized.xpixel, sized.ypixel)
	}

	// An unknown cell size must not be invented: a zero says "I do not
	// know", a wrong number sends the client off in the wrong direction.
	unknown := &kittySizePty{}
	SetPtySize(unknown, 80, 24, 0, 0)
	if unknown.xpixel != 0 || unknown.ypixel != 0 {
		t.Errorf("pixels: got %dx%d", unknown.xpixel, unknown.ypixel)
	}

	// A backend without pixel support still gets its size in cells.
	SetPtySize(&mockPty{}, 80, 24, 10, 20)
}

func TestTerminalReportsTheRealCellSize(t *testing.T) {
	e := newKittyEnv(t)
	scr := kittyGraphicsScreen(80, 24)
	e.tv.SetPosition(0, 0, 79, 23)
	e.tv.Show(scr)

	e.Pty.Reset()
	e.p.Process([]byte("\x1b[16t"))
	if got, want := e.Pty.String(), "\x1b[6;20;10t"; got != want {
		t.Errorf("CSI 16 t: expected %q, got %q", want, got)
	}

	e.Pty.Reset()
	e.p.Process([]byte("\x1b[14t"))
	if got, want := e.Pty.String(), "\x1b[4;480;800t"; got != want {
		t.Errorf("CSI 14 t: expected %q, got %q", want, got)
	}
}

func TestTerminalCellSizeFallback(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	if cw, ch := tv.CellSize(); cw != kittyFallbackCellW || ch != kittyFallbackCellH {
		t.Errorf("fallback cell size: got %dx%d", cw, ch)
	}
}

func TestKittyCellSpanUsesTheHostCellSize(t *testing.T) {
	e := newKittyEnv(t)
	scr := kittyGraphicsScreen(80, 24)
	e.tv.SetPosition(0, 0, 79, 23)
	e.tv.Show(scr)

	// Forty by forty pixels, ten columns wide, on ten by twenty cells:
	// 40 * 10 * 10 / (40 * 20) = 5 rows.
	kittySendImage(e, "a=T,i=1,f=32,s=40,v=40,c=10", 40, 40)
	if len(e.tv.Images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.Images))
	}
	if got := e.tv.Images[0].Rows; got != 5 {
		t.Errorf("rows: expected 5, got %d", got)
	}
}
