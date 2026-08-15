//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !arm

package vtui

import (
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // Включаем аппаратное ускорение рендеринга
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
)

type GogpuRenderer struct {
	mu           sync.Mutex
	host         *GogpuHost
	face         text.Face
	fallbacks    []text.Face
	faceCache    map[rune]text.Face
	cellW, cellH int // logical cell sizes from font measurement
	cols, rows   int // dimensions of the current renderBuf

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape
	lastCursorReset  time.Time
	lastBlinkState   bool
	blinkState       bool
	lastBlinkTime    time.Time

	canvas    *ggcanvas.Canvas
	renderBuf []CharInfo
	dirty     bool

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool
}

func NewGogpuRenderer(host *GogpuHost, face text.Face, cw, ch int) *GogpuRenderer {
	return &GogpuRenderer{
		host:            host,
		face:            face,
		cellW:           cw,
		cellH:           ch,
		lastCursorReset: time.Now(),
		lastBlinkState:  true,
		blinkState:      true,
		lastBlinkTime:   time.Now(),
	}
}

// SetFallbackFaces installs the faces consulted for runes the primary font
// has no glyph for. Passing nil restores primary-only rendering.
func (r *GogpuRenderer) SetFallbackFaces(faces []text.Face) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fallbacks = faces
	r.faceCache = nil
}

// faceFor resolves the face that actually owns a glyph for ch, memoising the
// answer. The probe itself is a cmap lookup, but it happens once per distinct
// rune on screen rather than once per cell per frame, which is what keeps this
// off the hot path: a panel full of Latin text asks exactly once per letter
// and then never again.
//
// The caller must hold r.mu. DrawToScreen, the only caller, holds it for the
// whole frame.
func (r *GogpuRenderer) faceFor(ch rune) text.Face {
	if r.face == nil || len(r.fallbacks) == 0 {
		return r.face
	}
	if f, ok := r.faceCache[ch]; ok {
		return f
	}

	f := r.face
	if !r.face.HasGlyph(ch) {
		for _, fb := range r.fallbacks {
			if fb != nil && fb.HasGlyph(ch) {
				f = fb
				break
			}
		}
	}

	if r.faceCache == nil {
		r.faceCache = make(map[rune]text.Face, 256)
	}
	r.faceCache[ch] = f
	return f
}

func (r *GogpuRenderer) Render(buf, shadow []CharInfo, w, h int, force bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cols = w
	r.rows = h

	needsRedraw := force
	if !needsRedraw {
		for i := 0; i < len(buf); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}

	now := time.Now()
	if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
		r.blinkState = !r.blinkState
		r.lastBlinkTime = r.lastBlinkTime.Add(500 * time.Millisecond)
		if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
			r.lastBlinkTime = now
		}
	}

	if !needsRedraw && r.cursorVis {
		if r.blinkState != r.lastBlinkState {
			needsRedraw = true
			r.lastBlinkState = r.blinkState
		}
	}

	if !needsRedraw {
		return
	}

	if len(r.renderBuf) != len(buf) {
		r.renderBuf = make([]CharInfo, len(buf))
	}
	copy(r.renderBuf, buf)
	r.dirty = true
}

func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.cursorX, r.cursorY = x, y
		r.cursorVis = visible
		r.cursorShape = shape
		r.lastCursorReset = time.Now()
		r.lastBlinkState = true
		r.blinkState = true
		r.dirty = true
	}
	r.mu.Unlock()
}

// RenderGraphics implements GraphicsRenderer. The GPU canvas is rebuilt as a
// whole whenever it is dirty, so here we only have to remember the snapshot
// and make sure the next frame is considered dirty.
func (r *GogpuRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	list, gen := layer.Snapshot(nil)

	r.mu.Lock()
	if force || !r.gfxKnown || gen != r.gfxGen {
		r.dirty = true
	}
	r.gfxList = list
	r.gfxGen = gen
	r.gfxKnown = true
	r.mu.Unlock()
}

func (r *GogpuRenderer) SetPalette(pal *[256]uint32) {}

func (r *GogpuRenderer) ResizeWindow(cols, rows int) {
	r.host.mu.Lock()
	app := r.host.app
	cw, ch := r.cellW, r.cellH
	r.host.mu.Unlock()

	if app != nil && cw > 0 && ch > 0 {
		app.RequestSize(cols*cw, rows*ch)
	}
}

func (r *GogpuRenderer) drawCustomChar(dc *gg.Context, char rune, x, y, w, h float64) bool {
	thick := 1.0

	mx := math.Floor(x + w/2 - thick/2)
	my := math.Floor(y + h/2 - thick/2)

	// Double line offsets
	ofs := math.Floor(math.Min(w, h) / 4)
	if ofs < 1 {
		ofs = 1
	}

	switch char {
	case '─', '━':
		t := thick
		if char == '━' {
			t *= 2
		}
		dc.DrawRectangle(x, math.Floor(y+h/2-t/2), w, t)
	case '│', '┃':
		t := thick
		if char == '┃' {
			t *= 2
		}
		dc.DrawRectangle(math.Floor(x+w/2-t/2), y, t, h)
	case '┌', '┏':
		t := thick
		if char == '┏' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '┐', '┓':
		t := thick
		if char == '┓' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '└', '┗':
		t := thick
		if char == '┗' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '┘', '┛':
		t := thick
		if char == '┛' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '├', '┣':
		t := thick
		if char == '┣' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, y, t, h)
	case '┤', '┫':
		t := thick
		if char == '┫' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, y, t, h)
	case '┬', '┳':
		t := thick
		if char == '┳' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '┴', '┻':
		t := thick
		if char == '┻' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '┼', '╋':
		t := thick
		if char == '╋' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, y, t, h)

	// Double lines
	case '═':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
	case '║':
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╔':
		dc.DrawRectangle(mx+ofs, my-ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, my+ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx-ofs, my+ofs, thick, (y+h)-(my+ofs))
		dc.DrawRectangle(mx+ofs, my-ofs, thick, (y+h)-(my-ofs))
	case '╗':
		dc.DrawRectangle(x, my-ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(x, my+ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, my+ofs, thick, (y+h)-(my+ofs))
		dc.DrawRectangle(mx-ofs, my-ofs, thick, (y+h)-(my-ofs))
	case '╚':
		dc.DrawRectangle(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, (my-ofs)-y+thick)
		dc.DrawRectangle(mx+ofs, y, thick, (my+ofs)-y+thick)
	case '╝':
		dc.DrawRectangle(x, my-ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(x, my+ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, y, thick, (my-ofs)-y+thick)
		dc.DrawRectangle(mx-ofs, y, thick, (my+ofs)-y+thick)
	case '╠':
		dc.DrawRectangle(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╣':
		dc.DrawRectangle(x, my-ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(x, my+ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, y, thick, h)
		dc.DrawRectangle(mx-ofs, y, thick, h)
	case '╦':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, my+ofs, thick, h-(my-y+ofs))
		dc.DrawRectangle(mx+ofs, my+ofs, thick, h-(my-y+ofs))
	case '╩':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, y, thick, my-y-ofs+thick)
		dc.DrawRectangle(mx+ofs, y, thick, my-y-ofs+thick)
	case '╬':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)

	// Mixed (used in VMenu)
	case '╟':
		dc.DrawRectangle(mx+ofs, my, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╢':
		dc.DrawRectangle(x, my, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)

	// Arrows and Triangles
	case '↑':
		dc.DrawLine(mx, y+h*0.15, mx, y+h*0.85)
		dc.DrawLine(mx, y+h*0.15, mx-w*0.25, y+h*0.4)
		dc.DrawLine(mx, y+h*0.15, mx+w*0.25, y+h*0.4)
		dc.SetLineWidth(thick)
		dc.Stroke()
		dc.SetLineWidth(0)
		return true
	case '↓':
		dc.DrawLine(mx, y+h*0.15, mx, y+h*0.85)
		dc.DrawLine(mx, y+h*0.85, mx-w*0.25, y+h*0.6)
		dc.DrawLine(mx, y+h*0.85, mx+w*0.25, y+h*0.6)
		dc.SetLineWidth(thick)
		dc.Stroke()
		dc.SetLineWidth(0)
		return true
	case '↕':
		dc.DrawLine(mx, y+h*0.15, mx, y+h*0.85)
		dc.DrawLine(mx, y+h*0.15, mx-w*0.25, y+h*0.35)
		dc.DrawLine(mx, y+h*0.15, mx+w*0.25, y+h*0.35)
		dc.DrawLine(mx, y+h*0.85, mx-w*0.25, y+h*0.65)
		dc.DrawLine(mx, y+h*0.85, mx+w*0.25, y+h*0.65)
		dc.SetLineWidth(thick)
		dc.Stroke()
		dc.SetLineWidth(0)
		return true
	case '▲':
		dc.MoveTo(mx, y+h*0.2)
		dc.LineTo(mx-w*0.3, y+h*0.8)
		dc.LineTo(mx+w*0.3, y+h*0.8)
		dc.ClosePath()
		dc.Fill()
		return true
	case '▼':
		dc.MoveTo(mx-w*0.3, y+h*0.2)
		dc.LineTo(mx+w*0.3, y+h*0.2)
		dc.LineTo(mx, y+h*0.8)
		dc.ClosePath()
		dc.Fill()
		return true

	// Solid Blocks
	case '█':
		dc.DrawRectangle(x, y, w, h)
	case '▀':
		dc.DrawRectangle(x, y, w, h/2)
	case '▄':
		dc.DrawRectangle(x, y+h/2, w, h/2)
	case '▌':
		dc.DrawRectangle(x, y, w/2, h)
	case '▐':
		dc.DrawRectangle(x+w/2, y, w/2, h)

	default:
		return false
	}

	dc.Fill()
	return true
}

func (r *GogpuRenderer) Flush() {
	r.host.mu.Lock()
	app := r.host.app
	forceDirty := r.host.resizePending
	r.host.resizePending = false
	r.host.mu.Unlock()

	r.mu.Lock()
	if forceDirty {
		r.dirty = true
		r.lastCursorReset = time.Now() // Ensure cursor is solid-visible on window restore/resize
	}
	shouldRedraw := r.dirty
	r.mu.Unlock()

	if shouldRedraw && app != nil {
		go app.RequestRedraw()
	}
}

func (r *GogpuRenderer) DrawToScreen(ctx *gogpu.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.renderBuf) == 0 {
		return
	}

	w, h := ctx.Width(), ctx.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if debugLastCtxW != w || debugLastCtxH != h {
		debugLastCtxW, debugLastCtxH = w, h
		if r.canvas != nil {
			r.canvas.Resize(w, h)
		}
		r.dirty = true
	}

	if r.canvas == nil {
		provider := r.host.app.GPUContextProvider()
		if provider == nil {
			return
		}
		r.canvas, _ = ggcanvas.New(provider, w, h)
	}

	var prof gogpuFrameStats
	drew := false

	if r.dirty {
		tDraw := gogpuProfNow()
		r.canvas.Draw(func(dc *gg.Context) {
			dc.SetRGB(0, 0, 0)
			dc.DrawRectangle(0, 0, float64(w), float64(h))
			dc.Fill()

			drawCols := r.cols
			drawRows := r.rows

			// The nil check below is not decoration: every other use of the
			// face in this function is guarded, and an unguarded Metrics()
			// call here would fault exactly the way the render thread did.
			var ascent float64
			if r.face != nil {
				dc.SetFont(r.face)
				ascent = float64(r.face.Metrics().Ascent)
			}

			// The baseline stays the primary font's for the whole frame even
			// when a fallback draws the glyph: a CJK face with its own ascent
			// would make text jump between lines that mix scripts.
			curFace := r.face

			for y := 0; y < drawRows; y++ {
				rowOff := y * drawCols
				ly := float64(y * r.cellH)
				for x := 0; x < drawCols; {
					cell := r.renderBuf[rowOff+x]
					fg, bg := r.getCellColors(cell)

					spanW := 0
					for x+spanW < drawCols {
						nextCell := r.renderBuf[rowOff+x+spanW]
						if nextCell.Char == WideCharFiller {
							spanW++
							continue
						}
						nextFg, nextBg := r.getCellColors(nextCell)
						if nextBg != bg || nextFg != fg {
							break
						}
						spanW++
					}

					lx := float64(x * r.cellW)
					spanPixW := float64(spanW * r.cellW)

					prof.spans++
					tBg := gogpuProfNow()
					dc.SetColor(bg)
					dc.DrawRectangle(lx, ly, spanPixW+1, float64(r.cellH)+1)
					dc.Fill()
					prof.bgFills++
					prof.bgTime += gogpuProfSince(tBg)
					dc.SetColor(fg)

					for sx := 0; sx < spanW; {
						idx := rowOff + x + sx
						currCell := r.renderBuf[idx]

						if currCell.Char == WideCharFiller {
							sx++
							continue
						}

						rw := 1
						if x+sx+1 < drawCols && r.renderBuf[idx+1].Char == WideCharFiller {
							rw = 2
						}

						char := CellBaseRune(currCell.Char)
						isBox := (char >= 0x2500 && char <= 0x25BF) || (char >= 0x2190 && char <= 0x2195)

						if isBox {
							tBox := gogpuProfNow()
							drawn := r.drawCustomChar(dc, char, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH))
							prof.boxTime += gogpuProfSince(tBox)
							if drawn {
								prof.boxChars++
								sx += rw
								continue
							}
						}

						str := CellString(currCell.Char)
						if str != "" && str != " " && r.face != nil {
							tTxt := gogpuProfNow()
							if f := r.faceFor(char); f != nil && f != curFace {
								curFace = f
								dc.SetFont(f)
							}
							dc.DrawString(str, lx+float64(sx*r.cellW), ly+ascent)
							prof.textTime += gogpuProfSince(tTxt)
							prof.strings++
							prof.glyphs += gogpuRuneCount(str)
						}
						sx += rw
					}

					x += spanW
				}
			}

			for i := range r.gfxList {
				p := &r.gfxList[i]
				px, py, pw, ph := placementPixelRect(p, r.cellW, r.cellH)
				if pw <= 0 || ph <= 0 {
					continue
				}
				entry := r.gfxCache.scaled(p, pw, ph)
				if entry == nil {
					continue
				}

				dc.SetRGBA(1, 1, 1, 1)
				dc.DrawImage(gg.ImageBufFromImage(entry.asImage()), float64(px), float64(py))
			}

			cursorVisible := r.cursorVis && r.blinkState

			if cursorVisible {
				dc.SetColor(color.White)
				curX, curSpan := CellSpanAt(r.renderBuf, drawCols, r.cursorX, r.cursorY)
				cx := float64(curX * r.cellW)
				cy := float64(r.cursorY * r.cellH)
				curW := float64(curSpan * r.cellW)
				if r.cursorShape == CursorShapeBlock {
					dc.DrawRectangle(cx, cy, curW, float64(r.cellH))
				} else {
					cy += float64(r.cellH) - 2
					dc.DrawRectangle(cx, cy, curW, 2)
				}
				dc.Fill()
			}
		})
		prof.drawTime = gogpuProfSince(tDraw)
		r.dirty = false
		drew = true
	}

	tRender := gogpuProfNow()
	r.canvas.Render(ctx.RenderTarget())
	prof.renderTime = gogpuProfSince(tRender)

	if gogpuProfileEnabled && drew {
		prof.report(r.cols, r.rows)
	}
}

func (r *GogpuRenderer) SetWindowTitle(title string) {
	r.host.app.SetTitle(title)
}
func (r *GogpuRenderer) getCellColors(cell CharInfo) (color.Color, color.Color) {
	bg := GetRGBBack(cell.Attributes)
	if cell.Attributes&IsBgRGB == 0 {
		bg = ThemePalette[GetIndexBack(cell.Attributes)]
	}
	fg := GetRGBFore(cell.Attributes)
	if cell.Attributes&IsFgRGB == 0 {
		fg = ThemePalette[GetIndexFore(cell.Attributes)]
	}

	f := color.RGBA{uint8(fg >> 16), uint8(fg >> 8), uint8(fg), 255}
	b := color.RGBA{uint8(bg >> 16), uint8(bg >> 8), uint8(bg), 255}
	return f, b
}
