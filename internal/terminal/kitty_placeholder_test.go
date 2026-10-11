package terminal

import "testing"

// phMark returns the combining mark that stands for n.
func phMark(n int) string { return string(kittyDiacritics[n]) }

// phCell is one placeholder with its row and column marks.
func phCell(row, col int) string {
	return string(rune(kittyPlaceholderRune)) + phMark(row) + phMark(col)
}

// phShow draws the terminal on the alternate screen, where the coordinates are
// plain, and returns the pictures it put in the graphics layer.
func phShow(e *kittyEnv) []phPicture {
	scr := kittyGraphicsScreen(80, 24)
	e.tv.SetPosition(0, 0, 79, 23)
	e.tv.Show(scr)
	list, _ := scr.Graphics().Snapshot(nil)
	out := make([]phPicture, 0, len(list))
	for _, p := range list {
		out = append(out, phPicture{p.Col, p.Row, p.Cols, p.Rows, p.SrcX, p.SrcY, p.SrcW, p.SrcH})
	}
	return out
}

type phPicture struct{ col, row, cols, rows, sx, sy, sw, sh int }

func phEnv(t *testing.T) *kittyEnv {
	t.Helper()
	e := newKittyEnv(t)
	e.p.Process([]byte("\x1b[?1049h"))
	e.tv.CellW, e.tv.CellH = 10, 20
	return e
}

func TestPlaceholdersDrawAVirtualPlacement(t *testing.T) {
	e := phEnv(t)
	// A 20x20 pixel picture in a 2x1 cell area (20x20 pixels at 10x20 cells).
	kittySendImage(e, "a=T,U=1,i=7,f=32,s=20,v=20,c=2,r=1,q=2", 20, 20)
	if len(e.tv.Images) != 0 {
		t.Fatal("a virtual placement must not show anything by itself")
	}
	if e.tv.CursorX != 0 || e.tv.CursorY != 0 {
		t.Errorf("a virtual placement moved the cursor to %d,%d", e.tv.CursorX, e.tv.CursorY)
	}

	// Foreground colour 7 carries the image id; each cell is a placeholder
	// followed by its row and column.
	e.tv.SetCursor(5, 2)
	e.p.Process([]byte("\x1b[38;5;7m" + phCell(0, 0) + phCell(0, 1) + "\x1b[0m"))
	if e.tv.CursorX != 7 || e.tv.CursorY != 2 {
		t.Fatalf("two placeholders left the cursor at %d,%d, want 7,2 (the marks take no cell)", e.tv.CursorX, e.tv.CursorY)
	}

	pics := phShow(e)
	if len(pics) != 1 {
		t.Fatalf("got %d pictures, want the two cells merged into one: %+v", len(pics), pics)
	}
	p := pics[0]
	if p.col != 5 || p.row != 2 || p.cols != 2 || p.rows != 1 {
		t.Errorf("drawn at %+v, want columns 5..6 of row 2", p)
	}
	if p.sx != 0 || p.sy != 0 || p.sw != 20 || p.sh != 20 {
		t.Errorf("source %+v, want the whole 20x20 picture", p)
	}
}

func TestPlaceholdersEachCellShowsItsOwnPiece(t *testing.T) {
	e := phEnv(t)
	// 2x2 cells of 10x20 pixels: a 20x40 picture, cut in four.
	kittySendImage(e, "a=T,U=1,i=9,f=32,s=20,v=40,c=2,r=2,q=2", 20, 40)
	e.tv.SetCursor(0, 0)
	// Only the first cell of each row says where it is; the rest is inherited
	// from the cell on the left, as the protocol allows.
	e.p.Process([]byte("\x1b[38;5;9m" + phCell(0, 0) + string(rune(kittyPlaceholderRune)) + "\r\n" +
		phCell(1, 0) + string(rune(kittyPlaceholderRune))))

	pics := phShow(e)
	if len(pics) != 2 {
		t.Fatalf("got %d pictures, want one per row: %+v", len(pics), pics)
	}
	top, bottom := pics[0], pics[1]
	if top.row > bottom.row {
		top, bottom = bottom, top
	}
	if top.sy != 0 || top.sh != 20 || bottom.sy != 20 || bottom.sh != 20 {
		t.Errorf("rows cut wrong: top %+v, bottom %+v", top, bottom)
	}
	if top.cols != 2 || bottom.cols != 2 {
		t.Errorf("an inherited cell should join its neighbour: %+v %+v", top, bottom)
	}
}

func TestPlaceholdersUseTheTopByteOfTheID(t *testing.T) {
	e := phEnv(t)
	// Image id 0x01000005: the low 24 bits in the colour (true colour), the
	// top byte as the third mark.
	kittySendImage(e, "a=T,U=1,i=16777221,f=32,s=10,v=20,c=1,r=1,q=2", 10, 20)
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("\x1b[38;2;0;0;5m" + string(rune(kittyPlaceholderRune)) + phMark(0) + phMark(0) + phMark(1) + "\x1b[0m"))
	if pics := phShow(e); len(pics) != 1 {
		t.Fatalf("the id with its top byte was not resolved: %+v", pics)
	}
	// Without the third mark it is another image, and there is none.
	e.tv.SetCursor(0, 5)
	e.p.Process([]byte("\x1b[38;2;0;0;5m" + phCell(0, 0) + "\x1b[0m"))
	if pics := phShow(e); len(pics) != 1 {
		t.Errorf("a placeholder for an unknown id drew something: %+v", pics)
	}
}

func TestPlaceholdersGoWithTheirText(t *testing.T) {
	e := phEnv(t)
	kittySendImage(e, "a=T,U=1,i=7,f=32,s=10,v=20,c=1,r=1,q=2", 10, 20)
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("\x1b[38;5;7m" + phCell(0, 0) + "\x1b[0m"))
	if len(phShow(e)) != 1 {
		t.Fatal("no picture for a placeholder")
	}

	// Overwriting the cell removes the picture, like any text.
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("x"))
	if pics := phShow(e); len(pics) != 0 {
		t.Errorf("an overwritten placeholder still shows %+v", pics)
	}

	// Deleting the image drops its virtual placement.
	e.tv.SetCursor(0, 1)
	e.p.Process([]byte("\x1b[38;5;7m" + phCell(0, 0) + "\x1b[0m"))
	if len(phShow(e)) != 1 {
		t.Fatal("no picture for the second placeholder")
	}
	e.send("a=d,d=I,i=7", "")
	if pics := phShow(e); len(pics) != 0 {
		t.Errorf("a deleted image is still drawn: %+v", pics)
	}
}

func TestPlaceholdersFitThePictureInsideTheArea(t *testing.T) {
	e := phEnv(t)
	// A wide 40x10 picture in a 2x2 cell area (20x40 pixels) is fitted by
	// width and centred: it fills 20x5 pixels in the middle of the area, so
	// only the middle of the two rows has anything in it.
	kittySendImage(e, "a=T,U=1,i=3,f=32,s=40,v=10,c=2,r=2,q=2", 40, 10)
	e.tv.SetCursor(0, 0)
	e.p.Process([]byte("\x1b[38;5;3m" + phCell(0, 0) + phCell(0, 1) + "\r\n" + phCell(1, 0) + phCell(1, 1)))
	pics := phShow(e)
	if len(pics) == 0 {
		t.Fatal("nothing was drawn")
	}
	for _, p := range pics {
		if p.sw > 40 || p.sh > 10 || p.sx < 0 || p.sy < 0 {
			t.Errorf("source rectangle outside the picture: %+v", p)
		}
	}
}

func TestPlaceholderMarksTable(t *testing.T) {
	if len(kittyDiacritics) != 297 {
		t.Fatalf("the diacritics table has %d entries, want the protocol's 297", len(kittyDiacritics))
	}
	for i, r := range map[int]rune{0: 0x0305, 1: 0x030D, 2: 0x030E} {
		if kittyDiacritics[i] != r {
			t.Errorf("mark %d is U+%04X, want U+%04X", i, kittyDiacritics[i], r)
		}
	}
	seen := map[rune]bool{}
	for _, r := range kittyDiacritics {
		if seen[r] {
			t.Errorf("U+%04X is listed twice", r)
		}
		seen[r] = true
	}
	// A mark that is not in the table is an ordinary character.
	e := phEnv(t)
	e.p.Process([]byte(string(rune(kittyPlaceholderRune)) + "\u0301"))
	if e.tv.CursorX != 2 {
		t.Errorf("a combining mark outside the table must take a cell, cursor at %d", e.tv.CursorX)
	}
}
