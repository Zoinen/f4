package vtui

import (
	"strconv"
	"sync/atomic"
	"time"
)

var (
	insertCursorShape   atomic.Int32
	overtypeCursorShape atomic.Int32
	cursorSteady        atomic.Bool
)

func init() {
	overtypeCursorShape.Store(int32(CursorShapeBlock))
}

// SetCursorStyle stores the caret shapes used by insert and overtype text
// entry and whether the caret blinks. Renderer-specific code reads these
// values when it paints or flushes the cursor.
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

func InsertCursorShape() CursorShape { return CursorShape(insertCursorShape.Load()) }

func OvertypeCursorShape() CursorShape { return CursorShape(overtypeCursorShape.Load()) }

func CursorBlinks() bool { return !cursorSteady.Load() }

func ParseCursorShape(name string) (CursorShape, bool) {
	switch name {
	case "underline":
		return CursorShapeUnderline, true
	case "bar":
		return CursorShapeBar, true
	case "block":
		return CursorShapeBlock, true
	default:
		return CursorShapeUnderline, false
	}
}

func (s CursorShape) String() string {
	switch s {
	case CursorShapeBar:
		return "bar"
	case CursorShapeBlock:
		return "block"
	default:
		return "underline"
	}
}

func (s CursorShape) valid() bool {
	return s == CursorShapeUnderline || s == CursorShapeBar || s == CursorShapeBlock
}

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

const softwareBlinkPeriod = 500 * time.Millisecond

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

func invertCursorRect(pix []uint8, stride, maxX, maxY, px, py int, shape CursorShape, spanW, cellH int, scaled bool) {
	x0, y0, x1, y1 := cursorCellRect(shape, spanW, cellH, scaled)
	for iy := y0; iy < y1; iy++ {
		pixelY := py + iy
		if pixelY < 0 || pixelY >= maxY {
			continue
		}
		row := pixelY * stride
		for ix := x0; ix < x1; ix++ {
			pixelX := px + ix
			if pixelX < 0 || pixelX >= maxX {
				continue
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
