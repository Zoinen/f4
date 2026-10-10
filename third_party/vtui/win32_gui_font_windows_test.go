//go:build windows

package vtui

import (
	"image"
	"testing"
)

// SetFont reloads the font and pushes the new cell size to the renderer and
// the screen's graphics layer, without touching the grid geometry -- the
// window resize (through wmPerformResize, which needs a live hwnd, absent
// in this unit test) is the only thing that follows the cell size, not
// cols/rows (vtui #136).
func TestWin32GuiHost_SetFont(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	host := &Win32GuiHost{
		cols:     10,
		rows:     5,
		cellW:    1,
		cellH:    1,
		fontName: "old-font",
		fontSize: 12,
		fontDPI:  72.0,
		scr:      scr,
	}
	renderer := NewWin32GuiRenderer(host, nil, host.cellW, host.cellH)
	renderer.glyphCache[glyphKey{}] = &image.RGBA{}
	renderer.gfxKnown = true
	host.renderer = renderer

	_, wantCellW, wantCellH := loadBestFont("NonExistentFontAtAll", 20, 72.0)

	if renderer.face != nil {
		t.Fatal("setup: renderer already has a face before SetFont")
	}
	host.SetFont("NonExistentFontAtAll", 20)

	if host.fontName != "NonExistentFontAtAll" || host.fontSize != 20 {
		t.Fatalf("fontName/fontSize = %q/%v, want NonExistentFontAtAll/20", host.fontName, host.fontSize)
	}
	if host.cellW != wantCellW || host.cellH != wantCellH {
		t.Fatalf("host cell size = %dx%d, want %dx%d", host.cellW, host.cellH, wantCellW, wantCellH)
	}
	if cw, ch := scr.Graphics().CellSize(); cw != wantCellW || ch != wantCellH {
		t.Fatalf("screen cell size = %dx%d, want %dx%d", cw, ch, wantCellW, wantCellH)
	}
	if renderer.cellW != wantCellW || renderer.cellH != wantCellH {
		t.Fatalf("renderer cell size = %dx%d, want %dx%d", renderer.cellW, renderer.cellH, wantCellW, wantCellH)
	}
	if renderer.face == nil {
		t.Error("renderer face was not set")
	}
	if len(renderer.glyphCache) != 0 {
		t.Errorf("glyphCache not cleared: %d entries left", len(renderer.glyphCache))
	}
	if renderer.gfxKnown {
		t.Error("gfxKnown not reset after font change")
	}
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid = %dx%d, want unchanged 10x5", host.cols, host.rows)
	}
}

// A host without a live window (no hwnd yet, as in the test above) must not
// panic when SetFont is called -- mirrors the "testable before a native
// window exists" contract Win32GuiHost.ResizeGrid documents, and
// WaylandHost.SetFont's/X11Host.SetFont's own tests.
func TestWin32GuiHost_SetFontWithoutWindow(t *testing.T) {
	host := &Win32GuiHost{fontName: "old-font", fontSize: 12, fontDPI: 72.0}
	host.SetFont("NonExistentFontAtAll", 20)
	if host.fontName != "NonExistentFontAtAll" {
		t.Errorf("fontName = %q, want NonExistentFontAtAll", host.fontName)
	}
}

// Win32GuiRenderer.SetFont is the fontSetter entry point
// frameManager.SetFont dispatches to (see
// TestFrameManagerSetFont_UsesRenderer); it reports true and forwards
// straight to Win32GuiHost.SetFont.
func TestWin32GuiRenderer_SetFont(t *testing.T) {
	host := &Win32GuiHost{fontName: "old-font", fontSize: 12, fontDPI: 72.0}
	renderer := NewWin32GuiRenderer(host, nil, 8, 16)
	host.renderer = renderer

	if ok := renderer.SetFont("NonExistentFontAtAll", 20); !ok {
		t.Error("Win32GuiRenderer.SetFont = false, want true")
	}
	if host.fontName != "NonExistentFontAtAll" || host.fontSize != 20 {
		t.Fatalf("fontName/fontSize = %q/%v, want NonExistentFontAtAll/20", host.fontName, host.fontSize)
	}
}
