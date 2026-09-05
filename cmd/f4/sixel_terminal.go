package main

// Placement half of the sixel receiver: where in the terminal grid a decoded
// image is shown and where the text cursor is left afterwards.
//
// A sixel picture goes through the same placement list as a kitty one, so it
// scrolls with the text, follows a resize and is clipped at the edges of the
// terminal without any of that being written twice. What it does not get is an
// image id: sixel has no server side store and no way to address a picture
// after the fact, which is why the placements carry a flag that keeps the
// kitty delete commands off them.

import (
	"encoding/binary"

	"github.com/unxed/vtui"
)

// HandleSixelDCS consumes one DCS ... q sequence: its parameters and the body
// between the final q and the string terminator. attr is the character
// attribute in force, which is where the background colour comes from.
func (tv *TerminalView) HandleSixelDCS(params []int, body string, attr uint64) {
	img, err := decodeSixel(params, body, tv.sixelBackground(attr))
	if err != nil {
		vtui.DebugLog("SIXEL: the image was not drawn: %v", err)
		return
	}
	if !img.Surface.Valid() {
		return
	}
	tv.sixelPlace(img)
}

// sixelBackground resolves the colour the unpainted pixels of an opaque image
// take. The protocol calls it the current background colour, so it is the
// background of the attribute in force and not the palette's black.
func (tv *TerminalView) sixelBackground(attr uint64) [3]byte {
	var rgb uint32
	if attr&vtui.IsBgRGB != 0 {
		rgb = vtui.GetRGBBack(attr)
	} else {
		tv.mu.Lock()
		rgb = tv.Palette[vtui.GetIndexBack(attr)]
		tv.mu.Unlock()
	}
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], rgb)
	return [3]byte{encoded[1], encoded[2], encoded[3]}
}

func (tv *TerminalView) sixelPlace(img *sixelImage) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	surf := img.Surface
	cw, ch := tv.cellSizeUnsafe()

	col, row := tv.CursorX, tv.CursorY
	if tv.SixelDisplayMode {
		// DECSDM set turns sixel scrolling off: the picture goes to the
		// top left corner of the screen, nothing scrolls, and the text
		// cursor stays where it was.
		col, row = 0, 0
	}

	p := terminalImage{
		Surface: surf,
		Col:     col,
		Row:     row,
		SrcW:    surf.Width,
		SrcH:    surf.Height,
		Alt:     tv.UseAltScreen,
		Sixel:   true,
	}
	// Neither side is a promise from the client the way kitty's c and r
	// are: a sixel picture is a rectangle of pixels, and the cells it
	// covers follow from the size of a cell. That is what a zero span
	// means to kittySpanFor, and it is also what makes the picture keep
	// its size when the cell changes.
	p.Cols, p.Rows = kittySpanFor(0, 0, p.SrcW, p.SrcH, cw, ch)

	// A picture larger than the screen is cropped rather than squeezed:
	// the source rectangle loses the same fraction as the span, so what is
	// left keeps its scale, exactly as kittyClipPlacement does at the
	// edges. A real terminal draws the top of such a picture and lets the
	// rest fall off the page, and this is as close as a placement gets.
	if p.Cols > tv.Width {
		p.SrcW = p.SrcW * tv.Width / p.Cols
		p.Cols = tv.Width
	}
	if p.Rows > tv.Height {
		p.SrcH = p.SrcH * tv.Height / p.Rows
		p.Rows = tv.Height
	}
	if p.Cols <= 0 || p.Rows <= 0 || p.SrcW <= 0 || p.SrcH <= 0 {
		return
	}

	tv.kittyAddPlacement(p)

	if tv.SixelDisplayMode {
		return
	}

	// Scroll until the whole picture is on screen. The hardware gets there
	// one band at a time, as the sixel cursor walks past the bottom
	// margin; the result is the same and the picture rides up with the
	// text because kittyScrollPlacements moves it too.
	if over := row + p.Rows - 1 - tv.ScrollBottom; over > 0 {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, over)
		row -= over
	}

	// Windows Terminal leaves the text cursor at the sixel active
	// position: the column the picture started in, and the row the sixel
	// cursor reached. The consequence, which is deliberate there and which
	// this follows, is that a dump ending in a graphics new line puts the
	// next line of text below the picture while one ending in data puts it
	// on top of the picture. Without that a program could not print an
	// image on the bottom row at all, since the terminal would always
	// scroll a line away to make room for a cursor it was never asked to
	// move. A client that wants the text below the image sends a line feed
	// of its own.
	//
	// The offset is capped at the height of the placement so that a
	// picture far taller than the screen cannot ask us to scroll the
	// scrollback away a row at a time.
	off := img.EndY / ch
	if off > p.Rows {
		off = p.Rows
	}
	tv.sixelMoveCursor(col, row+off)
}

// sixelMoveCursor puts the text cursor where the sixel cursor finished,
// scrolling if that is below the margin. The caller holds the lock.
func (tv *TerminalView) sixelMoveCursor(col, row int) {
	if col < 0 {
		col = 0
	}
	if col >= tv.Width {
		col = tv.Width - 1
	}
	if row > tv.ScrollBottom {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, row-tv.ScrollBottom)
		row = tv.ScrollBottom
	}
	if row < 0 {
		row = 0
	}
	tv.CursorX = col
	tv.CursorY = row
	tv.lastCharWasCR = false
}

// SetSixelDisplayMode handles DECSDM, private mode 80. Setting it disables
// sixel scrolling and resetting it enables it, which is the way round the
// hardware behaved and the way round xterm settled on after years of doing
// the opposite. Reset is the default.
func (tv *TerminalView) SetSixelDisplayMode(on bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.SixelDisplayMode = on
}
