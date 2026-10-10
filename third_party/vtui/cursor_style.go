package vtui

import (
	"strconv"
	"sync/atomic"
	"time"
)

// The caret a text-entry widget asks for is a role, not a picture: "the
// insert-mode caret" or "the overtype-mode caret". Which shape each role
// gets, and whether the caret blinks, is the application's choice (f4
// #1154), so widgets ask InsertCursorShape / OvertypeCursorShape instead of
// naming CursorShapeUnderline or CursorShapeBlock. A caller that needs a
// particular picture regardless of preference -- a cell marker rather than
// a text caret -- still passes the constant.
//
// The defaults are the shapes vtui always used: an underline for insert, a
// block for overtype, blinking.
var (
	insertCursorShape   atomic.Int32 // zero value is CursorShapeUnderline
	overtypeCursorShape atomic.Int32
	cursorSteady        atomic.Bool // inverse of blinking, so the zero value blinks
)

func init() {
	overtypeCursorShape.Store(int32(CursorShapeBlock))
}

// SetCursorStyle sets the caret shape used for insert and overtype text
// entry, and whether the caret blinks. An unknown shape falls back to the
// default of its role.
//
// On the ANSI backend it is honored while ManageCursorStyle is on: shape and
// blink go out as DECSCUSR (CSI Ps SP q) together with OSC 1337 CursorShape.
// The Linux virtual console (TERM=linux) and the classic Windows console
// have no vertical bar and no blink control, so there a bar is drawn the way
// an underline is and the blink setting is left to the console. The GUI
// renderers draw every shape and blink themselves.
func SetCursorStyle(insert, overtype CursorShape, blink bool) {
	if !insert.valid() {
		insert = CursorShapeUnderline
	}
	if !overtype.valid() {
		overtype = CursorShapeBlock
	}
	insertCursorShape.Store(int32(insert))
	overtypeCursorShape.Store(int32(overtype))
	cursorSteady.Store(!blink)
}

// InsertCursorShape is the caret shape for text entry in insert mode.
func InsertCursorShape() CursorShape { return CursorShape(insertCursorShape.Load()) }

// OvertypeCursorShape is the caret shape for text entry in overtype mode.
func OvertypeCursorShape() CursorShape { return CursorShape(overtypeCursorShape.Load()) }

// CursorBlinks reports whether the caret is to blink.
func CursorBlinks() bool { return !cursorSteady.Load() }

// ParseCursorShape maps the stable configuration names "underline", "bar"
// and "block" to a shape. ok is false for anything else.
func ParseCursorShape(name string) (shape CursorShape, ok bool) {
	switch name {
	case "underline":
		return CursorShapeUnderline, true
	case "bar":
		return CursorShapeBar, true
	case "block":
		return CursorShapeBlock, true
	}
	return CursorShapeUnderline, false
}

// String returns the configuration name ParseCursorShape accepts.
func (s CursorShape) String() string {
	switch s {
	case CursorShapeBar:
		return "bar"
	case CursorShapeBlock:
		return "block"
	}
	return "underline"
}

func (s CursorShape) valid() bool {
	return s == CursorShapeUnderline || s == CursorShapeBar || s == CursorShapeBlock
}

// cursorStyleSeq is DECSCUSR, CSI Ps SP q (xterm ctlseqs): 1 blinking
// block, 2 steady block, 3 blinking underline, 4 steady underline, 5
// blinking bar, 6 steady bar.
func cursorStyleSeq(shape CursorShape, blink bool) string {
	ps := 3
	switch shape {
	case CursorShapeBlock:
		ps = 1
	case CursorShapeBar:
		ps = 5
	}
	if !blink {
		ps++
	}
	return "\x1b[" + strconv.Itoa(ps) + " q"
}

// cursorShapeOSC1337 is the Konsole/iTerm2 form, OSC 1337 ; CursorShape=N
// ST with 0 block, 1 vertical bar, 2 underline. It carries no blink.
func cursorShapeOSC1337(shape CursorShape) string {
	n := "2"
	switch shape {
	case CursorShapeBlock:
		n = "0"
	case CursorShapeBar:
		n = "1"
	}
	return "\x1b]1337;CursorShape=" + n + "\x07"
}

// cursorCellRect is the part of a caret cell a GUI renderer paints, relative
// to the cell's top-left pixel: all of it for a block, a line along the
// bottom for an underline, a line down the left edge for a bar. spanW is the
// pixel width of the cell (two cells for a wide character). Lines are two
// pixels thick, four on a scaled display, so they do not thin to a hair on a
// HiDPI screen. The result is [x0,x1) x [y0,y1), clipped to the cell.
func cursorCellRect(shape CursorShape, spanW, cellH int, scaled bool) (x0, y0, x1, y1 int) {
	thickness := 2
	if scaled {
		thickness = 4
	}
	x1, y1 = spanW, cellH
	switch shape {
	case CursorShapeBlock:
	case CursorShapeBar:
		if thickness < x1 {
			x1 = thickness
		}
	default:
		if y0 = cellH - thickness; y0 < 0 {
			y0 = 0
		}
	}
	return x0, y0, x1, y1
}

// softwareBlinkPeriod is the half-period of a caret the renderer blinks.
const softwareBlinkPeriod = 500 * time.Millisecond

// stepSoftwareBlink advances the blink phase of a renderer that draws its
// own caret. The phase runs on wall clock, so a renderer at 60fps and one at
// 15fps blink alike; a renderer that fell far behind resynchronizes instead
// of flickering through the missed half-periods. With blinking off the
// caret stays in its visible phase.
func stepSoftwareBlink(visible *bool, last *time.Time, now time.Time) {
	if !CursorBlinks() {
		*visible = true
		*last = now
		return
	}
	if now.Sub(*last) >= softwareBlinkPeriod {
		*visible = !*visible
		*last = last.Add(softwareBlinkPeriod)
		if now.Sub(*last) >= softwareBlinkPeriod {
			*last = now
		}
	}
}

// invertCursorRect inverts the caret's part of a cell in an RGBA pixel
// buffer, so the caret stays visible whatever colours the cell carries.
// (px, py) is the cell's top-left pixel.
func invertCursorRect(pix []uint8, stride, maxX, maxY, px, py int, shape CursorShape, spanW, cellH int, scaled bool) {
	x0, y0, x1, y1 := cursorCellRect(shape, spanW, cellH, scaled)
	for iy := y0; iy < y1; iy++ {
		pixelY := py + iy
		if pixelY < 0 {
			continue
		}
		if pixelY >= maxY {
			break
		}
		row := pixelY * stride
		for ix := x0; ix < x1; ix++ {
			pixelX := px + ix
			if pixelX < 0 {
				continue
			}
			if pixelX >= maxX {
				break
			}
			off := row + pixelX*4
			if off+2 < len(pix) {
				pix[off] = 255 - pix[off]
				pix[off+1] = 255 - pix[off+1]
				pix[off+2] = 255 - pix[off+2]
			}
		}
	}
}
