package vtui

import (
	"image"
	"image/color"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// gridRaster composes the cell grid into an RGBA image on the CPU: cell
// backgrounds, glyphs from a golang.org/x/image font face, the geometric box
// drawing and checkbox shapes, underlines, the software caret and native
// graphics placements. It knows nothing about windows; a backend embeds it
// and hands imgBuf to its platform the way that platform wants it.
//
// It was the platform-neutral half of Win32GuiRenderer, which puts the frame
// on screen with GDI, and the Cocoa renderer, which gives it to Core
// Animation, now shares it instead of carrying a second copy. Like the file
// it came from it carries no build tag of its own, so its tests run
// everywhere.
type gridRaster struct {
	mu           sync.Mutex
	face         font.Face
	cellW, cellH int
	cols, rows   int
	scale        int

	// imgBuf is exactly cols*cellW by rows*cellH pixels. What lies outside
	// it in the window -- the partial column and row of a window that is
	// not a whole number of cells -- is the backend's to clear.
	imgBuf *image.RGBA
	// dirty says imgBuf changed since the backend last took it.
	dirty bool

	glyphCache map[glyphKey]*image.RGBA

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	paintedCursor  bool
	paintedCursorY int

	blinkState    bool
	lastBlinkTime time.Time
}

func newGridRaster(face font.Face, cellW, cellH, scale int) gridRaster {
	if scale < 1 {
		scale = 1
	}
	return gridRaster{
		face:          face,
		cellW:         cellW,
		cellH:         cellH,
		scale:         scale,
		glyphCache:    make(map[glyphKey]*image.RGBA),
		blinkState:    true,
		lastBlinkTime: time.Now(),
	}
}

func (r *gridRaster) SetPalette(pal *[256]uint32) {}

func (r *gridRaster) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.cursorX, r.cursorY = x, y
		r.cursorVis = visible
		r.cursorShape = shape
		r.blinkState = true
		r.lastBlinkTime = time.Now()
		r.dirty = true
	}
}

func (r *gridRaster) getCellColors(cell CharInfo) (uint32, uint32) {
	bg := GetRGBBack(cell.Attributes)
	if cell.Attributes&IsBgRGB == 0 {
		bg = ThemePalette[GetIndexBack(cell.Attributes)]
	}
	fg := GetRGBFore(cell.Attributes)
	if cell.Attributes&IsFgRGB == 0 {
		fg = ThemePalette[GetIndexFore(cell.Attributes)]
	}
	return fg, bg
}

func (r *gridRaster) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	if w <= 0 || h <= 0 || len(buf) < w*h || len(shadow) < w*h {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	stepSoftwareBlink(&r.blinkState, &r.lastBlinkTime, time.Now())
	cursorVisible := r.cursorVis && r.blinkState

	pixW, pixH := w*r.cellW, h*r.cellH
	if r.imgBuf == nil || r.imgBuf.Rect.Dx() != pixW || r.imgBuf.Rect.Dy() != pixH {
		r.imgBuf = image.NewRGBA(image.Rect(0, 0, pixW, pixH))
		forceRedraw = true
		// A brand-new bitmap always has to reach the window, even if the
		// row loop below finds nothing to draw (an all-blank screen at
		// startup compares equal to a blank shadow buffer).
		r.dirty = true
	}
	r.cols, r.rows = w, h
	img := r.imgBuf

	for y := 0; y < h; y++ {
		rowOff := y * w
		rowDirty := forceRedraw ||
			(cursorVisible && y == r.cursorY) ||
			(r.paintedCursor && y == r.paintedCursorY)
		if !rowDirty {
			for x := 0; x < w; x++ {
				if buf[rowOff+x] != shadow[rowOff+x] {
					rowDirty = true
					break
				}
			}
		}
		if !rowDirty {
			continue
		}
		r.dirty = true

		for x := 0; x < w; {
			cell := buf[rowOff+x]
			_, bg := r.getCellColors(cell)

			spanW := 0
			for x+spanW < w {
				next := buf[rowOff+x+spanW]
				if next.Char == WideCharFiller {
					spanW++
					continue
				}
				if _, nextBg := r.getCellColors(next); nextBg != bg {
					break
				}
				spanW++
			}

			r.fillRect(img, x*r.cellW, y*r.cellH, spanW*r.cellW, r.cellH, bg)

			for sx := 0; sx < spanW; {
				currX := x + sx
				curr := buf[rowOff+currX]
				if curr.Char == WideCharFiller {
					sx++
					continue
				}

				if IsSymChar(curr.Char) && sx+2 < spanW && currX+2 < w {
					if sym, symOk := symGlyphAt(curr.Char, buf[rowOff+currX+1].Char, buf[rowOff+currX+2].Char); symOk {
						symPx, symPy := currX*r.cellW, y*r.cellH
						symFg, _ := r.getCellColors(curr)
						if drawSymGlyphRaster(img, sym, symPx, symPy, r.cellW*3, r.cellH, r.scale, symFg) {
							if curr.Attributes&CommonLvbUnderscore != 0 {
								drawUnderline(img, symPx, symPy, r.cellW*3, r.cellH, r.scale, symFg)
							}
							for k := 0; k < 3; k++ {
								colX := currX + k
								if cursorVisible && y == r.cursorY && r.cursorX == colX {
									r.invertCursor(img, colX*r.cellW, y*r.cellH, 1)
								}
							}
							sx += 3
							continue
						}
					}
				}

				_, rw := CellSpanAt(buf, w, currX, y)
				if rw < 1 {
					rw = 1
				}
				fg, cbg := r.getCellColors(curr)
				px, py := currX*r.cellW, y*r.cellH

				if ch := CellBaseRune(curr.Char); ch != 0 && ch != ' ' {
					if !isBoxDrawRune(ch) ||
						!drawBoxGlyph(img, ch, px, py, r.cellW*rw, r.cellH, r.scale, fg) {
						r.drawCachedGlyph(img, curr.Char, px, py, rw, fg, cbg)
					}
				}
				// Neither GDI nor Core Animation ever sees the cell
				// attributes, so the underline of a hovered URL (f4 #459)
				// is painted here like in the other pixel backends.
				if curr.Attributes&CommonLvbUnderscore != 0 {
					drawUnderline(img, px, py, r.cellW*rw, r.cellH, r.scale, fg)
				}

				if cursorVisible && y == r.cursorY && r.cursorX >= currX && r.cursorX < currX+rw {
					r.invertCursor(img, currX*r.cellW, y*r.cellH, rw)
				}
				sx += rw
			}
			x += spanW
		}
	}

	r.paintedCursor = cursorVisible
	r.paintedCursorY = r.cursorY
}

func (r *gridRaster) fillRect(img *image.RGBA, px, py, w, h int, rgb uint32) {
	if w <= 0 || h <= 0 {
		return
	}
	if px+w > img.Rect.Dx() {
		w = img.Rect.Dx() - px
	}
	if w <= 0 {
		return
	}
	cr, cg, cb := uint8(rgb>>16), uint8(rgb>>8), uint8(rgb) //nolint:gosec // G115: the byte channels of a 24-bit colour

	base := py*img.Stride + px*4
	rowBytes := w * 4
	if base < 0 || base+rowBytes > len(img.Pix) {
		return
	}
	img.Pix[base], img.Pix[base+1], img.Pix[base+2], img.Pix[base+3] = cr, cg, cb, 255
	for n := 4; n < rowBytes; n *= 2 {
		copy(img.Pix[base+n:base+rowBytes], img.Pix[base:base+n])
	}
	for iy := 1; iy < h; iy++ {
		if py+iy >= img.Rect.Dy() {
			break
		}
		off := (py+iy)*img.Stride + px*4
		if off+rowBytes <= len(img.Pix) {
			copy(img.Pix[off:off+rowBytes], img.Pix[base:base+rowBytes])
		}
	}
}

func (r *gridRaster) drawCachedGlyph(img *image.RGBA, cellVal uint64, px, py, rw int, fg, bg uint32) {
	key := glyphKey{ch: cellVal, fg: fg, bg: bg, w: rw}
	cached, ok := r.glyphCache[key]
	drawW := r.cellW * rw

	if !ok {
		cached = image.NewRGBA(image.Rect(0, 0, drawW, r.cellH))
		fgCol := color.RGBA{R: uint8(fg >> 16), G: uint8(fg >> 8), B: uint8(fg), A: 255} //nolint:gosec // G115: the byte channels of a 24-bit colour
		bgCol := color.RGBA{R: uint8(bg >> 16), G: uint8(bg >> 8), B: uint8(bg), A: 255} //nolint:gosec // G115: as above

		for i := 0; i+3 < len(cached.Pix); i += 4 {
			cached.Pix[i], cached.Pix[i+1], cached.Pix[i+2], cached.Pix[i+3] = bgCol.R, bgCol.G, bgCol.B, 255
		}

		if r.face != nil {
			d := &font.Drawer{
				Dst:  cached,
				Src:  image.NewUniform(fgCol),
				Face: r.face,
				Dot:  fixed.Point26_6{X: 0, Y: r.face.Metrics().Ascent},
			}
			d.DrawString(CellString(cellVal))
		}
		r.glyphCache[key] = cached
	}

	rowBytes := drawW * 4
	for iy := 0; iy < r.cellH; iy++ {
		if py+iy >= img.Rect.Dy() {
			break
		}
		dst := (py+iy)*img.Stride + px*4
		src := iy * cached.Stride
		if dst+rowBytes <= len(img.Pix) && src+rowBytes <= len(cached.Pix) {
			copy(img.Pix[dst:dst+rowBytes], cached.Pix[src:src+rowBytes])
		}
	}
}

// invertCursor inverts the caret's part of the cell under it, with the
// geometry every pixel renderer shares (see cursorCellRect), so the caret
// stays visible whatever colours the cell happens to carry.
func (r *gridRaster) invertCursor(img *image.RGBA, px, py, rw int) {
	invertCursorRect(img.Pix, img.Stride, img.Rect.Dx(), img.Rect.Dy(), px, py, r.cursorShape, r.cellW*rw, r.cellH, r.scale > 1)
}

func (r *gridRaster) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	gen := layer.Generation()
	if !force && r.gfxKnown && gen == r.gfxGen && !layer.DirtyRowsUnder(buf, shadow, w, h) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.imgBuf == nil || r.cellW <= 0 || r.cellH <= 0 {
		return
	}
	r.gfxGen = gen
	r.gfxKnown = true

	r.gfxList, _ = layer.Snapshot(r.gfxList)
	drawNativePlacements(r.imgBuf, r.gfxList, r.cellW, r.cellH, &r.gfxCache)
	r.dirty = true
}
