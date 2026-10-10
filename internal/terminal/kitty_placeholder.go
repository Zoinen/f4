package terminal

// Unicode placeholders of the kitty graphics protocol (f4#1685). A program
// creates a "virtual" placement of an image (a=p,U=1,i=ID,c=COLS,r=ROWS), which
// shows nothing by itself, and then prints the character U+10EEEE where the
// picture should appear. Which piece of the picture a cell shows, and of which
// image, is written in the cell: the foreground colour carries the image id,
// and up to three combining marks after the placeholder carry the row, the
// column and the top byte of the id (kitty_diacritics.go).
//
// Because the picture lives in the text, it scrolls, wraps, is overwritten and
// is erased with the text, which is what makes the protocol usable inside
// multiplexers and full-screen programs. So nothing is stored as a placement:
// the cells are read again whenever the screen is drawn, and the pictures are
// worked out from them.

import (
	"fmt"

	"github.com/unxed/vtui"
)

const kittyPlaceholderRune = 0x10EEEE

var kittyDiacriticIndex = func() map[rune]int {
	m := make(map[rune]int, len(kittyDiacritics))
	for i, r := range kittyDiacritics {
		m[r] = i
	}
	return m
}()

// kittyVirtual is a virtual placement: the size in cells of the area an image
// is fitted into.
type kittyVirtual struct {
	Cols, Rows int
	// Surface is kept here so that drawing never has to ask the graphics
	// receiver, which calls into the terminal while holding its own lock.
	Surface *vtui.ImageSurface
}

// placeholderCell is what the marks after one placeholder said. n is how many
// of the three were present, since a missing one is inherited from the cell on
// the left.
type placeholderCell struct {
	n             int
	row, col, msb int
}

// placeholderRun is where the last placeholder was written, so that the marks
// that follow it can be attached to it instead of taking cells of their own.
type placeholderRun struct {
	ok  bool
	row *vtui.CharInfo // identity of the screen row
	x   int
}

// noteVirtualPlacement records a virtual placement. The caller holds the lock.
func (tv *TerminalView) noteVirtualPlacement(img *kittyImage, cmd kittyCommand) {
	if tv.virtual == nil {
		tv.virtual = make(map[uint32]kittyVirtual)
	}
	tv.virtual[img.ID] = kittyVirtual{Cols: cmd.Int('c', 0), Rows: cmd.Int('r', 0), Surface: img.Surface}
}

// placeholderStarted remembers the placeholder just written at (x, y).
func (tv *TerminalView) placeholderStarted(y, x int) {
	buf := tv.GetBuffer()
	if y < 0 || y >= len(buf) || len(buf[y]) == 0 {
		return
	}
	key := &buf[y][0]
	if tv.phMeta == nil {
		tv.phMeta = make(map[*vtui.CharInfo]map[int]placeholderCell)
	}
	if tv.phMeta[key] == nil {
		tv.phMeta[key] = make(map[int]placeholderCell)
	}
	tv.phMeta[key][x] = placeholderCell{}
	tv.phLast = placeholderRun{ok: true, row: key, x: x}
}

// placeholderMark attaches a combining mark to the placeholder before it. It
// reports whether the mark was taken, in which case it must not be drawn.
func (tv *TerminalView) placeholderMark(r rune) bool {
	if !tv.phLast.ok {
		return false
	}
	idx, ok := kittyDiacriticIndex[r]
	if !ok {
		return false
	}
	cell := tv.phMeta[tv.phLast.row][tv.phLast.x]
	switch cell.n {
	case 0:
		cell.row = idx
	case 1:
		cell.col = idx
	case 2:
		cell.msb = idx
	default:
		return false
	}
	cell.n++
	tv.phMeta[tv.phLast.row][tv.phLast.x] = cell
	return true
}

// placeholderColorKey folds a foreground colour into one comparable number: an
// index in 256-colour mode, the 24 bits of an RGB colour otherwise. It is also
// the low 24 bits of the image id.
func placeholderColorKey(attr uint64) (id uint32, rgb bool) {
	if attr&vtui.IsFgRGB != 0 {
		return vtui.GetRGBFore(attr), true
	}
	return uint32(vtui.GetIndexFore(attr)), false
}

// placeholderRunOut is a stretch of placeholder cells on one screen row that
// show consecutive columns of one row of one image.
type placeholderRunOut struct {
	id    uint32
	vrow  int
	col0  int // first column of the image's grid
	x0, y int // where the stretch starts on the screen
	n     int
}

// drawPlaceholders paints the pictures the placeholder cells stand for. The
// caller holds the lock.
func (tv *TerminalView) drawPlaceholders(scr *vtui.ScreenBuf, offset int) {
	if len(tv.virtual) == 0 || !scr.SupportsGraphics() {
		return
	}
	buf := tv.GetBuffer()
	cw, ch := tv.cellSizeUnsafe()

	for y := 0; y < tv.Height && y < len(buf); y++ {
		line := buf[y]
		if len(line) == 0 {
			continue
		}
		meta := tv.phMeta[&line[0]]
		if len(meta) == 0 {
			continue
		}
		var (
			prev    placeholderCell
			prevKey uint32
			prevRGB bool
			havePrv bool
			run     *placeholderRunOut
		)
		flush := func() {
			if run != nil {
				tv.drawPlaceholderRun(scr, offset, *run, cw, ch)
				run = nil
			}
		}
		for x := 0; x < len(line); x++ {
			if line[x].Char != kittyPlaceholderRune {
				flush()
				havePrv = false
				continue
			}
			m := meta[x]
			key, rgb := placeholderColorKey(line[x].Attributes)
			same := havePrv && prevKey == key && prevRGB == rgb

			cur := m
			switch {
			case m.n == 0 && same:
				cur.row, cur.col, cur.msb = prev.row, prev.col+1, prev.msb
			case m.n == 0:
				// Nothing to say which piece this is and nothing to inherit.
				flush()
				havePrv = false
				continue
			case m.n == 1:
				if same && prev.row == m.row {
					cur.col, cur.msb = prev.col+1, prev.msb
				}
			case m.n == 2:
				if same && prev.row == m.row && prev.col+1 == m.col {
					cur.msb = prev.msb
				}
			}
			prev, prevKey, prevRGB, havePrv = cur, key, rgb, true

			id := key | uint32(cur.msb&0xFF)<<24 // #nosec G115 -- masked to one byte, the top byte of the id
			if run != nil && run.id == id && run.vrow == cur.row && run.col0+run.n == cur.col && run.x0+run.n == x {
				run.n++
				continue
			}
			flush()
			run = &placeholderRunOut{id: id, vrow: cur.row, col0: cur.col, x0: x, y: y, n: 1}
		}
		flush()
	}
}

// drawPlaceholderRun paints one stretch: the part of the image that falls on
// its cells, taken from the picture fitted inside the virtual placement.
func (tv *TerminalView) drawPlaceholderRun(scr *vtui.ScreenBuf, offset int, r placeholderRunOut, cw, ch int) {
	virt, ok := tv.virtual[r.id]
	if !ok || !virt.Surface.Valid() {
		return
	}
	iw, ih := virt.Surface.Width, virt.Surface.Height
	vc, vr := virt.Cols, virt.Rows
	if vc <= 0 || vr <= 0 {
		vc, vr = kittySpanFor(vc, vr, iw, ih, cw, ch)
	}
	if vc <= 0 || vr <= 0 {
		return
	}

	// The picture is fitted inside the vc x vr cells, keeping its proportions
	// and centred; the cells it does not reach stay empty.
	areaW, areaH := float64(vc*cw), float64(vr*ch)
	scale := areaW / float64(iw)
	if s := areaH / float64(ih); s < scale {
		scale = s
	}
	shownW, shownH := float64(iw)*scale, float64(ih)*scale
	offX, offY := (areaW-shownW)/2, (areaH-shownH)/2

	// The run's rectangle in the area, cut down to what the picture covers.
	x0, x1 := float64(r.col0*cw), float64((r.col0+r.n)*cw)
	y0, y1 := float64(r.vrow*ch), float64((r.vrow+1)*ch)
	if x0 < offX {
		x0 = offX
	}
	if y0 < offY {
		y0 = offY
	}
	if x1 > offX+shownW {
		x1 = offX + shownW
	}
	if y1 > offY+shownH {
		y1 = offY + shownH
	}
	if x1 <= x0 || y1 <= y0 {
		return
	}

	ip := vtui.ImagePlacement{
		Surface: virt.Surface,
		Col:     tv.X1 + r.x0,
		Row:     tv.Y1 + r.y,
		Cols:    r.n,
		Rows:    1,
		SrcX:    int((x0 - offX) / scale),
		SrcY:    int((y0 - offY) / scale),
		SrcW:    int((x1-x0)/scale + 0.5),
		SrcH:    int((y1-y0)/scale + 0.5),
	}
	if !tv.UseAltScreen {
		ip.Row += offset
	}
	if ip.SrcW < 1 {
		ip.SrcW = 1
	}
	if ip.SrcH < 1 {
		ip.SrcH = 1
	}
	left, top := tv.X1, tv.Y1
	right, bottom := tv.X1+tv.Width-1, tv.Y1+tv.Height-1
	if !kittyClipPlacement(&ip, left, top, right, bottom) {
		return
	}
	scr.Graphics().DrawImage(fmt.Sprintf("ph:%d:%d:%d", r.id, r.y, r.x0), ip)
}
