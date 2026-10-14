package vtui

import (
	"image"
	"testing"
)

var coverSurface = NewImageSurface(8, 5)

// coverPass paints one frame the way FrameManager does: a panel declares a
// picture over cells (2,2)-(9,6), then paint, if any, runs on top of it.
func coverPass(t *testing.T, scr *ScreenBuf, paint func()) {
	t.Helper()
	g := scr.Graphics()
	g.BeginFrame()
	scr.FillRect(0, 0, 19, 9, ' ', 0) // the panel, painted before its picture
	g.DrawImage("panel", ImagePlacement{Surface: coverSurface, Col: 2, Row: 2, Cols: 8, Rows: 5})
	if paint != nil {
		paint()
	}
	g.EndFrame()
}

func newCoverScreen(protocol GraphicsProtocol) *ScreenBuf {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)
	scr.Graphics().SetProtocol(protocol)
	return scr
}

// Whatever is painted over a picture after it was declared covers it, cell
// by cell; what was painted before lies under it (f4#1885).
func TestCoverFollowsPaintersOrder(t *testing.T) {
	scr := newCoverScreen(GraphicsNative)
	coverPass(t, scr, func() {
		scr.Write(6, 3, StringToCharInfo("menu", 0)) // (6..9, 3)
		scr.ApplyShadow(8, 5, 12, 5)                 // (8..9, 5) inside
		scr.FillRect(0, 8, 19, 9, ' ', 0)            // below the picture
	})
	list, _ := scr.Graphics().Snapshot(nil)
	if len(list) != 1 {
		t.Fatalf("native snapshot = %d placements, want 1", len(list))
	}
	m := list[0].Covered
	for _, c := range [][2]int{{6, 3}, {9, 3}} {
		if !m.Covered(c[0], c[1]) {
			t.Errorf("cell %v painted over the picture is not covered", c)
		}
	}
	for _, c := range [][2]int{{8, 5}, {9, 5}} {
		if m.Covered(c[0], c[1]) || !m.Shaded(c[0], c[1]) {
			t.Errorf("cell %v under a shadow must be shaded, not covered", c)
		}
	}
	for _, c := range [][2]int{{2, 2}, {5, 3}, {7, 5}, {9, 6}} {
		if m.Covered(c[0], c[1]) || m.Shaded(c[0], c[1]) {
			t.Errorf("cell %v is marked, but nothing was painted over it", c)
		}
	}
	if m.Full() {
		t.Error("a partly covered picture reported full cover")
	}
}

// The menu closing uncovers the picture, and both transitions bump the
// generation so every backend redraws it; an unchanged menu does not.
func TestCoverChangesBumpGeneration(t *testing.T) {
	scr := newCoverScreen(GraphicsNative)
	g := scr.Graphics()
	menu := func() { scr.FillRect(5, 3, 15, 4, ' ', 0) }

	coverPass(t, scr, nil)
	gen := g.Generation()
	coverPass(t, scr, menu)
	if g.Generation() == gen {
		t.Fatal("covering the picture must bump the generation")
	}
	gen = g.Generation()
	coverPass(t, scr, menu)
	if g.Generation() != gen {
		t.Fatal("the same cover two frames running must not bump the generation")
	}
	coverPass(t, scr, nil)
	if g.Generation() == gen {
		t.Fatal("uncovering the picture must bump the generation")
	}
	if list, _ := g.Snapshot(nil); list[0].Covered.Any() {
		t.Fatal("the closed menu still covers the picture")
	}
}

// Terminal protocols cannot cut a picture per cell: a covered placement
// reaches them as crops over its uncovered rectangles, each showing the part
// of the picture the whole placement shows there. Native drops only a fully
// covered one.
func TestCoverCropsPictureForTerminalProtocols(t *testing.T) {
	for _, proto := range []GraphicsProtocol{GraphicsKitty, GraphicsSixel, GraphicsITerm2, GraphicsFar2l} {
		scr := newCoverScreen(proto)
		// A dialog over the right half of rows 3-4 of the 8x5 picture.
		coverPass(t, scr, func() { scr.FillRect(6, 3, 15, 4, ' ', 0) })
		list, _ := scr.Graphics().Snapshot(nil)
		cells := 0
		ids := map[uint32]bool{}
		for _, c := range list {
			cells += c.Cols * c.Rows
			ids[c.ID] = true
			for row := c.Row; row < c.Row+c.Rows; row++ {
				for col := c.Col; col < c.Col+c.Cols; col++ {
					if col >= 6 && row >= 3 && row <= 4 {
						t.Fatalf("protocol %v: crop %+v reaches the covered cell (%d,%d)", proto, c, col, row)
					}
				}
			}
			// 8x5 surface over 8x5 cells: one pixel per cell.
			if c.SrcX != c.Col-2 || c.SrcY != c.Row-2 || c.SrcW != c.Cols || c.SrcH != c.Rows {
				t.Fatalf("protocol %v: crop %+v shows the wrong part of the picture", proto, c)
			}
		}
		if cells != 8*5-4*2 || len(ids) != len(list) {
			t.Fatalf("protocol %v: crops cover %d cells with %d ids over %d crops, want 32 cells, distinct ids", proto, cells, len(ids), len(list))
		}
		if len(list) > 3 {
			t.Errorf("protocol %v: an L-shaped remainder took %d crops", proto, len(list))
		}

		coverPass(t, scr, nil)
		if list, _ := scr.Graphics().Snapshot(nil); len(list) != 1 || list[0].Covered != nil {
			t.Errorf("protocol %v: the uncovered picture must come back whole", proto)
		}
	}

	scr := newCoverScreen(GraphicsNative)
	coverPass(t, scr, func() { scr.FillRect(0, 0, 19, 9, ' ', 0) })
	if list, _ := scr.Graphics().Snapshot(nil); len(list) != 0 {
		t.Error("native: a fully covered picture leaves nothing to draw")
	}
}

// A shadow darkens the picture under it; something painted over the shadow
// covers the cell again, and a shadow over a cover does not lift it.
func TestShadowShadesPicture(t *testing.T) {
	scr := newCoverScreen(GraphicsNative)
	coverPass(t, scr, func() {
		scr.ApplyShadow(3, 3, 6, 3)
		scr.Write(5, 3, StringToCharInfo("x", 0)) // over the shadow
		scr.FillRect(3, 4, 4, 4, ' ', 0)
		scr.ApplyShadow(3, 4, 4, 4) // over the menu
	})
	list, _ := scr.Graphics().Snapshot(nil)
	m := list[0].Covered
	if !m.Shaded(3, 3) || !m.Shaded(6, 3) || !m.Covered(5, 3) || !m.Covered(3, 4) || m.Shaded(3, 4) {
		t.Fatal("shadow and cover do not stack in painting order")
	}

	// A terminal cannot darken a picture: a shadow alone leaves it whole.
	for _, proto := range []GraphicsProtocol{GraphicsKitty, GraphicsSixel} {
		scr := newCoverScreen(proto)
		coverPass(t, scr, func() { scr.ApplyShadow(3, 3, 6, 6) })
		if list, _ := scr.Graphics().Snapshot(nil); len(list) != 1 || list[0].Cols != 8 || list[0].Rows != 5 {
			t.Errorf("protocol %v: a shadow alone must leave the picture whole, got %+v", proto, list)
		}
	}
}

// Writes outside a painting pass, and placements added outside one, never
// produce a cover.
func TestCoverOnlyInsidePaintingPass(t *testing.T) {
	scr := newCoverScreen(GraphicsNative)
	coverPass(t, scr, nil)
	scr.Write(3, 3, StringToCharInfo("late", 0))
	if list, _ := scr.Graphics().Snapshot(nil); list[0].Covered.Any() {
		t.Fatal("a write after EndFrame must not cover the picture")
	}
}

// The native blit leaves covered cells' pixels — the menu — untouched.
func TestNativeBlitSkipsCoveredCells(t *testing.T) {
	const cell = 2
	dst := image.NewRGBA(image.Rect(0, 0, 20*cell, 10*cell))
	p := ImagePlacement{Surface: solidSurface(8*cell, 5*cell, 255, 0, 0, 255), Col: 2, Row: 2, Cols: 8, Rows: 5}
	p.Covered = newCoverMask(&p)
	p.Covered.cover(6, 3, 9, 3)

	var cache nativeGraphicsCache
	drawNativePlacements(dst, []ImagePlacement{p}, cell, cell, &cache)

	red := func(col, row int) bool {
		return dst.RGBAAt(col*cell, row*cell).R == 255
	}
	if !red(2, 2) || !red(5, 3) || !red(9, 6) {
		t.Error("uncovered cells must show the picture")
	}
	if red(6, 3) || red(9, 3) {
		t.Error("covered cells must keep what was painted over the picture")
	}

	// A shaded cell shows the picture at the 3/8 a shadow leaves.
	dst = image.NewRGBA(dst.Rect)
	p.Covered = newCoverMask(&p)
	p.Covered.shade(4, 4, 4, 4)
	drawNativePlacements(dst, []ImagePlacement{p}, cell, cell, &cache)
	if got, want := dst.RGBAAt(4*cell, 4*cell).R, byte(255*ShadeKeepNum/ShadeKeepDen); got != want {
		t.Errorf("shaded cell red = %d, want %d", got, want)
	}
	if !red(5, 4) {
		t.Error("the cell beside the shadow must keep the picture's colour")
	}
}

// gogpu draws fills under images, so it shades with a darkened copy of the
// picture; the copy is darkened like the blit, and the original is kept.
func TestShadedCopyLeavesOriginal(t *testing.T) {
	e := &nativeCacheEntry{surf: solidSurface(2, 2, 255, 128, 0, 255)}
	dark := e.asShadedImage()
	if got := dark.RGBAAt(1, 1); got.R != 255*ShadeKeepNum/ShadeKeepDen || got.G != 128*ShadeKeepNum/ShadeKeepDen || got.A != 255 {
		t.Fatalf("shaded copy pixel = %+v", got)
	}
	if got := e.asImage().RGBAAt(1, 1); got.R != 255 || got.G != 128 {
		t.Fatalf("shading the copy changed the original: %+v", got)
	}
}
