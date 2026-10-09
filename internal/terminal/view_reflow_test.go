package terminal

import (
	"strings"
	"testing"
)

func putText(tv *TerminalView, s string) {
	for _, r := range s {
		tv.PutChar(r, DefaultTermAttr)
	}
}

// screenRows returns the viewport rows as text, trailing blanks trimmed.
func screenRows(tv *TerminalView) []string {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	rows := make([]string, len(tv.Lines))
	for y, row := range tv.Lines {
		rows[y] = strings.TrimRight(CellsText(row), " ")
	}
	return rows
}

func newReflowView(w, h int) *TerminalView {
	tv := NewTerminalView(w, h)
	tv.SetReflow(true)
	return tv
}

func TestReflowKeepsTheLogAcrossWidths(t *testing.T) {
	tv := newReflowView(20, 6)
	long := strings.Repeat("0123456789", 7) + "xyz" // 73 columns
	putText(tv, "first\r\n"+long+"\r\n"+strings.Repeat("=", 20)+"\r\n"+strings.Repeat("=", 20)+"\r\n$ ")
	want := string(tv.GetAllLogBytes())

	for _, w := range []int{7, 13, 1, 40, 80, 19, 21, 20} {
		tv.Resize(w, 6)
		if got := string(tv.GetAllLogBytes()); got != want {
			t.Fatalf("after resize to %d the log changed:\n got %q\nwant %q", w, got, want)
		}
	}
	if !strings.Contains(want, "\n"+long+"\n") {
		t.Fatalf("the long line is not one line in the log: %q", want)
	}
}

func TestReflowRewrapsTheScreenAndCarriesTheCursor(t *testing.T) {
	tv := newReflowView(20, 4)
	putText(tv, "abcdefghijklmnopqrstuvwxyz") // 26 columns: wraps once at 20
	if tv.CursorY != 3 || tv.CursorX != 6 {
		t.Fatalf("cursor before resize = (%d,%d), want (6,3)", tv.CursorX, tv.CursorY)
	}

	tv.Resize(10, 4)
	rows := screenRows(tv)
	if got, want := strings.Join(rows[1:], "|"), "abcdefghij|klmnopqrst|uvwxyz"; got != want {
		t.Fatalf("rows at width 10 = %q, want %q", got, want)
	}
	if tv.CursorY != 3 || tv.CursorX != 6 {
		t.Fatalf("cursor at width 10 = (%d,%d), want (6,3): after the z", tv.CursorX, tv.CursorY)
	}
	if !tv.WrapFlags[1] || !tv.WrapFlags[2] || tv.WrapFlags[3] {
		t.Fatalf("wrap flags at width 10 = %v", tv.WrapFlags)
	}

	putText(tv, "!")
	tv.Resize(40, 4)
	if got := screenRows(tv)[3]; got != "abcdefghijklmnopqrstuvwxyz!" {
		t.Fatalf("row at width 40 = %q", got)
	}
	if tv.CursorY != 3 || tv.CursorX != 27 {
		t.Fatalf("cursor at width 40 = (%d,%d), want (27,3)", tv.CursorX, tv.CursorY)
	}
}

func TestReflowNeverJoinsLinesTheApplicationEnded(t *testing.T) {
	tv := newReflowView(10, 5)
	// Two lines of exactly the window's width, each ended by the stream.
	putText(tv, "0123456789\r\nabcdefghij\r\n")
	tv.Resize(30, 5)
	rows := screenRows(tv)
	joined := strings.Join(rows, "|")
	if !strings.Contains(joined, "0123456789|abcdefghij") {
		t.Fatalf("lines of exactly the old width were joined: %q", joined)
	}
	if got := string(tv.GetAllLogBytes()); !strings.Contains(got, "0123456789\nabcdefghij\n") {
		t.Fatalf("log = %q", got)
	}
}

func TestReflowPullsHistoryBackWhenWidening(t *testing.T) {
	tv := newReflowView(10, 3)
	for i := 0; i < 5; i++ {
		putText(tv, strings.Repeat(string(rune('a'+i)), 15)+"\r\n")
	}
	putText(tv, "$")
	want := string(tv.GetAllLogBytes())

	tv.Resize(5, 3)
	tv.Resize(20, 3)
	rows := screenRows(tv)
	if rows[0] != strings.Repeat("d", 15) || rows[1] != strings.Repeat("e", 15) || rows[2] != "$" {
		t.Fatalf("screen at width 20 = %q, want the last two lines pulled back above the prompt", rows)
	}
	if got := string(tv.GetAllLogBytes()); got != want {
		t.Fatalf("log changed:\n got %q\nwant %q", got, want)
	}
}

func TestWideCharacterAtTheEdgeWrapsWithoutInventingASpace(t *testing.T) {
	tv := newReflowView(5, 3)
	putText(tv, "abcd世z")
	if got := string(tv.GetAllLogBytes()); !strings.HasSuffix(got, "abcd世z") {
		t.Fatalf("log = %q, want the wide character kept and no space added", got)
	}
	for _, w := range []int{6, 5, 3, 7} {
		tv.Resize(w, 3)
		if got := string(tv.GetAllLogBytes()); !strings.HasSuffix(got, "abcd世z") {
			t.Fatalf("after resize to %d log = %q", w, got)
		}
	}
}

func TestResizeWithoutReflowKeepsRowsAsTheyAre(t *testing.T) {
	tv := NewTerminalView(20, 3)
	putText(tv, strings.Repeat("x", 30))
	before := screenRows(tv)
	tv.Resize(10, 3)
	after := screenRows(tv)
	if strings.Join(after, "|") != strings.Join(before, "|") {
		t.Fatalf("rows changed without reflow: %q -> %q", before, after)
	}
}
