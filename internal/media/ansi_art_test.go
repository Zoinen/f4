package media

import (
	"image"
	"image/color"
	"testing"

	"github.com/unxed/vtui"
)

func solidImage(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestHalfBlockArtCarriesTwoPixelsPerCell(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 2; x++ {
			if y < 2 {
				img.SetRGBA(x, y, color.RGBA{0, 0, 255, 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{0, 255, 0, 255})
			}
		}
	}
	cells := HalfBlockArt(img, 2, 2)
	if len(cells) != 4 {
		t.Fatalf("%d cells, want 4", len(cells))
	}
	// A 2x4 picture fills 2x2 cells exactly: its top half is blue, the bottom
	// green, and each cell shows one pixel above and one below.
	if c := cells[0]; c.Char != halfBlock || vtui.GetRGBFore(c.Attributes) != 0x0000FF || vtui.GetRGBBack(c.Attributes) != 0x0000FF {
		t.Errorf("top cell = %#x fore %06x back %06x", c.Char, vtui.GetRGBFore(c.Attributes), vtui.GetRGBBack(c.Attributes))
	}
	if c := cells[2]; vtui.GetRGBFore(c.Attributes) != 0x00FF00 || vtui.GetRGBBack(c.Attributes) != 0x00FF00 {
		t.Errorf("bottom cell fore %06x back %06x, want green", vtui.GetRGBFore(c.Attributes), vtui.GetRGBBack(c.Attributes))
	}

	split := image.NewRGBA(image.Rect(0, 0, 1, 2))
	split.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	split.SetRGBA(0, 1, color.RGBA{0, 0, 255, 255})
	one := HalfBlockArt(split, 1, 1)
	if len(one) != 1 || vtui.GetRGBFore(one[0].Attributes) != 0xFF0000 || vtui.GetRGBBack(one[0].Attributes) != 0x0000FF {
		t.Fatalf("one cell over a red and a blue pixel: %+v", one)
	}
}

func TestHalfBlockArtLetterboxesAndAveragesAndHandlesEmpty(t *testing.T) {
	// A wide picture in a tall area: black above and below.
	cells := HalfBlockArt(solidImage(8, 2, color.RGBA{255, 255, 255, 255}), 4, 4)
	if fg := vtui.GetRGBFore(cells[0].Attributes); fg != 0 {
		t.Errorf("the top row of a letterboxed picture is %06x, want black", fg)
	}
	mid := cells[1*4+1]
	if vtui.GetRGBFore(mid.Attributes) != 0xFFFFFF && vtui.GetRGBBack(mid.Attributes) != 0xFFFFFF {
		t.Errorf("the picture itself is missing from the middle: %+v", mid)
	}

	// Two source pixels averaged into one.
	pair := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pair.SetRGBA(0, 0, color.RGBA{200, 0, 0, 255})
	pair.SetRGBA(1, 0, color.RGBA{100, 0, 0, 255})
	pair.SetRGBA(0, 1, color.RGBA{200, 0, 0, 255})
	pair.SetRGBA(1, 1, color.RGBA{100, 0, 0, 255})
	one := HalfBlockArt(pair, 1, 1)
	if got := vtui.GetRGBFore(one[0].Attributes); got != 150<<16 {
		t.Errorf("averaged pixel = %06x, want 960000", got)
	}

	// Transparency is laid over black.
	glass := solidImage(2, 2, color.RGBA{0, 0, 0, 0})
	if c := HalfBlockArt(glass, 1, 1)[0]; vtui.GetRGBFore(c.Attributes) != 0 {
		t.Errorf("transparent pixel = %06x", vtui.GetRGBFore(c.Attributes))
	}

	if HalfBlockArt(solidImage(2, 2, color.RGBA{}), 0, 3) != nil || HalfBlockArt(solidImage(2, 2, color.RGBA{}), 3, 0) != nil {
		t.Error("an empty area produced cells")
	}
	if got := HalfBlockArt(image.NewRGBA(image.Rect(0, 0, 0, 0)), 2, 2); len(got) != 4 {
		t.Errorf("an empty picture gave %d cells, want a black area of 4", len(got))
	}
}
