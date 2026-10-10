//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"testing"

	"github.com/gogpu/gg/text"
)

// TestGogpuHost_SetFont mirrors the other backends' SetFont test (see
// x11_host_test.go's TestX11Host_SetFont): it checks the reload actually
// reaches the host's own cell size, the screen's graphics layer and the
// renderer, and that the renderer's per-font caches are dropped rather than
// served stale for the new font (vtui #136).
func TestGogpuHost_SetFont(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	host := &GogpuHost{
		cols:  10,
		rows:  5,
		cellW: 1,
		cellH: 1,
		scr:   scr,
	}
	renderer := NewGogpuRenderer(host, nil, 1, 1)
	renderer.faceCache = map[rune]text.Face{'A': nil}
	renderer.glyphMemo = map[rune]glyphMemoEntry{'A': {}}
	renderer.gfxKnown = true
	scr.Renderer = renderer

	_, _, wantCellW, wantCellH := loadGogpuFont("NonExistentFontAtAll", 20)

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
	if len(renderer.faceCache) != 0 {
		t.Errorf("faceCache not cleared: %d entries left", len(renderer.faceCache))
	}
	if len(renderer.glyphMemo) != 0 {
		t.Errorf("glyphMemo not cleared: %d entries left", len(renderer.glyphMemo))
	}
	if renderer.gfxKnown {
		t.Error("gfxKnown not reset, graphics layer would skip its next redraw")
	}
}

// A host without an app or screen yet -- SetFont called before the window
// exists -- must not panic, mirroring WaylandHost's
// TestWaylandHost_SetFontWithoutWidget and X11Host's
// TestX11Host_SetFontWithoutConn.
func TestGogpuHost_SetFontWithoutAppOrScreen(t *testing.T) {
	host := &GogpuHost{}
	host.SetFont("NonExistentFontAtAll", 20)
	if host.cellW <= 0 || host.cellH <= 0 {
		t.Errorf("cell size = %dx%d, want positive fallback values", host.cellW, host.cellH)
	}
}

// GogpuRenderer.SetFont is the fontSetter entry point frameManager.SetFont
// dispatches to (see TestFrameManagerSetFont_UsesRenderer); it forwards
// straight to GogpuHost.SetFont and reports whether it had a host to
// forward to.
func TestGogpuRenderer_SetFont(t *testing.T) {
	host := &GogpuHost{cellW: 1, cellH: 1}
	renderer := NewGogpuRenderer(host, nil, 1, 1)

	if ok := renderer.SetFont("NonExistentFontAtAll", 20); !ok {
		t.Error("GogpuRenderer.SetFont = false, want true")
	}
	if renderer.cellW <= 0 || renderer.cellH <= 0 {
		t.Errorf("renderer cell size = %dx%d, want positive fallback values", renderer.cellW, renderer.cellH)
	}
}

func TestGogpuRenderer_SetFontNoHost(t *testing.T) {
	renderer := &GogpuRenderer{}
	if ok := renderer.SetFont("Any", 20); ok {
		t.Error("GogpuRenderer.SetFont = true with no host, want false")
	}
}
