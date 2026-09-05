package vtui

import (
	"bytes"
	"image"
	"testing"
)

func solidSurface(w, h int, r, g, b, a byte) *ImageSurface {
	s := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s.SetPixel(x, y, r, g, b, a)
		}
	}
	return s
}

func TestScaleSurfaceIdentity(t *testing.T) {
	s := solidSurface(4, 4, 1, 2, 3, 255)
	if got := ScaleSurface(s, 4, 4); got != s {
		t.Error("scaling to the same size must not copy")
	}
	if ScaleSurface(s, 0, 4) != nil || ScaleSurface(nil, 2, 2) != nil {
		t.Error("invalid arguments must return nil")
	}
}

func TestScaleSurfaceDownscaleAverages(t *testing.T) {
	s := NewImageSurface(2, 2)
	s.SetPixel(0, 0, 0, 0, 0, 255)
	s.SetPixel(1, 0, 100, 100, 100, 255)
	s.SetPixel(0, 1, 200, 200, 200, 255)
	s.SetPixel(1, 1, 255, 255, 255, 255)

	out := ScaleSurface(s, 1, 1)
	if out == nil {
		t.Fatal("downscale returned nil")
	}
	r, _, _, a := out.PixelAt(0, 0)
	if a != 255 {
		t.Errorf("alpha must survive, got %d", a)
	}
	if r < 138 || r > 140 {
		t.Errorf("downscale must average the block, got %d, want about 139", r)
	}
}

func TestScaleSurfaceUpscaleInterpolates(t *testing.T) {
	s := NewImageSurface(2, 1)
	s.SetPixel(0, 0, 0, 0, 0, 255)
	s.SetPixel(1, 0, 255, 255, 255, 255)

	out := ScaleSurface(s, 4, 1)
	if out == nil {
		t.Fatal("upscale returned nil")
	}
	var vals [4]byte
	for i := 0; i < 4; i++ {
		vals[i], _, _, _ = out.PixelAt(i, 0)
	}
	for i := 1; i < 4; i++ {
		if vals[i] < vals[i-1] {
			t.Fatalf("gradient must stay monotonic, got %v", vals)
		}
	}
	if vals[0] != 0 || vals[3] != 255 {
		t.Errorf("edges must be preserved, got %v", vals)
	}
	if vals[1] == vals[2] {
		t.Errorf("upscaling must interpolate, not repeat: %v", vals)
	}
}

func TestScaleSurfaceTransparencyDoesNotBleed(t *testing.T) {
	s := NewImageSurface(2, 1)
	s.SetPixel(0, 0, 255, 0, 0, 255)
	s.SetPixel(1, 0, 0, 255, 0, 0)

	out := ScaleSurface(s, 1, 1)
	if out == nil {
		t.Fatal("downscale returned nil")
	}
	r, g, _, a := out.PixelAt(0, 0)
	if g > 8 {
		t.Errorf("a fully transparent pixel must not contribute colour, got green %d", g)
	}
	if r < 245 {
		t.Errorf("the opaque pixel must keep its colour, got red %d", r)
	}
	if a < 120 || a > 136 {
		t.Errorf("alpha must be averaged, got %d, want about 128", a)
	}
}

func TestFitInside(t *testing.T) {
	if w, h := FitInside(200, 100, 50, 50); w != 50 || h != 25 {
		t.Errorf("wide image: got %dx%d", w, h)
	}
	if w, h := FitInside(100, 200, 50, 50); w != 25 || h != 50 {
		t.Errorf("tall image: got %dx%d", w, h)
	}
	if w, h := FitInside(10, 10, 0, 5); w != 0 || h != 0 {
		t.Errorf("degenerate box must return zeroes, got %dx%d", w, h)
	}
	if w, h := FitInside(1000, 1, 10, 10); w < 1 || h < 1 {
		t.Errorf("extreme ratios must not collapse to zero, got %dx%d", w, h)
	}
}

func TestSurfaceImageRoundTrip(t *testing.T) {
	s := solidSurface(3, 2, 200, 100, 50, 128)
	back := NewImageSurfaceFromImage(s.ToRGBA())
	if back == nil {
		t.Fatal("round trip lost the image")
	}
	r, g, b, a := back.PixelAt(1, 1)
	if a != 128 {
		t.Errorf("alpha changed: %d", a)
	}
	if r < 197 || r > 203 || g < 97 || g > 103 || b < 47 || b > 53 {
		t.Errorf("colour drifted too far: %d,%d,%d", r, g, b)
	}
}

func TestToRGBASharesOpaqueMemory(t *testing.T) {
	s := solidSurface(2, 2, 10, 20, 30, 255)
	img := s.ToRGBA()
	img.Pix[0] = 99
	if got, _, _, _ := s.PixelAt(0, 0); got != 99 {
		t.Error("an opaque surface should expose its memory instead of copying")
	}

	alpha := solidSurface(2, 2, 200, 200, 200, 128)
	premul := alpha.ToRGBA()
	if premul.Pix[0] > 110 {
		t.Errorf("the standard library expects premultiplied alpha, got %d", premul.Pix[0])
	}
}

func TestBlitSurfaceOpaqueAndClipping(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 4, 4))
	src := solidSurface(2, 2, 10, 20, 30, 255)

	rect := blitSurface(dst, src, 3, 3)
	if rect != image.Rect(3, 3, 4, 4) {
		t.Errorf("blit must clip to the framebuffer, got %v", rect)
	}
	if dst.Pix[3*dst.Stride+3*4] != 10 {
		t.Error("clipped blit did not write the visible pixel")
	}

	if r := blitSurface(dst, src, 10, 10); !r.Empty() {
		t.Error("a fully clipped blit must report an empty rectangle")
	}
	if r := blitSurface(nil, src, 0, 0); !r.Empty() {
		t.Error("a nil destination must be handled")
	}
}

func TestBlitSurfaceAlphaBlend(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i := 3; i < len(dst.Pix); i += 4 {
		dst.Pix[i] = 255
	}
	src := solidSurface(2, 2, 255, 255, 255, 128)

	blitSurface(dst, src, 0, 0)
	if v := dst.Pix[0]; v < 120 || v > 136 {
		t.Errorf("half transparent white over black should be mid grey, got %d", v)
	}
	if dst.Pix[3] != 255 {
		t.Error("the framebuffer must stay opaque")
	}

	transparent := solidSurface(2, 2, 0, 255, 0, 0)
	blitSurface(dst, transparent, 0, 0)
	if dst.Pix[1] > 136 {
		t.Error("a fully transparent source must leave the framebuffer alone")
	}
}

func TestPlacementPixelRect(t *testing.T) {
	p := ImagePlacement{Col: 2, Row: 3, Cols: 4, Rows: 5}
	x, y, w, h := placementPixelRect(&p, 8, 16)
	if x != 16 || y != 48 || w != 32 || h != 80 {
		t.Errorf("got %d,%d,%d,%d", x, y, w, h)
	}
	if _, _, w, _ := placementPixelRect(&p, 0, 16); w != 0 {
		t.Error("an unknown cell size must produce an empty rectangle")
	}
}

func TestNativeCacheReusesScaledSurfaces(t *testing.T) {
	var cache nativeGraphicsCache
	p := ImagePlacement{Surface: solidSurface(8, 8, 1, 2, 3, 255), Cols: 2, Rows: 1}

	first := cache.scaled(&p, 16, 16)
	if first == nil {
		t.Fatal("scaling failed")
	}
	if second := cache.scaled(&p, 16, 16); second != first {
		t.Error("the same request must hit the cache")
	}
	if other := cache.scaled(&p, 32, 32); other == first {
		t.Error("a different size must produce a different entry")
	}
	if first.asImage() != first.asImage() {
		t.Error("the image conversion must be cached too")
	}

	cache.reset()
	if again := cache.scaled(&p, 16, 16); again == first {
		t.Error("reset must drop the cache")
	}
}

func TestNativeCacheEviction(t *testing.T) {
	var cache nativeGraphicsCache
	for i := 0; i < nativeCacheLimit+5; i++ {
		p := ImagePlacement{Surface: solidSurface(2, 2, byte(i), 0, 0, 255), Cols: 1, Rows: 1}
		cache.scaled(&p, 4, 4)
	}
	if len(cache.entries) > nativeCacheLimit {
		t.Errorf("cache grew to %d entries", len(cache.entries))
	}
}

func TestDrawNativePlacementsRespectsOrder(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var cache nativeGraphicsCache

	list := []ImagePlacement{
		{Surface: solidSurface(4, 4, 255, 0, 0, 255), Col: 0, Row: 0, Cols: 2, Rows: 1},
		{Surface: solidSurface(4, 4, 0, 0, 255, 255), Col: 0, Row: 0, Cols: 1, Rows: 1},
	}

	rect := drawNativePlacements(dst, list, 8, 8, &cache)
	if rect != image.Rect(0, 0, 16, 8) {
		t.Errorf("union rectangle is wrong: %v", rect)
	}
	if dst.Pix[2] != 255 || dst.Pix[0] != 0 {
		t.Error("the later placement must win where they overlap")
	}
	if dst.Pix[8*4] != 255 {
		t.Error("the uncovered part of the first placement must survive")
	}
}

func TestDrawNativePlacementsSkipsUndrawable(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var cache nativeGraphicsCache
	rect := drawNativePlacements(dst, []ImagePlacement{
		{Surface: nil, Cols: 1, Rows: 1},
		{Surface: solidSurface(2, 2, 1, 1, 1, 255), Cols: 0, Rows: 1},
	}, 8, 8, &cache)
	if !rect.Empty() {
		t.Errorf("nothing drawable must touch nothing, got %v", rect)
	}
}

func TestDirtyRowsUnderIsRowWide(t *testing.T) {
	const w, h = 10, 5
	buf := make([]CharInfo, w*h)
	shadow := make([]CharInfo, w*h)

	var g GraphicsLayer
	g.Add(ImagePlacement{Surface: NewImageSurface(1, 1), Col: 2, Row: 1, Cols: 3, Rows: 2})

	if g.DirtyRowsUnder(buf, shadow, w, h) {
		t.Error("identical buffers must not be dirty")
	}

	buf[1*w+9].Char = 'x'
	if !g.DirtyRowsUnder(buf, shadow, w, h) {
		t.Error("a change anywhere in a covered row must be reported")
	}
	if g.DirtyUnder(buf, shadow, w, h) {
		t.Error("the cell precise check must still ignore it")
	}

	buf[1*w+9].Char = 0
	buf[4*w+0].Char = 'y'
	if g.DirtyRowsUnder(buf, shadow, w, h) {
		t.Error("rows outside the image must be ignored")
	}
}

func TestGraphicsLayerRepaintRequest(t *testing.T) {
	var g GraphicsLayer
	id := g.Add(ImagePlacement{Surface: NewImageSurface(2, 2), Cols: 1, Rows: 1})
	g.TakeRepaintRequest()

	if !g.Update(id, func(p *ImagePlacement) { p.Col = 3 }) {
		t.Fatal("update failed")
	}
	if !g.TakeRepaintRequest() {
		t.Error("moving an image must ask for a text repaint")
	}
	if g.TakeRepaintRequest() {
		t.Error("the request must be consumed once")
	}

	g.Remove(id)
	if !g.TakeRepaintRequest() {
		t.Error("removing an image must ask for a text repaint")
	}
}

func TestBlitSurfaceOpaqueRowCopyMatchesPerPixel(t *testing.T) {
	// A padded source, the shape a working-copy cut has: the row copy must
	// honour the stride just like the per-pixel path does.
	src := NewImageSurfaceFromPix(6, 4, 6*4+8, make([]byte, (6*4+8)*4))
	if src == nil {
		t.Fatal("padded surface failed")
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 6; x++ {
			src.SetPixel(x, y, byte(10+x*7), byte(20+y*13), byte(5+x*3+y*17), 255)
		}
	}
	src.Opaque = true

	row := image.NewRGBA(image.Rect(0, 0, 8, 8))
	alpha := image.NewRGBA(image.Rect(0, 0, 8, 8))
	opaque := *src
	nonOpaque := *src
	nonOpaque.Opaque = false
	// Partly offscreen, to send both paths through the same clipping.
	blitSurface(row, &opaque, 3, 2)
	blitSurface(alpha, &nonOpaque, 3, 2)
	if !bytes.Equal(row.Pix, alpha.Pix) {
		t.Error("the row-copy path and the per-pixel path disagree")
	}
}

// nativeGradient paints every pixel with a colour that encodes its position.
func nativeGradient(w, h int) *ImageSurface {
	s := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s.SetPixel(x, y, byte(x&255), byte(y&255), byte((x+y)&255), 255)
		}
	}
	s.Opaque = true
	return s
}

func TestNativeCacheWorkingCopyCutsInsideTheMargin(t *testing.T) {
	src := nativeGradient(400, 300)
	var cache nativeGraphicsCache

	// A zoomed view of 200x150 source pixels shown 1:1.
	p := ImagePlacement{Surface: src, SrcX: 100, SrcY: 60, SrcW: 200, SrcH: 150}
	first := cache.scaled(&p, 200, 150)
	if first == nil {
		t.Fatal("the first placement failed")
	}
	if cache.workSurf == nil {
		t.Fatal("the working copy was not created")
	}

	// A pan inside the margin must be answered by the copy, not by a
	// fresh resample, and must carry the exact source pixels (scale 1).
	workRect := cache.workRect
	p.SrcX, p.SrcY = 140, 90
	second := cache.scaled(&p, 200, 150)
	if second == nil {
		t.Fatal("the panned placement failed")
	}
	if cache.workRect != workRect {
		t.Error("a pan inside the margin must reuse the working copy")
	}
	want := src.Crop(140, 90, 200, 150)
	for y := 0; y < 150; y++ {
		for x := 0; x < 200; x++ {
			r1, g1, b1, a1 := second.surf.PixelAt(x, y)
			r2, g2, b2, a2 := want.PixelAt(x, y)
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Fatalf("cut pixel %d,%d = %d,%d,%d,%d, want %d,%d,%d,%d",
					x, y, r1, g1, b1, a1, r2, g2, b2, a2)
			}
		}
	}
}

func TestNativeCacheWorkingCopyRebuildsWhenThePanLeavesIt(t *testing.T) {
	src := nativeGradient(400, 300)
	var cache nativeGraphicsCache

	p := ImagePlacement{Surface: src, SrcX: 50, SrcY: 40, SrcW: 200, SrcH: 150}
	cache.scaled(&p, 200, 150)
	before := cache.workRect

	// A jump far past the margin has to start a fresh working copy and
	// still answer with a valid window.
	p.SrcX, p.SrcY = 160, 140
	entry := cache.scaled(&p, 200, 150)
	if entry == nil || entry.surf.Width != 200 || entry.surf.Height != 150 {
		t.Fatalf("the panned placement failed: %+v", entry)
	}
	if cache.workRect == before {
		t.Error("a pan outside the margin must rebuild the working copy")
	}
}

func TestNativeCacheWorkingCopySkipsExtremeZoom(t *testing.T) {
	src := nativeGradient(800, 600)
	var cache nativeGraphicsCache

	// At 8x zoom a margin wide enough for a pan step would make the copy
	// several times larger than the window, so the exact path must answer.
	p := ImagePlacement{Surface: src, SrcX: 100, SrcY: 50, SrcW: 100, SrcH: 75}
	entry := cache.scaled(&p, 200, 150)
	if entry == nil {
		t.Fatal("the placement failed")
	}
	if cache.workSurf != nil {
		t.Error("extreme zoom must not build a working copy")
	}
	want := ScaleSurface(src.Crop(100, 50, 100, 75), 200, 150)
	for y := 0; y < 150; y++ {
		for x := 0; x < 200; x++ {
			r1, g1, b1, a1 := entry.surf.PixelAt(x, y)
			r2, g2, b2, a2 := want.PixelAt(x, y)
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Fatalf("exact path pixel %d,%d = %d,%d,%d,%d, want %d,%d,%d,%d",
					x, y, r1, g1, b1, a1, r2, g2, b2, a2)
			}
		}
	}
}
