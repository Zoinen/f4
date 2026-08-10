package vtui

import "image"

// nativeCacheLimit bounds the number of resampled images the GUI backends
// keep around. A viewer shows one picture at a time, a gallery a screenful.
const nativeCacheLimit = 64

type nativeCacheEntry struct {
	surf *ImageSurface
	rgba *image.RGBA
}

// nativeGraphicsCache keeps surfaces already resampled to their on screen
// size, so a still picture costs one blit per frame instead of a full
// resampling pass.
type nativeGraphicsCache struct {
	entries map[uint64]*nativeCacheEntry
	order   []uint64
}

func (c *nativeGraphicsCache) reset() {
	c.entries = nil
	c.order = c.order[:0]
}

func nativeCacheKey(hash uint64, values ...int) uint64 {
	key := hash
	for _, v := range values {
		key ^= uint64(uint32(v))
		key *= fnvPrime64
	}
	return key
}

// scaled returns the placement content resampled to w x h device pixels.
func (c *nativeGraphicsCache) scaled(p *ImagePlacement, w, h int) *nativeCacheEntry {
	if p == nil || !p.Surface.Valid() || w <= 0 || h <= 0 {
		return nil
	}
	sx, sy, sw, sh := p.Source()
	if sw <= 0 || sh <= 0 {
		return nil
	}

	key := nativeCacheKey(p.Surface.Hash(), sx, sy, sw, sh, w, h)
	if entry, ok := c.entries[key]; ok {
		return entry
	}

	src := p.Surface
	if sx != 0 || sy != 0 || sw != src.Width || sh != src.Height {
		src = src.Crop(sx, sy, sw, sh)
	}
	out := ScaleSurface(src, w, h)
	if out == nil {
		return nil
	}

	entry := &nativeCacheEntry{surf: out}
	if c.entries == nil {
		c.entries = make(map[uint64]*nativeCacheEntry)
	}
	c.entries[key] = entry
	c.order = append(c.order, key)
	for len(c.order) > nativeCacheLimit {
		delete(c.entries, c.order[0])
		c.order = c.order[1:]
	}
	return entry
}

// asImage returns the cached entry as a standard Go image, converting once.
func (e *nativeCacheEntry) asImage() *image.RGBA {
	if e == nil {
		return nil
	}
	if e.rgba == nil {
		e.rgba = e.surf.ToRGBA()
	}
	return e.rgba
}

// placementPixelRect converts a cell rectangle into device pixels.
func placementPixelRect(p *ImagePlacement, cellW, cellH int) (x, y, w, h int) {
	if p == nil || cellW <= 0 || cellH <= 0 {
		return 0, 0, 0, 0
	}
	return p.Col * cellW, p.Row * cellH, p.Cols * cellW, p.Rows * cellH
}

// blitSurface composites src over dst at the given device pixel origin and
// returns the rectangle that was actually touched.
func blitSurface(dst *image.RGBA, src *ImageSurface, px, py int) image.Rectangle {
	if dst == nil || !src.Valid() {
		return image.Rectangle{}
	}
	b := dst.Bounds()
	x0, y0 := px, py
	x1, y1 := px+src.Width, py+src.Height
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x0 >= x1 || y0 >= y1 {
		return image.Rectangle{}
	}

	for y := y0; y < y1; y++ {
		srcOff := (y-py)*src.Stride + (x0-px)*4
		dstOff := (y-b.Min.Y)*dst.Stride + (x0-b.Min.X)*4
		for x := x0; x < x1; x++ {
			a := uint32(src.Pix[srcOff+3])
			switch a {
			case 0:
			case 255:
				dst.Pix[dstOff] = src.Pix[srcOff]
				dst.Pix[dstOff+1] = src.Pix[srcOff+1]
				dst.Pix[dstOff+2] = src.Pix[srcOff+2]
				dst.Pix[dstOff+3] = 255
			default:
				inv := 255 - a
				dst.Pix[dstOff] = byte((uint32(src.Pix[srcOff])*a + uint32(dst.Pix[dstOff])*inv) / 255)
				dst.Pix[dstOff+1] = byte((uint32(src.Pix[srcOff+1])*a + uint32(dst.Pix[dstOff+1])*inv) / 255)
				dst.Pix[dstOff+2] = byte((uint32(src.Pix[srcOff+2])*a + uint32(dst.Pix[dstOff+2])*inv) / 255)
				dst.Pix[dstOff+3] = 255
			}
			srcOff += 4
			dstOff += 4
		}
	}
	return image.Rect(x0, y0, x1, y1)
}

// drawNativePlacements composites a whole snapshot into an RGBA framebuffer
// and returns the union of the touched rectangles.
func drawNativePlacements(dst *image.RGBA, list []ImagePlacement, cellW, cellH int, cache *nativeGraphicsCache) image.Rectangle {
	var union image.Rectangle
	if dst == nil || cache == nil {
		return union
	}
	for i := range list {
		p := &list[i]
		px, py, pw, ph := placementPixelRect(p, cellW, cellH)
		if pw <= 0 || ph <= 0 {
			continue
		}
		entry := cache.scaled(p, pw, ph)
		if entry == nil {
			continue
		}
		union = union.Union(blitSurface(dst, entry.surf, px, py))
	}
	return union
}

// imageDrawer is satisfied by drawing contexts that can stamp a bitmap. It is
// used as a runtime probe so that a backend whose canvas cannot draw images
// still builds and simply shows no pictures.
type imageDrawer interface {
	DrawImage(img image.Image, x, y int)
}

var noImageDrawerWarned bool

func warnNoImageDrawer() {
	if noImageDrawerWarned {
		return
	}
	noImageDrawerWarned = true
	DebugLog("GRAPHICS: drawing context has no DrawImage, images will not be shown")
}

// DirtyRowsUnder reports whether any text row overlapped by a placement was
// repainted. Framebuffer backends redraw a whole row at a time, so a change
// anywhere in such a row wipes the image pixels sitting in it.
func (g *GraphicsLayer) DirtyRowsUnder(buf, shadow []CharInfo, width, height int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.placements) == 0 || width <= 0 || height <= 0 {
		return false
	}
	if len(buf) < width*height || len(shadow) < width*height {
		return false
	}
	for i := range g.placements {
		p := &g.placements[i]
		for row := p.Row; row < p.Row+p.Rows; row++ {
			if row < 0 || row >= height {
				continue
			}
			off := row * width
			for col := 0; col < width; col++ {
				if buf[off+col] != shadow[off+col] {
					return true
				}
			}
		}
	}
	return false
}
