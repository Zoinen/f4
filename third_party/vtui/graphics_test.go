package vtui

import (
	"runtime"
	"testing"
)

func fakeEnv(pairs map[string]string) func(string) string {
	return func(k string) string { return pairs[k] }
}

func TestDetectGraphicsProtocol(t *testing.T) {
	// WezTerm speaks kitty natively on Unix. On Windows, and in a WSL session
	// hosted by WezTerm on Windows, the app is bridged through ConPTY, whose
	// capability the environment cannot express: env detection must stay
	// silent and leave the decision to the active DA1 query.
	weztermWant := GraphicsKitty
	if runtime.GOOS == "windows" {
		weztermWant = GraphicsNone
	}

	cases := []struct {
		name string
		env  map[string]string
		want GraphicsProtocol
	}{
		{"kitty by term", map[string]string{"TERM": "xterm-kitty"}, GraphicsKitty},
		{"kitty by window id", map[string]string{"KITTY_WINDOW_ID": "1"}, GraphicsKitty},
		{"ghostty", map[string]string{"TERM_PROGRAM": "ghostty"}, GraphicsKitty},
		{"wezterm", map[string]string{"WEZTERM_PANE": "0"}, weztermWant},
		{"wezterm via wsl", map[string]string{"WEZTERM_PANE": "0", "WSL_DISTRO_NAME": "Ubuntu"}, GraphicsNone},
		{"windows terminal", map[string]string{"WT_SESSION": "abc-123"}, GraphicsSixel},
		{"iterm", map[string]string{"TERM_PROGRAM": "iTerm.app"}, GraphicsITerm2},
		{"konsole", map[string]string{"KONSOLE_VERSION": "220400"}, GraphicsSixel},
		{"foot", map[string]string{"TERM": "foot-extra"}, GraphicsSixel},
		{"plain xterm", map[string]string{"TERM": "xterm-256color"}, GraphicsNone},
		{"tmux hides everything", map[string]string{"TMUX": "/tmp/s", "TERM": "xterm-kitty"}, GraphicsNone},
		{"screen hides everything", map[string]string{"TERM": "screen.xterm-kitty"}, GraphicsNone},
		{"override wins over tmux", map[string]string{"TMUX": "/tmp/s", "VTUI_GRAPHICS": "sixel"}, GraphicsSixel},
		{"override off", map[string]string{"TERM": "xterm-kitty", "VTUI_GRAPHICS": "none"}, GraphicsNone},
		{"bad override falls through", map[string]string{"TERM": "xterm-kitty", "VTUI_GRAPHICS": "nonsense"}, GraphicsKitty},
	}

	for _, tc := range cases {
		if got := detectGraphicsProtocol(fakeEnv(tc.env)); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEnvGraphicsProtocolsListsAlternates(t *testing.T) {
	// Windows Terminal offers only sixel; a plain xterm offers nothing.
	wt := envGraphicsProtocolsWith(fakeEnv(map[string]string{"WT_SESSION": "x"}))
	if len(wt) != 1 || wt[0] != GraphicsSixel {
		t.Errorf("windows terminal must offer only sixel, got %v", wt)
	}
	if got := envGraphicsProtocolsWith(fakeEnv(map[string]string{"TERM": "xterm-256color"})); len(got) != 0 {
		t.Errorf("a plain xterm must offer no protocols, got %v", got)
	}
	// WezTerm on Unix leads with kitty and lists sixel as the alternate. On
	// Windows, and in a WSL session hosted from Windows, the ConPTY bridge
	// makes the env result ambiguous, so the list stays empty and the active
	// DA1 query takes over.
	wezterm := envGraphicsProtocolsWith(fakeEnv(map[string]string{"WEZTERM_PANE": "0"}))
	if runtime.GOOS == "windows" {
		if len(wezterm) != 0 {
			t.Errorf("wezterm on windows must defer to the DA1 query, got %v", wezterm)
		}
	} else {
		if len(wezterm) != 2 || wezterm[0] != GraphicsKitty || wezterm[1] != GraphicsSixel {
			t.Errorf("wezterm on unix must offer kitty then sixel, got %v", wezterm)
		}
	}
	weztermWSL := envGraphicsProtocolsWith(fakeEnv(map[string]string{"WEZTERM_PANE": "0", "WSL_DISTRO_NAME": "Ubuntu"}))
	if len(weztermWSL) != 0 {
		t.Errorf("wezterm through a wsl conpty bridge must defer to the DA1 query, got %v", weztermWSL)
	}
}

func TestProbeGraphicsProtocolsWezTermAfterDA1(t *testing.T) {
	// A WezTerm reached through the modern ConPTY bridge declares sixel in
	// DA1 (parameter 4); when it does, kitty is also available and preferred.
	got := probeGraphicsProtocolsWith(
		fakeEnv(map[string]string{"WEZTERM_PANE": "0", "WSL_DISTRO_NAME": "Ubuntu"}),
		func() bool { return true },
	)
	if len(got) != 2 || got[0] != GraphicsKitty || got[1] != GraphicsSixel {
		t.Errorf("wezterm after a sixel-positive DA1 must offer kitty then sixel, got %v", got)
	}

	// An old ConPTY that swallows sixel also answers DA1 without parameter 4,
	// so no protocol is offered and the viewer falls back to half-block.
	got = probeGraphicsProtocolsWith(
		fakeEnv(map[string]string{"WEZTERM_PANE": "0", "WSL_DISTRO_NAME": "Ubuntu"}),
		func() bool { return false },
	)
	if len(got) != 0 {
		t.Errorf("wezterm through an old conpty must offer nothing, got %v", got)
	}

	// A bare conhost still offers only sixel.
	got = probeGraphicsProtocolsWith(fakeEnv(map[string]string{"TERM": "xterm"}), func() bool { return true })
	if len(got) != 1 || got[0] != GraphicsSixel {
		t.Errorf("a sixel conhost must offer only sixel, got %v", got)
	}
}

func TestParseCellSizeResponse(t *testing.T) {
	cw, ch, ok := parseCellSizeResponse("\x1b[6;20;10t")
	if !ok || cw != 10 || ch != 20 {
		t.Errorf("cell size = %dx%d ok=%v, want 10x20 true", cw, ch, ok)
	}
	if _, _, ok := parseCellSizeResponse("\x1b[?62;c"); ok {
		t.Error("a DA1 answer is not a cell size report")
	}
	if _, _, ok := parseCellSizeResponse("\x1b[6;0;10t"); ok {
		t.Error("a zero cell height must be rejected")
	}
}

func TestGraphicsLayerEffectiveCellSize(t *testing.T) {
	var g GraphicsLayer
	g.SetProtocol(GraphicsKitty)
	g.SetCellSize(9, 19)
	if cw, ch := g.EffectiveCellSize(); cw != 9 || ch != 19 {
		t.Errorf("kitty must use the reported cell, got %dx%d", cw, ch)
	}

	// Sixel uses the reported cell on terminals that rasterise at font size
	// (WezTerm), rather than conhost's fixed virtual cell.
	g.SetProtocol(GraphicsSixel)
	t.Setenv("WEZTERM_PANE", "0")
	if cw, ch := g.EffectiveCellSize(); cw != 9 || ch != 19 {
		t.Errorf("sixel on a font-sized terminal must use the reported cell, got %dx%d", cw, ch)
	}

	// Windows Terminal rasterises sixel into a fixed 10x20 virtual cell, so
	// the effective size overrides the font metrics the terminal reports.
	t.Setenv("WT_SESSION", "abc-123")
	if cw, ch := g.EffectiveCellSize(); cw != 10 || ch != 20 {
		t.Errorf("sixel on windows terminal must use 10x20, got %dx%d", cw, ch)
	}
}

func TestImageSurfacePixelAccess(t *testing.T) {
	s := NewImageSurface(3, 2)
	if !s.Valid() {
		t.Fatal("freshly allocated surface must be valid")
	}
	s.SetPixel(2, 1, 10, 20, 30, 40)
	r, g, b, a := s.PixelAt(2, 1)
	if r != 10 || g != 20 || b != 30 || a != 40 {
		t.Errorf("got %d,%d,%d,%d", r, g, b, a)
	}

	s.SetPixel(-1, 0, 1, 1, 1, 1)
	s.SetPixel(3, 0, 1, 1, 1, 1)
	if _, _, _, a := s.PixelAt(99, 99); a != 0 {
		t.Error("out of range read should be transparent")
	}
}

func TestImageSurfaceRejectsOverflowGeometry(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if NewImageSurface(maxInt, maxInt) != nil {
		t.Fatal("overflowing allocation must be rejected")
	}
	if NewImageSurfaceFromPix(maxInt, 2, maxInt, nil) != nil {
		t.Fatal("overflowing wrapped geometry must be rejected")
	}
	if (&ImageSurface{Width: maxInt, Height: 2, Stride: maxInt}).Valid() {
		t.Fatal("overflowing surface must be invalid")
	}
}

func TestNewImageSurfaceFromPixValidation(t *testing.T) {
	if NewImageSurfaceFromPix(4, 4, 16, make([]byte, 8)) != nil {
		t.Error("undersized buffer must be rejected")
	}
	if NewImageSurfaceFromPix(2, 2, 8, make([]byte, 16)) == nil {
		t.Error("exactly sized buffer must be accepted")
	}
}

func TestImageSurfaceHashTracksContent(t *testing.T) {
	a := NewImageSurface(2, 2)
	b := NewImageSurface(2, 2)
	if a.Hash() != b.Hash() {
		t.Fatal("identical surfaces must hash equal")
	}
	a.SetPixel(0, 0, 255, 0, 0, 255)
	if a.Hash() == b.Hash() {
		t.Fatal("hash must change after SetPixel")
	}

	before := a.Hash()
	a.Pix[7] = 0xAB
	if a.Hash() != before {
		t.Error("cached hash should survive until invalidated")
	}
	a.Invalidate()
	if a.Hash() == before {
		t.Error("hash must be recomputed after Invalidate")
	}
}

func TestImageSurfaceCrop(t *testing.T) {
	s := NewImageSurface(4, 4)
	s.SetPixel(2, 2, 1, 2, 3, 4)

	c := s.Crop(2, 2, 2, 2)
	if c == nil || c.Width != 2 || c.Height != 2 {
		t.Fatalf("unexpected crop %v", c)
	}
	if r, g, b, a := c.PixelAt(0, 0); r != 1 || g != 2 || b != 3 || a != 4 {
		t.Errorf("crop lost pixel data: %d,%d,%d,%d", r, g, b, a)
	}

	if c := s.Crop(3, 3, 10, 10); c == nil || c.Width != 1 || c.Height != 1 {
		t.Error("crop must clamp to source bounds")
	}
	if s.Crop(10, 10, 2, 2) != nil {
		t.Error("fully outside crop must return nil")
	}
}

func TestPlacementSourceDefaults(t *testing.T) {
	p := ImagePlacement{Surface: NewImageSurface(8, 6)}
	x, y, w, h := p.Source()
	if x != 0 || y != 0 || w != 8 || h != 6 {
		t.Errorf("empty source rect should mean whole surface, got %d,%d,%d,%d", x, y, w, h)
	}

	p.SrcX, p.SrcY, p.SrcW, p.SrcH = 6, 5, 10, 10
	x, y, w, h = p.Source()
	if x != 6 || y != 5 || w != 2 || h != 1 {
		t.Errorf("source rect must be clamped, got %d,%d,%d,%d", x, y, w, h)
	}
}

func TestPlacementCoversCell(t *testing.T) {
	p := ImagePlacement{Col: 2, Row: 3, Cols: 4, Rows: 2}
	if !p.CoversCell(2, 3) || !p.CoversCell(5, 4) {
		t.Error("corners must be covered")
	}
	if p.CoversCell(1, 3) || p.CoversCell(6, 4) || p.CoversCell(2, 5) {
		t.Error("cells outside the rectangle must not be covered")
	}
}

func TestGraphicsLayerLifecycle(t *testing.T) {
	var g GraphicsLayer
	g.SetProtocol(GraphicsKitty)

	gen0 := g.Generation()
	id := g.Add(ImagePlacement{Surface: NewImageSurface(2, 2), Cols: 1, Rows: 1})
	if id == 0 {
		t.Fatal("Add must return a non-zero id")
	}
	if g.Generation() == gen0 {
		t.Error("Add must bump the generation")
	}
	if g.Len() != 1 {
		t.Fatalf("expected 1 placement, got %d", g.Len())
	}

	gen1 := g.Generation()
	if !g.Update(id, func(p *ImagePlacement) { p.Col = 7 }) {
		t.Fatal("Update should find the placement")
	}
	if g.Generation() == gen1 {
		t.Error("Update must bump the generation")
	}
	list, _ := g.Snapshot(nil)
	if len(list) != 1 || list[0].Col != 7 || list[0].ID != id {
		t.Errorf("update did not apply: %+v", list)
	}

	if g.Update(id+1000, func(*ImagePlacement) {}) {
		t.Error("Update of an unknown id must fail")
	}
	if g.Remove(id + 1000) {
		t.Error("Remove of an unknown id must fail")
	}
	if !g.Remove(id) || g.Len() != 0 {
		t.Error("Remove must drop the placement")
	}

	genEmpty := g.Generation()
	g.Clear()
	if g.Generation() != genEmpty {
		t.Error("clearing an empty layer must not bump the generation")
	}
}

func TestGraphicsLayerSnapshotOrder(t *testing.T) {
	var g GraphicsLayer
	surf := NewImageSurface(1, 1)
	top := g.Add(ImagePlacement{Surface: surf, ZIndex: 5})
	bottom := g.Add(ImagePlacement{Surface: surf, ZIndex: -1})
	mid := g.Add(ImagePlacement{Surface: surf, ZIndex: 0})

	list, gen := g.Snapshot(nil)
	if gen != g.Generation() {
		t.Error("Snapshot must report the current generation")
	}
	if len(list) != 3 || list[0].ID != bottom || list[1].ID != mid || list[2].ID != top {
		t.Errorf("wrong z order: %v %v %v", list[0].ID, list[1].ID, list[2].ID)
	}

	list, _ = g.Snapshot(list)
	if len(list) != 3 {
		t.Errorf("reused snapshot buffer grew to %d", len(list))
	}
}

func TestGraphicsLayerProtocolOverride(t *testing.T) {
	var g GraphicsLayer
	g.SetProtocol(GraphicsNone)
	if g.Supported() {
		t.Error("GraphicsNone must not be reported as supported")
	}
	gen := g.Generation()
	g.SetProtocol(GraphicsNone)
	if g.Generation() != gen {
		t.Error("setting the same protocol must not bump the generation")
	}
	g.SetProtocol(GraphicsSixel)
	if !g.Supported() || g.Generation() == gen {
		t.Error("switching protocol must be observable")
	}
}

func TestGraphicsLayerCellSize(t *testing.T) {
	var g GraphicsLayer
	if w, h := g.CellSize(); w != 0 || h != 0 {
		t.Error("cell size must start unknown")
	}
	gen := g.Generation()
	g.SetCellSize(8, 16)
	if w, h := g.CellSize(); w != 8 || h != 16 {
		t.Errorf("got %dx%d", w, h)
	}
	if g.Generation() == gen {
		t.Error("cell size change must bump the generation")
	}
	gen = g.Generation()
	g.SetCellSize(8, 16)
	if g.Generation() != gen {
		t.Error("identical cell size must not bump the generation")
	}
}

func TestGraphicsLayerDirtyUnder(t *testing.T) {
	const w, h = 10, 5
	buf := make([]CharInfo, w*h)
	shadow := make([]CharInfo, w*h)

	var g GraphicsLayer
	g.Add(ImagePlacement{Surface: NewImageSurface(1, 1), Col: 2, Row: 1, Cols: 3, Rows: 2})

	if g.DirtyUnder(buf, shadow, w, h) {
		t.Error("identical buffers must not be dirty")
	}

	buf[0*w+0].Char = 'x'
	if g.DirtyUnder(buf, shadow, w, h) {
		t.Error("change outside the image must be ignored")
	}

	buf[1*w+3].Char = 'y'
	if !g.DirtyUnder(buf, shadow, w, h) {
		t.Error("change under the image must be reported")
	}

	if g.DirtyUnder(buf[:3], shadow[:3], w, h) {
		t.Error("undersized buffers must not report dirt")
	}
}
