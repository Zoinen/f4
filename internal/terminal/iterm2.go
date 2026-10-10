package terminal

// Receiver for iTerm2's inline image protocol, OSC 1337 ; File=... (also spoken
// by imgcat, WezTerm's imgcat and many "show a picture" tools). The picture goes
// through the same placement list as kitty and sixel pictures, so it scrolls
// with the text, follows a resize and is clipped at the edges of the terminal
// without any of that being written a third time (f4#1685).

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/unxed/vtui"
)

// iterm2MaxPayload bounds the decoded picture, so that a stray or hostile
// sequence cannot make the terminal hold hundreds of megabytes.
const iterm2MaxPayload = 64 << 20

// iterm2Size is one of the width and height arguments of a File sequence.
type iterm2Size struct {
	kind  byte // 0 auto, 'c' cells, 'p' pixels, '%' percent of the terminal
	value int
}

func parseITerm2Size(s string) iterm2Size {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "auto" {
		return iterm2Size{}
	}
	kind := byte('c')
	switch {
	case strings.HasSuffix(s, "px"):
		kind, s = 'p', strings.TrimSuffix(s, "px")
	case strings.HasSuffix(s, "%"):
		kind, s = '%', strings.TrimSuffix(s, "%")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return iterm2Size{}
	}
	return iterm2Size{kind: kind, value: n}
}

// HandleITerm2File consumes the argument of one OSC 1337 File sequence: the
// text after "File=", that is "key=value;...:base64 data". Only inline
// pictures are drawn; a File that asks to be downloaded is ignored, since a
// terminal that saved files a program names would be handing it the disk.
func (tv *TerminalView) HandleITerm2File(arg string) {
	head, payload, ok := strings.Cut(arg, ":")
	if !ok {
		return
	}
	inline := false
	var width, height iterm2Size
	keep := true
	for _, field := range strings.Split(head, ";") {
		key, value, _ := strings.Cut(field, "=")
		switch strings.ToLower(key) {
		case "inline":
			inline = value == "1"
		case "width":
			width = parseITerm2Size(value)
		case "height":
			height = parseITerm2Size(value)
		case "preserveaspectratio":
			keep = value != "0"
		}
	}
	if !inline {
		return
	}
	if base64.StdEncoding.DecodedLen(len(payload)) > iterm2MaxPayload {
		vtui.DebugLog("ITERM2: the picture is larger than %d bytes and was not drawn", iterm2MaxPayload)
		return
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		// Senders often leave the padding off.
		data, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(payload, "="))
	}
	if err != nil || len(data) == 0 {
		return
	}
	surf, err := App.DecodeImage(data)
	if err != nil || !surf.Valid() {
		vtui.DebugLog("ITERM2: the picture was not drawn: %v", err)
		return
	}
	tv.iterm2Place(surf, width, height, keep)
}

// iterm2Cells turns the width and height arguments into the span the client
// asked for, in cells; zero is a side left to the terminal. When both sides are
// given and the aspect ratio is to be kept, the picture is fitted inside that
// box instead of being stretched over it. The caller holds the lock.
func (tv *TerminalView) iterm2Cells(surf *vtui.ImageSurface, w, h iterm2Size, keep bool, cw, ch int) (cols, rows int) {
	side := func(s iterm2Size, cell, screen int) int {
		switch s.kind {
		case 'c':
			return s.value
		case 'p':
			return (s.value + cell - 1) / cell
		case '%':
			return (s.value*screen + 99) / 100
		}
		return 0
	}
	cols, rows = side(w, cw, tv.Width), side(h, ch, tv.Height)
	if cols > 0 && rows > 0 && keep {
		// Fit inside cols x rows cells: the limiting side stays, the other
		// shrinks to the picture's own proportions.
		boxW, boxH := int64(cols)*int64(cw), int64(rows)*int64(ch)
		if int64(surf.Width)*boxH > int64(surf.Height)*boxW {
			rows = kittyCeilDiv(int64(surf.Height)*boxW, int64(surf.Width)*int64(ch))
		} else {
			cols = kittyCeilDiv(int64(surf.Width)*boxH, int64(surf.Height)*int64(cw))
		}
		if cols < 1 {
			cols = 1
		}
		if rows < 1 {
			rows = 1
		}
	}
	return cols, rows
}

func (tv *TerminalView) iterm2Place(surf *vtui.ImageSurface, w, h iterm2Size, keep bool) {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	cw, ch := tv.cellSizeUnsafe()
	col, row := tv.CursorX, tv.CursorY
	wantCols, wantRows := tv.iterm2Cells(surf, w, h, keep, cw, ch)

	p := terminalImage{
		Surface:  surf,
		Col:      col,
		Row:      row,
		SrcW:     surf.Width,
		SrcH:     surf.Height,
		WantCols: wantCols,
		WantRows: wantRows,
		Alt:      tv.UseAltScreen,
		// The picture has no id and cannot be addressed afterwards, which
		// keeps the kitty delete commands off it, as for a sixel.
		Sixel: true,
	}
	p.Cols, p.Rows = kittySpanFor(wantCols, wantRows, p.SrcW, p.SrcH, cw, ch)
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

	// Scroll until the whole picture is on screen, and leave the cursor on
	// the picture's last row: the newline a client sends after it puts the
	// next text below the picture.
	if over := row + p.Rows - 1 - tv.ScrollBottom; over > 0 {
		tv.scrollUp(tv.ScrollTop, tv.ScrollBottom, over)
		row -= over
	}
	tv.sixelMoveCursor(col, row+p.Rows-1)
}
