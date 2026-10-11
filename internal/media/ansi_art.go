package media

// ANSI half-block art (docs/VIDEO.md, V2): a picture drawn in coloured text.
// Every cell is an upper half block ('▀') whose foreground is the pixel above
// and whose background is the pixel below, so a cell carries two pixels and a
// cols x rows area shows a cols x 2*rows picture. It works on any colour
// terminal - ssh into a plain xterm included - which is what makes it the rung
// of the ladder below terminal graphics: still pictures and video frames get
// an answer where no image protocol is on offer.

import (
	"image"

	"github.com/unxed/vtui"
)

// halfBlock is the character of every cell of the art.
const halfBlock = '▀'

// HalfBlockArt draws img into cols x rows cells, row by row. The picture keeps
// its proportions (a cell is taken to be twice as tall as wide, so its two
// pixels are square), is centred and the rest of the area is black.
func HalfBlockArt(img image.Image, cols, rows int) []vtui.CharInfo {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	cells := make([]vtui.CharInfo, cols*rows)
	pixels := scaleToPixels(img, cols, rows*2)
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			top := pixels[(2*row)*cols+col]
			bottom := pixels[(2*row+1)*cols+col]
			cells[row*cols+col] = vtui.CharInfo{
				Char:       halfBlock,
				Attributes: vtui.SetRGBBoth(0, top, bottom),
			}
		}
	}
	return cells
}

// scaleToPixels fits img into w x h pixels, keeping the proportions, by
// averaging the source pixels each destination pixel covers. The result is 0xRRGGBB
// per pixel, row by row; what the picture does not cover is black, and
// transparency is laid over black.
func scaleToPixels(img image.Image, w, h int) []uint32 {
	out := make([]uint32, w*h)
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return out
	}
	// The scale that makes the whole picture fit.
	fw, fh := w, sh*w/sw
	if fh > h {
		fw, fh = sw*h/sh, h
	}
	fw, fh = max(fw, 1), max(fh, 1)
	x0, y0 := (w-fw)/2, (h-fh)/2
	for y := 0; y < fh; y++ {
		sy0, sy1 := b.Min.Y+y*sh/fh, b.Min.Y+(y+1)*sh/fh
		sy1 = max(sy1, sy0+1)
		for x := 0; x < fw; x++ {
			sx0, sx1 := b.Min.X+x*sw/fw, b.Min.X+(x+1)*sw/fw
			sx1 = max(sx1, sx0+1)
			out[(y0+y)*w+x0+x] = averageOverBlack(img, sx0, sy0, sx1, sy1)
		}
	}
	return out
}

// averageOverBlack is the mean colour of the source rectangle, alpha
// composited on black, as 0xRRGGBB.
func averageOverBlack(img image.Image, x0, y0, x1, y1 int) uint32 {
	var r, g, b, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			pr, pg, pb, _ := img.At(x, y).RGBA() // premultiplied: already over black
			r, g, b, n = r+uint64(pr>>8), g+uint64(pg>>8), b+uint64(pb>>8), n+1
		}
	}
	if n == 0 {
		return 0
	}
	return uint32(r/n)<<16 | uint32(g/n)<<8 | uint32(b/n) //nolint:gosec // each mean is at most 255
}
