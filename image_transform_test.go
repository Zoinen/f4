package main

import (
	"bytes"
	"testing"

	"github.com/unxed/vtui"
)

// gradientSurface paints every pixel with a colour that encodes its own
// position, so that a transformed copy can be checked pixel by pixel.
func gradientSurface(w, h int) *vtui.ImageSurface {
	s := vtui.NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s.SetPixel(x, y, byte(x+1), byte(y+1), byte(x*h+y+1), 255)
		}
	}
	return s
}

// samePixel reports whether two surfaces carry the same colour at the two
// given points.
func samePixel(src *vtui.ImageSurface, sx, sy int, dst *vtui.ImageSurface, dx, dy int) bool {
	r1, g1, b1, a1 := src.PixelAt(sx, sy)
	r2, g2, b2, a2 := dst.PixelAt(dx, dy)
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func TestRotateSurfaceQuarterTurn(t *testing.T) {
	src := gradientSurface(4, 2)
	dst := RotateSurface(src, 90)

	if dst.Width != 2 || dst.Height != 4 {
		t.Fatalf("a quarter turn must swap the sides, got %dx%d", dst.Width, dst.Height)
	}
	// Clockwise, so the top left corner ends up in the top right one.
	if !samePixel(src, 0, 0, dst, 1, 0) {
		t.Error("the top left corner did not travel to the top right one")
	}
	if !samePixel(src, 3, 0, dst, 1, 3) {
		t.Error("the top right corner did not travel to the bottom right one")
	}
	if !samePixel(src, 0, 1, dst, 0, 0) {
		t.Error("the bottom left corner did not travel to the top left one")
	}
}

func TestRotateSurfaceFullTurn(t *testing.T) {
	src := gradientSurface(5, 3)

	out := src
	for i := 0; i < 4; i++ {
		out = RotateSurface(out, 90)
	}
	if out.Width != src.Width || out.Height != src.Height {
		t.Fatalf("four quarter turns gave %dx%d", out.Width, out.Height)
	}
	if out.Hash() != src.Hash() {
		t.Error("four quarter turns must bring the picture back")
	}
	if RotateSurface(src, 360).Hash() != src.Hash() {
		t.Error("a full turn must change nothing")
	}
	if RotateSurface(src, -90).Hash() != RotateSurface(src, 270).Hash() {
		t.Error("a turn back is three turns forward")
	}
}

func TestFlipSurfaceMirrors(t *testing.T) {
	src := gradientSurface(4, 3)

	h := FlipSurface(src, true, false)
	if h.Width != 4 || h.Height != 3 {
		t.Fatalf("mirroring must keep the size, got %dx%d", h.Width, h.Height)
	}
	if !samePixel(src, 0, 0, h, 3, 0) || !samePixel(src, 3, 2, h, 0, 2) {
		t.Error("the horizontal mirror did not swap the columns")
	}

	v := FlipSurface(src, false, true)
	if !samePixel(src, 0, 0, v, 0, 2) || !samePixel(src, 3, 2, v, 3, 0) {
		t.Error("the vertical mirror did not swap the rows")
	}

	if FlipSurface(src, true, true).Hash() != RotateSurface(src, 180).Hash() {
		t.Error("mirroring both axes is a half turn")
	}
}

func TestTransformSurfaceTurnsThenMirrors(t *testing.T) {
	src := gradientSurface(4, 2)

	want := FlipSurface(RotateSurface(src, 90), true, false)
	got := TransformSurface(src, 90, true, false)
	if got.Width != want.Width || got.Height != want.Height || got.Hash() != want.Hash() {
		t.Error("the mirror has to be applied to the already turned picture")
	}

	// The two orders differ, which is what makes the check above worth
	// making at all.
	other := RotateSurface(FlipSurface(src, true, false), 90)
	if other.Hash() == got.Hash() {
		t.Fatal("this test cannot tell the two orders apart")
	}
}

func TestTransformLeavesTheSourceAlone(t *testing.T) {
	src := gradientSurface(3, 3)
	kept := append([]byte(nil), src.Pix...)

	RotateSurface(src, 90)
	FlipSurface(src, true, false)
	TransformSurface(src, 180, true, true)

	if !bytes.Equal(src.Pix, kept) {
		t.Error("a transform must leave the decoded picture untouched")
	}
}
