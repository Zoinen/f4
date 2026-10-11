//go:build (linux || windows || darwin) && !android && (amd64 || arm64) && !vtui_noebiten

package vtui

import (
	"image"

	"testing"
)

// TestEbitenHost_SetFont mirrors the other backends' SetFont test (see
// x11_host_test.go's TestX11Host_SetFont): it checks the reload actually
// reaches the host's own cell size, the screen's graphics layer and the
// renderer, and that the renderer's per-font glyph cache is dropped rather
// than served stale for the new font (vtui #136).
func TestEbitenHost_SetFont(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	host := &EbitenHost{
		cols:  10,
		rows:  5,
		cellW: 1,
		cellH: 1,
		scale: 1,
		scr:   scr,
	}
	renderer := NewEbitenRenderer(host, nil, 1, 1, 1)
	renderer.glyphCache[glyphKey{}] = &image.RGBA{}
	renderer.gfxKnown = true
	host.renderer = renderer

	_, wantCellW, wantCellH := loadBestFont("NonExistentFontAtAll", 20, 72)

	host.SetFont("NonExistentFontAtAll", 20)

	if host.cellW != wantCellW || host.cellH != wantCellH {
		t.Fatalf("host cell size = %dx%d, want %dx%d", host.cellW, host.cellH, wantCellW, wantCellH)
	}
	if cw, ch := scr.Graphics().CellSize(); cw != wantCellW || ch != wantCellH {
		t.Fatalf("screen cell size = %dx%d, want %dx%d", cw, ch, wantCellW, wantCellH)
	}
	if renderer.cellW != wantCellW || renderer.cellH != wantCellH {
		t.Fatalf("renderer cell size = %dx%d, want %dx%d", renderer.cellW, renderer.cellH, wantCellW, wantCellH)
	}
	if len(renderer.glyphCache) != 0 {
		t.Errorf("glyphCache not cleared: %d entries left", len(renderer.glyphCache))
	}
	if renderer.gfxKnown {
		t.Error("gfxKnown not reset, graphics layer would skip its next redraw")
	}

	h, w := host.pendingSize.h, host.pendingSize.w
	if !host.pendingSize.valid || w != host.cols*wantCellW || h != host.rows*wantCellH {
		t.Errorf("pendingSize = %dx%d (valid=%v), want %dx%d (valid=true)",
			w, h, host.pendingSize.valid, host.cols*wantCellW, host.rows*wantCellH)
	}
}

// A host without a renderer or screen yet -- SetFont called before the
// window exists -- must not panic, mirroring WaylandHost's
// TestWaylandHost_SetFontWithoutWidget and X11Host's
// TestX11Host_SetFontWithoutConn.
func TestEbitenHost_SetFontWithoutRendererOrScreen(t *testing.T) {
	host := &EbitenHost{scale: 1}
	host.SetFont("NonExistentFontAtAll", 20)
	if host.cellW <= 0 || host.cellH <= 0 {
		t.Errorf("cell size = %dx%d, want positive fallback values", host.cellW, host.cellH)
	}
}

// EbitenRenderer.SetFont is the fontSetter entry point frameManager.SetFont
// dispatches to (see TestFrameManagerSetFont_UsesRenderer); it forwards
// straight to EbitenHost.SetFont and reports whether it had a host to
// forward to.
func TestEbitenRenderer_SetFont(t *testing.T) {
	host := &EbitenHost{cellW: 1, cellH: 1, scale: 1}
	renderer := NewEbitenRenderer(host, nil, 1, 1, 1)
	host.renderer = renderer

	if ok := renderer.SetFont("NonExistentFontAtAll", 20); !ok {
		t.Error("EbitenRenderer.SetFont = false, want true")
	}
	if renderer.cellW <= 0 || renderer.cellH <= 0 {
		t.Errorf("renderer cell size = %dx%d, want positive fallback values", renderer.cellW, renderer.cellH)
	}
}

func TestEbitenRenderer_SetFontNoHost(t *testing.T) {
	renderer := &EbitenRenderer{}
	if ok := renderer.SetFont("Any", 20); ok {
		t.Error("EbitenRenderer.SetFont = true with no host, want false")
	}
}
