//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"image"
	"image/color"
)

// drawBoxGlyph rasterises the box-drawing, arrow and block characters
// geometrically instead of going through the font.
//
// Fonts are the wrong tool for these: a monospace face renders U+2500 and its
// neighbours as glyphs positioned by metrics, so adjacent cells leave hairline
// gaps at the joins and a frame comes out dotted. Drawing the segments
// directly against the cell rectangle makes them meet exactly, which is what a
// Far Manager style UI is made of.
//
// thick is the line width in pixels, normally the display scale factor, so
// frames stay proportionate on a HiDPI screen. Bounds come from img itself,
// so the caller does not have to pass a surface size that could disagree with
// the buffer. Returns false for characters it does not handle, leaving the
// caller to fall back to the font.
func drawBoxGlyph(img *image.RGBA, char rune, px, py, cw, ch, thick int, col color.Color) bool {
	mx, my := px+cw/2, py+ch/2
	if thick < 1 {
		thick = 1
	}
	cr, cg, cb, _ := col.RGBA()
	r8, g8, b8 := uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)

	drawHLine := func(x1, x2, y int) {
		for x := x1; x <= x2; x++ {
			if x < 0 || x >= img.Rect.Max.X {
				continue
			}
			for t := 0; t < thick; t++ {
				if y+t < 0 || y+t >= img.Rect.Max.Y {
					continue
				}
				off := (y+t)*img.Stride + x*4
				if off+3 < len(img.Pix) {
					img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
				}
			}
		}
	}
	drawVLine := func(x, y1, y2 int) {
		for y := y1; y <= y2; y++ {
			if y < 0 || y >= img.Rect.Max.Y {
				continue
			}
			for t := 0; t < thick; t++ {
				if x+t < 0 || x+t >= img.Rect.Max.X {
					continue
				}
				off := y*img.Stride + (x+t)*4
				if off+3 < len(img.Pix) {
					img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
				}
			}
		}
	}

	ofs := cw / 4
	if ofs < 1 {
		ofs = 1
	}

	switch char {
	case '─':
		drawHLine(px, px+cw-1, my)
		return true
	case '│':
		drawVLine(mx, py, py+ch-1)
		return true
	case '┌':
		drawHLine(mx, px+cw-1, my)
		drawVLine(mx, my, py+ch-1)
		return true
	case '┐':
		drawHLine(px, mx, my)
		drawVLine(mx, my, py+ch-1)
		return true
	case '└':
		drawHLine(mx, px+cw-1, my)
		drawVLine(mx, py, my)
		return true
	case '┘':
		drawHLine(px, mx, my)
		drawVLine(mx, py, my)
		return true
	case '├':
		drawHLine(mx, px+cw-1, my)
		drawVLine(mx, py, py+ch-1)
		return true
	case '┤':
		drawHLine(px, mx, my)
		drawVLine(mx, py, py+ch-1)
		return true
	case '┬':
		drawHLine(px, px+cw-1, my)
		drawVLine(mx, my, py+ch-1)
		return true
	case '┴':
		drawHLine(px, px+cw-1, my)
		drawVLine(mx, py, my)
		return true
	case '┼':
		drawHLine(px, px+cw-1, my)
		drawVLine(mx, py, py+ch-1)
		return true
	case '═':
		drawHLine(px, px+cw-1, my-ofs)
		drawHLine(px, px+cw-1, my+ofs)
		return true
	case '║':
		drawVLine(mx-ofs, py, py+ch-1)
		drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╔':
		drawHLine(mx+ofs, px+cw-1, my-ofs)
		drawHLine(mx-ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, my+ofs, py+ch-1)
		drawVLine(mx+ofs, my-ofs, py+ch-1)
		return true
	case '╗':
		drawHLine(px, mx-ofs, my-ofs)
		drawHLine(px, mx+ofs, my+ofs)
		drawVLine(mx+ofs, my+ofs, py+ch-1)
		drawVLine(mx-ofs, my-ofs, py+ch-1)
		return true
	case '╚':
		drawHLine(mx-ofs, px+cw-1, my-ofs)
		drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, py, my-ofs)
		drawVLine(mx+ofs, py, my+ofs)
		return true
	case '╝':
		drawHLine(px, mx+ofs, my-ofs)
		drawHLine(px, mx-ofs, my+ofs)
		drawVLine(mx+ofs, py, my-ofs)
		drawVLine(mx-ofs, py, my+ofs)
		return true
	case '╠':
		drawHLine(mx-ofs, px+cw-1, my-ofs)
		drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, py, py+ch-1)
		drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╣':
		drawHLine(px, mx+ofs, my-ofs)
		drawHLine(px, mx-ofs, my+ofs)
		drawVLine(mx-ofs, py, py+ch-1)
		drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╟':
		drawHLine(mx+ofs, px+cw-1, my)
		drawVLine(mx-ofs, py, py+ch-1)
		drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╢':
		drawHLine(px, mx-ofs, my)
		drawVLine(mx-ofs, py, py+ch-1)
		drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╩':
		drawHLine(px, px+cw-1, my+ofs)
		drawHLine(px, mx-ofs, my-ofs)
		drawHLine(mx+ofs, px+cw-1, my-ofs)
		drawVLine(mx-ofs, py, my-ofs)
		drawVLine(mx+ofs, py, my-ofs)
		return true
	case '╦':
		drawHLine(px, px+cw-1, my-ofs)
		drawHLine(px, mx-ofs, my+ofs)
		drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, my+ofs, py+ch-1)
		drawVLine(mx+ofs, my+ofs, py+ch-1)
		return true

	// Arrows and Triangles
	case '↑':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawVLine(mx, yTop, yBot)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawHLine(mx-i, mx+i, yTop+i)
		}
		return true
	case '↓':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawVLine(mx, yTop, yBot)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawHLine(mx-(arm-i), mx+(arm-i), yBot-arm+i)
		}
		return true
	case '↕':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawVLine(mx, yTop, yBot)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawHLine(mx-i, mx+i, yTop+i)
		}
		for i := 0; i <= arm; i++ {
			drawHLine(mx-(arm-i), mx+(arm-i), yBot-arm+i)
		}
		return true
	case '▲':
		yTop := py + ch/5
		yBot := py + ch - ch/5
		hSpan := yBot - yTop
		maxW := cw / 3
		if maxW < 2 {
			maxW = 2
		}
		for y := yTop; y <= yBot; y++ {
			dx := (y - yTop) * maxW / max(hSpan, 1)
			drawHLine(mx-dx, mx+dx, y)
		}
		return true
	case '▼':
		yTop := py + ch/5
		yBot := py + ch - ch/5
		hSpan := yBot - yTop
		maxW := cw / 3
		if maxW < 2 {
			maxW = 2
		}
		for y := yTop; y <= yBot; y++ {
			dx := (yBot - y) * maxW / max(hSpan, 1)
			drawHLine(mx-dx, mx+dx, y)
		}
		return true
	case '█':
		baseOff := py*img.Stride + px*4
		maxBytes := cw * 4
		if baseOff+maxBytes <= len(img.Pix) {
			img.Pix[baseOff], img.Pix[baseOff+1], img.Pix[baseOff+2], img.Pix[baseOff+3] = r8, g8, b8, 255
			for n := 4; n < maxBytes; n *= 2 {
				copy(img.Pix[baseOff+n:baseOff+maxBytes], img.Pix[baseOff:baseOff+n])
			}
			for y := 1; y < ch; y++ {
				lineOff := (py+y)*img.Stride + px*4
				if lineOff+maxBytes <= len(img.Pix) {
					copy(img.Pix[lineOff:lineOff+maxBytes], img.Pix[baseOff:baseOff+maxBytes])
				}
			}
		}
		return true
	}
	return false
}

// isBoxDrawRune reports whether a rune is worth offering to drawBoxGlyph.
//
// It is a cheap range test in front of a thirty-way switch, and the switch
// would otherwise run for every letter on the screen. The ranges are the box
// drawing and block elements blocks plus the arrows drawBoxGlyph handles;
// being generous costs only a switch that returns false, whereas being too
// narrow would silently send a frame character back to the font.
func isBoxDrawRune(r rune) bool {
	return (r >= 0x2500 && r <= 0x259F) || (r >= 0x2190 && r <= 0x2195) ||
		r == 0x25B2 || r == 0x25BC
}

// rgbColor turns the packed 0xRRGGBB used throughout the buffer into a color.
func rgbColor(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}
