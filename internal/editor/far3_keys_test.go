package editor

import (
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

func newFAR3Editor(t *testing.T, text string) *EditorView {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.txt")
	ev.SetPosition(0, 0, 80, 12)
	t.Cleanup(ev.Close)
	return ev
}

func TestFAR3WordDeletion(t *testing.T) {
	forward := newFAR3Editor(t, "one  two three")
	forward.CursorPos = len("one")
	forward.DeleteSpacersForward()
	if got, want := forward.GetText(), "onetwo three"; got != want {
		t.Fatalf("forward deletion = %q, want %q", got, want)
	}

	backward := newFAR3Editor(t, "one  two three")
	backward.CursorPos = len("one  two")
	backward.DeleteWordBackward()
	if got, want := backward.GetText(), "one three"; got != want {
		t.Fatalf("backward deletion = %q, want %q", got, want)
	}
}

func TestFAR3BlockTransferAndIndent(t *testing.T) {
	ev := newFAR3Editor(t, "one two three")
	ev.SelActive = true
	ev.SelAnchorOffset = 0
	ev.CursorPos = 3
	ev.CopySelectionToCursor()
	if got, want := ev.GetText(), "oneone two three"; got != want {
		t.Fatalf("copied block = %q, want %q", got, want)
	}

	indent := newFAR3Editor(t, "one")
	indent.CursorPos = 1
	indent.ShiftCurrentOrSelectedLines(false)
	if got, want := indent.GetText(), "\tone"; got != want {
		t.Fatalf("indented line = %q, want %q", got, want)
	}
	if indent.CursorPos != 2 {
		t.Fatalf("cursor after indent = %d, want 2", indent.CursorPos)
	}
}

func TestFAR3Bookmarks(t *testing.T) {
	ev := newFAR3Editor(t, "first\nsecond")
	ev.CursorLine = 1
	ev.CursorPos = 2
	ev.SetEditorBookmark(3, true)
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.SetEditorBookmark(3, false)
	if ev.CursorLine != 1 || ev.CursorPos != 2 {
		t.Fatalf("bookmark restored %d:%d, want 1:2", ev.CursorLine, ev.CursorPos)
	}
}
