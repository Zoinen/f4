package main

// Placement half of the kitty graphics protocol: where in the terminal grid a
// received image is shown. A placement is anchored to a row of the grid, so
// the picture scrolls together with the text it was printed next to and it
// disappears once that text is gone.

import (
	"strconv"

	"github.com/unxed/vtui"
)

const (
	// kittyMaxPlacements bounds how many pictures one screen can hold.
	kittyMaxPlacements = 32

	// Cell metrics used while the real ones are unknown. They match the
	// answer the terminal gives to CSI 16 t, so a client that asks and a
	// client that guesses end up with the same geometry.
	kittyFallbackCellW = 8
	kittyFallbackCellH = 16
)

// terminalImage is one placement on the grid of the built-in terminal.
type terminalImage struct {
	Key       string
	ImageID   uint32
	Number    uint32
	Placement uint32
	Surface   *vtui.ImageSurface

	Col  int
	Row  int
	Cols int
	Rows int

	SrcX int
	SrcY int
	SrcW int
	SrcH int

	// WantCols and WantRows are the span the client asked for in c and r,
	// zero for a side it left to us to work out. They are kept so that our
	// side can be worked out again when a cell changes size or the terminal
	// is resized.
	WantCols int
	WantRows int

	Z   int
	Alt bool

	// Sixel marks a placement that came from a sixel sequence. Those have
	// no image id and no way of being addressed after the fact, so the id
	// and number forms of the kitty delete command must not sweep them up
	// on their way past: without the flag every one of them would answer
	// to i=0, which is what an a=d,d=I with no i asks for.
	Sixel bool

	// Far2lID is the name far2l gave the picture over its own channel.
	// That protocol, unlike sixel, can address a picture after the fact,
	// which is what its delete command needs.
	Far2lID string
}

// covers reports whether the placement paints over the given grid cell.
func (p *terminalImage) covers(col, row int) bool {
	return col >= p.Col && col < p.Col+p.Cols && row >= p.Row && row < p.Row+p.Rows
}

// kittyDisplay connects the receiver of the protocol to the terminal grid.
// It is a separate type so that the placement methods stay out of the public
// surface of TerminalView.
type kittyDisplay struct{ tv *TerminalView }

func (d kittyDisplay) Put(img *kittyImage, cmd kittyCommand) string {
	return d.tv.kittyPut(img, cmd)
}

func (d kittyDisplay) Delete(cmd kittyCommand) []uint32 {
	return d.tv.kittyDelete(cmd)
}

func (d kittyDisplay) DropImage(id uint32) {
	d.tv.kittyDropImage(id)
}

// kittyPut shows an image at the cursor and moves the cursor past it.
func (tv *TerminalView) kittyPut(img *kittyImage, cmd kittyCommand) string {
	if img == nil || !img.Surface.Valid() {
		return "EINVAL:the image has no pixels"
	}

	tv.mu.Lock()
	defer tv.mu.Unlock()

	p := terminalImage{
		ImageID:   img.ID,
		Number:    img.Number,
		Placement: cmd.Uint32('p', 0),
		Surface:   img.Surface,
		Col:       tv.CursorX,
		Row:       tv.CursorY,
		Z:         cmd.Int('z', 0),
		Alt:       tv.UseAltScreen,
	}

	// The source rectangle is given in pixels of the stored image; an empty
	// one means the whole picture.
	p.SrcX, p.SrcY = cmd.Int('x', 0), cmd.Int('y', 0)
	p.SrcW, p.SrcH = cmd.Int('w', 0), cmd.Int('h', 0)
	if p.SrcX < 0 || p.SrcY < 0 || p.SrcX >= img.Surface.Width || p.SrcY >= img.Surface.Height {
		return "EINVAL:the source rectangle is outside the image"
	}
	if p.SrcW <= 0 || p.SrcX+p.SrcW > img.Surface.Width {
		p.SrcW = img.Surface.Width - p.SrcX
	}
	if p.SrcH <= 0 || p.SrcY+p.SrcH > img.Surface.Height {
		p.SrcH = img.Surface.Height - p.SrcY
	}

	p.WantCols, p.WantRows = cmd.Int('c', 0), cmd.Int('r', 0)
	p.Cols, p.Rows = tv.kittyCellSpan(p.WantCols, p.WantRows, p.SrcW, p.SrcH)
	if p.Cols <= 0 || p.Rows <= 0 {
		return "EINVAL:the placement is empty"
	}

	tv.kittyAddPlacement(p)

	// The protocol moves the cursor past the picture unless the client asks
	// it not to. Clients rely on that: without it the shell prompt would be
	// printed straight over the image.
	if cmd.Int('C', 0) != 1 {
		tv.kittyAdvanceCursor(p.Cols, p.Rows)
	}
	return ""
}

// kittyCellSpan turns the wanted span into a rectangle of cells, clamped to
// the size of the terminal. The caller holds the lock.
func (tv *TerminalView) kittyCellSpan(wantCols, wantRows, srcW, srcH int) (int, int) {
	cw, ch := tv.cellSizeUnsafe()
	cols, rows := kittySpanFor(wantCols, wantRows, srcW, srcH, cw, ch)
	if cols > tv.Width {
		cols = tv.Width
	}
	if rows > tv.Height {
		rows = tv.Height
	}
	return cols, rows
}

// kittySpanFor is the arithmetic on its own, with no terminal attached, so
// that placing a picture and working its span out again after the cell has
// changed size follow exactly the same rules. A side the client did not ask
// for is computed from the aspect ratio, so that the picture is not
// distorted.
func kittySpanFor(wantCols, wantRows, srcW, srcH, cellW, cellH int) (int, int) {
	cols, rows := wantCols, wantRows
	switch {
	case cols > 0 && rows > 0:
	case cols > 0:
		rows = kittyCeilDiv(int64(srcH)*int64(cols)*int64(cellW), int64(srcW)*int64(cellH))
	case rows > 0:
		cols = kittyCeilDiv(int64(srcW)*int64(rows)*int64(cellH), int64(srcH)*int64(cellW))
	default:
		cols = kittyCeilDiv(int64(srcW), int64(cellW))
		rows = kittyCeilDiv(int64(srcH), int64(cellH))
	}
	return cols, rows
}

// kittyRecomputeSpans works every span out again after a cell has changed
// size or the terminal has been resized. A side the client gave in c or r
// stands: it is a promise about the layout of the screen, and keeping it
// matters more than the aspect ratio. What moves is the side we chose and the
// clamp to the screen, so a picture squeezed by a narrow window comes back
// whole when the window widens. The caller holds the lock.
func (tv *TerminalView) kittyRecomputeSpans() {
	for i := range tv.images {
		p := &tv.images[i]
		p.Cols, p.Rows = tv.kittyCellSpan(p.WantCols, p.WantRows, p.SrcW, p.SrcH)
	}
}

func kittyCeilDiv(a, b int64) int {
	if b <= 0 || a <= 0 {
		return 0
	}
	return int((a + b - 1) / b)
}

// kittyAddPlacement installs a placement. A placement that carries an id
// replaces the previous one with the same pair of ids, which is how a client
// moves or resizes a picture without flicker.
func (tv *TerminalView) kittyAddPlacement(p terminalImage) {
	if p.Placement != 0 {
		for i := range tv.images {
			if tv.images[i].ImageID == p.ImageID && tv.images[i].Placement == p.Placement {
				p.Key = tv.images[i].Key
				tv.images[i] = p
				return
			}
		}
	}
	tv.kittyKeySeq++
	p.Key = "kitty.term:" + strconv.FormatUint(tv.kittyKeySeq, 10)
	tv.images = append(tv.images, p)
	if len(tv.images) > kittyMaxPlacements {
		tv.images = append(tv.images[:0], tv.images[len(tv.images)-kittyMaxPlacements:]...)
	}
}

// kittyAdvanceCursor moves the cursor right by the width of the picture and
// down by its height, scrolling the screen when it runs out of room.
func (tv *TerminalView) kittyAdvanceCursor(cols, rows int) {
	tv.CursorX += cols
	if tv.CursorX >= tv.Width {
		tv.CursorX = tv.Width - 1
	}
	if tv.CursorX < 0 {
		tv.CursorX = 0
	}

	target := tv.CursorY + rows
	if target > tv.ScrollBottom {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, target-tv.ScrollBottom)
		target = tv.ScrollBottom
	}
	if target < 0 {
		target = 0
	}
	tv.CursorY = target
	tv.lastCharWasCR = false
}

// kittyDelete executes the placement half of an a=d command and reports the
// images that are left without a placement, which the store may then free.
func (tv *TerminalView) kittyDelete(cmd kittyCommand) []uint32 {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	what := cmd.Char('d', 'a')
	if what >= 'A' && what <= 'Z' {
		what += 'a' - 'A'
	}
	pid := cmd.Uint32('p', 0)
	hasPID := cmd.Has('p')

	kept := make([]terminalImage, 0, len(tv.images))
	var hit []uint32
	for _, p := range tv.images {
		if p.Sixel && (what == 'i' || what == 'n') {
			kept = append(kept, p)
			continue
		}
		match := false
		switch what {
		case 'a':
			match = p.Alt == tv.UseAltScreen
		case 'i':
			match = p.ImageID == cmd.Uint32('i', 0)
		case 'n':
			match = p.Number != 0 && p.Number == cmd.Uint32('I', 0)
		case 'c':
			match = p.covers(tv.CursorX, tv.CursorY)
		case 'p':
			match = p.covers(cmd.Int('x', 1)-1, cmd.Int('y', 1)-1)
		case 'q':
			match = p.Z == cmd.Int('z', 0) && p.covers(cmd.Int('x', 1)-1, cmd.Int('y', 1)-1)
		case 'x':
			col := cmd.Int('x', 1) - 1
			match = col >= p.Col && col < p.Col+p.Cols
		case 'y':
			row := cmd.Int('y', 1) - 1
			match = row >= p.Row && row < p.Row+p.Rows
		case 'z':
			match = p.Z == cmd.Int('z', 0)
		}
		if match && hasPID {
			match = p.Placement == pid
		}
		if match {
			hit = append(hit, p.ImageID)
			continue
		}
		kept = append(kept, p)
	}
	tv.images = kept

	// Report only the images that have no placement left at all.
	var orphaned []uint32
	for _, id := range hit {
		if tv.kittyHasPlacement(id) || kittyContainsID(orphaned, id) {
			continue
		}
		orphaned = append(orphaned, id)
	}
	return orphaned
}

func (tv *TerminalView) kittyHasPlacement(id uint32) bool {
	for i := range tv.images {
		if tv.images[i].ImageID == id {
			return true
		}
	}
	return false
}

func kittyContainsID(list []uint32, id uint32) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// kittyDropImage forgets every placement of an image that no longer exists.
func (tv *TerminalView) kittyDropImage(id uint32) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	kept := make([]terminalImage, 0, len(tv.images))
	for _, p := range tv.images {
		if p.ImageID != id {
			kept = append(kept, p)
		}
	}
	tv.images = kept
}

// kittyClearPlacements drops the pictures of one screen. The caller holds the
// lock.
func (tv *TerminalView) kittyClearPlacements(alt bool) {
	if len(tv.images) == 0 {
		return
	}
	kept := make([]terminalImage, 0, len(tv.images))
	for _, p := range tv.images {
		if p.Alt != alt {
			kept = append(kept, p)
		}
	}
	tv.images = kept
}

// kittyScrollPlacements moves the pictures of the active screen along with the
// text and forgets the ones that scrolled out of the buffer. A negative n
// scrolls down. The caller holds the lock.
func (tv *TerminalView) kittyScrollPlacements(top, bottom, n int) {
	if len(tv.images) == 0 || n == 0 {
		return
	}
	kept := make([]terminalImage, 0, len(tv.images))
	for _, p := range tv.images {
		if p.Alt == tv.UseAltScreen && p.Row <= bottom && p.Row+p.Rows > top {
			p.Row -= n
			if p.Row+p.Rows <= 0 {
				// Gone off the top of the screen for good.
				continue
			}
		}
		kept = append(kept, p)
	}
	tv.images = kept
}

// kittyResizePlacements follows the pictures through a change of the terminal
// size. The rows of the main screen move by the same shift the reflow gives
// the text, so that a picture never drifts away from the line it was printed
// next to; the alternate screen is not reflowed, so its pictures stay where
// they are. Nothing is rescaled here: a placement is a rectangle of cells and
// kitty keeps it that way, and what no longer fits is clipped when it is
// drawn, so widening the window brings the whole picture back. The caller
// holds the lock.
func (tv *TerminalView) kittyResizePlacements(shift, height int) {
	if len(tv.images) == 0 {
		return
	}
	kept := make([]terminalImage, 0, len(tv.images))
	for _, p := range tv.images {
		if !p.Alt {
			p.Row += shift
		}
		// Off the top for good, or past the bottom of a screen that shrank:
		// the text of those rows is gone as well and neither comes back.
		if p.Row+p.Rows <= 0 || p.Row >= height {
			continue
		}
		kept = append(kept, p)
	}
	tv.images = kept
}

// kittyDrawPlacements paints the pictures of the active screen. The offset is
// the visual gravity shift the text is drawn with, so that a picture and the
// text around it never drift apart.
func (tv *TerminalView) kittyDrawPlacements(scr *vtui.ScreenBuf, offset int) {
	if len(tv.images) == 0 || !scr.SupportsGraphics() {
		return
	}
	left, top := tv.X1, tv.Y1
	right, bottom := tv.X1+tv.Width-1, tv.Y1+tv.Height-1

	for i := range tv.images {
		p := &tv.images[i]
		if p.Alt != tv.UseAltScreen {
			continue
		}
		row := tv.Y1 + p.Row
		if !tv.UseAltScreen {
			row += offset
		}

		ip := vtui.ImagePlacement{
			Surface: p.Surface,
			Col:     tv.X1 + p.Col,
			Row:     row,
			Cols:    p.Cols,
			Rows:    p.Rows,
			SrcX:    p.SrcX,
			SrcY:    p.SrcY,
			SrcW:    p.SrcW,
			SrcH:    p.SrcH,
			ZIndex:  p.Z,
		}
		if !kittyClipPlacement(&ip, left, top, right, bottom) {
			continue
		}
		scr.Graphics().DrawImage(p.Key, ip)
	}
}

// kittyClipPlacement trims a placement to the area of the terminal, cropping
// the source rectangle by the same fraction so that the visible part keeps
// its scale instead of being squeezed. It reports whether anything is left.
func kittyClipPlacement(p *vtui.ImagePlacement, left, top, right, bottom int) bool {
	if p.Cols <= 0 || p.Rows <= 0 || p.SrcW <= 0 || p.SrcH <= 0 {
		return false
	}
	if p.Row > bottom || p.Col > right || p.Row+p.Rows <= top || p.Col+p.Cols <= left {
		return false
	}

	if cut := top - p.Row; cut > 0 {
		px := p.SrcH * cut / p.Rows
		p.SrcY += px
		p.SrcH -= px
		p.Rows -= cut
		p.Row = top
	}
	if cut := p.Row + p.Rows - 1 - bottom; cut > 0 {
		p.SrcH -= p.SrcH * cut / p.Rows
		p.Rows -= cut
	}
	if cut := left - p.Col; cut > 0 {
		px := p.SrcW * cut / p.Cols
		p.SrcX += px
		p.SrcW -= px
		p.Cols -= cut
		p.Col = left
	}
	if cut := p.Col + p.Cols - 1 - right; cut > 0 {
		p.SrcW -= p.SrcW * cut / p.Cols
		p.Cols -= cut
	}
	return p.Cols > 0 && p.Rows > 0 && p.SrcW > 0 && p.SrcH > 0
}
