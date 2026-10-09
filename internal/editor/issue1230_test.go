package editor

import (
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestIssue1230EnterDropsColorerCacheBelowTheCursor is the regression test for
// issue #1230: with the cursor at the start of a line, pressing Enter moved
// the text down but left the Colorer's cached colours where they were, so the
// colours no longer matched the lines they were drawn on. The cache is keyed
// by line number, so every edit that adds or removes a line has to drop it
// from the edited line on, as typing, Tab, Delete and Backspace already do.
func TestIssue1230EnterDropsColorerCacheBelowTheCursor(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	ev := NewEditorView(piecetable.New([]byte("first\nsecond\nthird\nfourth")), nil, "test.txt")
	defer ev.Close()

	ch := &ColorerHighlighter{}
	for line := 0; line < 4; line++ {
		ch.storeAttrs(line, []uint64{uint64(line + 1)}, 0, nil)
	}
	ev.Highlighter = ch
	ev.CursorLine, ev.CursorPos = 1, 0

	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if got := ev.Pt.String(); got != "first\n\nsecond\nthird\nfourth" {
		t.Fatalf("text after Enter = %q", got)
	}
	if _, ok := ch.attrCache[0]; !ok {
		t.Error("colours of line 0, above the edit, were dropped")
	}
	for line := 1; line < 4; line++ {
		if attrs, ok := ch.attrCache[line]; ok {
			t.Errorf("line %d still has the colours %v cached from before the text moved", line, attrs)
		}
	}
}
