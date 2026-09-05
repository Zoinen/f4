package main

import (
	"strings"
	"testing"
)

// sixelEnv wires a terminal view to a parser and a fake pty, the way the
// receiver sees the world in production.
type sixelEnv struct {
	tv  *TerminalView
	p   *AnsiParser
	pty *mockPty
}

func newSixelEnv(t *testing.T) *sixelEnv {
	t.Helper()
	tv := NewTerminalView(80, 24)
	pty := &mockPty{}
	tv.pty = pty
	// A known cell keeps the arithmetic in the tests explicit.
	tv.cellW, tv.cellH = 10, 20
	return &sixelEnv{tv: tv, p: NewAnsiParser(tv, pty), pty: pty}
}

// send feeds one whole sixel sequence to the parser.
func (e *sixelEnv) send(params, body string) {
	e.p.Process([]byte("\x1bP" + params + "q" + body + "\x1b\\"))
}

// sixelBody paints a solid rectangle of w by h pixels, in whole bands.
func sixelBody(w, h int) string {
	var sb strings.Builder
	sb.WriteString("\"1;1;")
	sb.WriteString(itoa(w))
	sb.WriteByte(';')
	sb.WriteString(itoa(h))
	sb.WriteString("#0;2;100;0;0")
	for band := 0; band < (h+5)/6; band++ {
		if band > 0 {
			sb.WriteByte('-')
		}
		sb.WriteString("!")
		sb.WriteString(itoa(w))
		sb.WriteString("~")
	}
	return sb.String()
}

// sixelLineText reads one row of the terminal grid as a string.
func sixelLineText(tv *TerminalView, row int) string {
	var sb strings.Builder
	for _, c := range tv.Lines[row] {
		sb.WriteRune(testRune(c.Char))
	}
	return sb.String()
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [12]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func TestSixelPlacedAtCursor(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(4, 6)
	// 30x40 pixels on a 10x20 cell is three columns by two rows.
	e.send("0;1;0", sixelBody(30, 40))

	if len(e.tv.images) != 1 {
		t.Fatalf("expected one placement, got %d", len(e.tv.images))
	}
	p := e.tv.images[0]
	if p.Col != 4 || p.Row != 6 {
		t.Errorf("position: got %d,%d, want 4,6", p.Col, p.Row)
	}
	if p.Cols != 3 || p.Rows != 2 {
		t.Errorf("span: got %dx%d cells, want 3x2", p.Cols, p.Rows)
	}
	if !p.Sixel {
		t.Error("the placement must be marked as a sixel one")
	}
	if p.ImageID != 0 {
		t.Error("a sixel picture has no image id")
	}
}

// Windows Terminal leaves the text cursor at the sixel active position: the
// column the picture started in, and the row the sixel cursor reached. Data
// alone leaves it on the picture.
func TestSixelCursorStaysOnImageWithoutTrailingNewLine(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(4, 6)
	e.send("0;1;0", `"1;1;10;20#0;2;100;0;0!10~`)

	if e.tv.CursorX != 4 || e.tv.CursorY != 6 {
		t.Errorf("cursor: got %d,%d, want 4,6", e.tv.CursorX, e.tv.CursorY)
	}
}

// A dump that ends in graphics new lines moves the sixel cursor past the
// picture, and the text cursor follows it there.
func TestSixelCursorFollowsTrailingNewLines(t *testing.T) {
	// Ten bands of six pixel rows each: sixty pixel rows, which is three
	// cell rows on a cell twenty high.
	body := func(trailing string) string {
		var sb strings.Builder
		sb.WriteString(`"1;1;10;60#0;2;100;0;0`)
		for i := 0; i < 10; i++ {
			if i > 0 {
				sb.WriteByte('-')
			}
			sb.WriteString("!10~")
		}
		sb.WriteString(trailing)
		return sb.String()
	}

	// Data alone leaves the sixel position in the last band, pixel row 54,
	// which is the third and last cell row of the picture.
	e := newSixelEnv(t)
	e.tv.SetCursor(4, 6)
	e.send("7;1;0", body(""))
	if e.tv.CursorX != 4 || e.tv.CursorY != 8 {
		t.Errorf("cursor after data: got %d,%d, want 4,8", e.tv.CursorX, e.tv.CursorY)
	}

	// The graphics new line that the hardware sends after a dump takes it
	// to pixel row 60, the row below the picture.
	e = newSixelEnv(t)
	e.tv.SetCursor(4, 6)
	e.send("7;1;0", body("-"))
	if e.tv.CursorX != 4 || e.tv.CursorY != 9 {
		t.Errorf("cursor after a graphics new line: got %d,%d, want 4,9", e.tv.CursorX, e.tv.CursorY)
	}
}

func TestSixelScrollsToFitAtBottom(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(0, 23)
	// Three rows of picture printed on the last row of a 24 row screen.
	e.send("0;1;0", sixelBody(10, 60))

	p := e.tv.images[0]
	if p.Row != 21 {
		t.Errorf("the screen must scroll until the picture fits: row %d, want 21", p.Row)
	}
	if p.Row+p.Rows-1 != 23 {
		t.Errorf("the bottom of the picture must sit on the last row: %d", p.Row+p.Rows-1)
	}
}

// DECSDM set disables sixel scrolling: the picture goes to the top left and
// the text cursor does not move.
func TestSixelDisplayModeAnchorsTopLeft(t *testing.T) {
	e := newSixelEnv(t)
	e.p.Process([]byte("\x1b[?80h"))
	if !e.tv.SixelDisplayMode {
		t.Fatal("CSI ? 80 h must set sixel display mode")
	}
	e.tv.SetCursor(5, 7)
	e.send("0;1;0", sixelBody(30, 40))

	p := e.tv.images[0]
	if p.Col != 0 || p.Row != 0 {
		t.Errorf("position: got %d,%d, want 0,0", p.Col, p.Row)
	}
	if e.tv.CursorX != 5 || e.tv.CursorY != 7 {
		t.Errorf("the cursor must not move: got %d,%d", e.tv.CursorX, e.tv.CursorY)
	}

	e.p.Process([]byte("\x1b[?80l"))
	if e.tv.SixelDisplayMode {
		t.Error("CSI ? 80 l must reset sixel display mode")
	}
}

func TestSixelSurvivesChunkedDelivery(t *testing.T) {
	e := newSixelEnv(t)
	seq := "\x1bP0;1;0q" + sixelBody(30, 40) + "\x1b\\"
	for i := 0; i < len(seq); i++ {
		e.p.Process([]byte{seq[i]})
	}
	if len(e.tv.images) != 1 {
		t.Fatalf("a sequence split byte by byte must still arrive: %d placements", len(e.tv.images))
	}
}

// A device control string the terminal does not understand used to spill onto
// the screen, because ESC P was swallowed and the body was not.
func TestSixelParserSwallowsUnknownDCS(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("\x1bP$q m\x1b\\hello"))
	if len(e.tv.images) != 0 {
		t.Error("a DECRQSS reply is not a picture")
	}
	if got := strings.TrimSpace(sixelLineText(e.tv, 0)); got != "hello" {
		t.Errorf("only the text after the string terminator may reach the screen: %q", got)
	}
}

func TestSixelDeviceAttributesAnnounceSixel(t *testing.T) {
	e := newSixelEnv(t)
	// The trailing carriage return keeps the heuristic that hides the
	// background cd command from holding the final c back: it looks like
	// the start of one.
	e.p.Process([]byte("\x1b[c\r"))
	got := e.pty.String()
	if !strings.Contains(got, ";4") {
		t.Errorf("the primary device attributes must contain 4: %q", got)
	}
}

func TestSixelGraphicsAttributesQuery(t *testing.T) {
	e := newSixelEnv(t)
	e.p.Process([]byte("\x1b[?1;1S"))
	if got := e.pty.String(); got != "\x1b[?1;0;256S" {
		t.Errorf("colour registers: got %q", got)
	}

	e = newSixelEnv(t)
	e.p.Process([]byte("\x1b[?2;1S"))
	if got := e.pty.String(); got != "\x1b[?2;0;800;480S" {
		t.Errorf("raster geometry: got %q", got)
	}

	e = newSixelEnv(t)
	e.p.Process([]byte("\x1b[?3;1S"))
	if got := e.pty.String(); got != "\x1b[?3;1S" {
		t.Errorf("ReGIS is not supported and must say so: got %q", got)
	}
}

// CSI S without the question mark is still a scroll.
func TestSixelGraphicsAttributesDoesNotEatScrollUp(t *testing.T) {
	e := newSixelEnv(t)
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("hello\r\n\x1b[1S"))
	if got := e.pty.String(); got != "" {
		t.Errorf("a plain CSI S must not answer anything: %q", got)
	}
	if got := strings.TrimSpace(sixelLineText(e.tv, 0)); got != "" {
		t.Errorf("the text must have scrolled up: first row %q", got)
	}
}

// A kitty delete addressed by image id must leave sixel placements alone:
// they carry no id, so without the guard every one of them would answer to
// the zero the command defaults to.
func TestSixelPlacementSurvivesKittyDeleteByID(t *testing.T) {
	e := newSixelEnv(t)
	e.send("0;1;0", sixelBody(30, 40))
	if len(e.tv.images) != 1 {
		t.Fatalf("setup: %d placements", len(e.tv.images))
	}
	e.p.Process([]byte("\x1b_Ga=d,d=I\x1b\\"))
	if len(e.tv.images) != 1 {
		t.Error("a sixel picture must not be deleted by image id")
	}
	// Deleting everything visible does take it.
	e.p.Process([]byte("\x1b_Ga=d,d=a\x1b\\"))
	if len(e.tv.images) != 0 {
		t.Error("a=d,d=a must clear the screen of pictures")
	}
}

func TestSixelClearedByEraseDisplay(t *testing.T) {
	e := newSixelEnv(t)
	e.send("0;1;0", sixelBody(30, 40))
	e.p.Process([]byte("\x1b[2J"))
	if len(e.tv.images) != 0 {
		t.Error("erasing the display must take the pictures with it")
	}
}
