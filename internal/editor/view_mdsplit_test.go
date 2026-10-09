package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// A left click on the split preview's text puts the editor's cursor at the
// matching line of the text (f4#1625), by the row-to-line map.
func TestMarkdownSplitClickMovesCursor(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	var doc strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&doc, "paragraph %d\n\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(doc.String())), nil, "notes.md")
	defer ev.Close()
	ev.ResizeConsole(80, 25)
	ev.ToggleMarkdownSplit()
	view := ev.mdSplit.view
	if view == nil {
		t.Fatal("the preview was not built")
	}
	_, ty1, tx2, _ := view.TextArea()
	tx1, _, _, _ := view.TextArea()

	click := func(x, y int, button uint32) bool {
		return ev.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true, ButtonState: button,
			MouseX: int16(x), MouseY: int16(y), //nolint:gosec // bounded test screen positions
		})
	}

	// Paragraph k is line 2k of the text and row 2k of the preview.
	const row = 8
	if !click(tx1+1, ty1+row, vtinput.FromLeft1stButtonPressed) {
		t.Fatal("a left click on the preview text was not handled")
	}
	// The jump is a goto: the cursor lands on the line at once or, while the
	// line index is still being built, the line is the pending target.
	if want := row; ev.CursorLine != want && ev.TargetLine != want {
		t.Errorf("cursor line %d, target %d after the click, want %d", ev.CursorLine, ev.TargetLine, want)
	}

	ev.CursorLine, ev.TargetLine = 0, -1
	if click(tx2, ty1+row, vtinput.RightmostButtonPressed) && (ev.CursorLine != 0 || ev.TargetLine != -1) {
		t.Error("a right click moved the cursor")
	}
}

// The preview follows the cursor to the exact row made from its line.
func TestMarkdownSplitScrollFollowsCursor(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	var doc strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&doc, "paragraph %d\n\n", i)
	}
	ev := NewEditorView(piecetable.New([]byte(doc.String())), nil, "notes.md")
	defer ev.Close()
	ev.ResizeConsole(80, 25)
	ev.ToggleMarkdownSplit()
	view := ev.mdSplit.view
	if got := view.ScrollTop(); got != 0 {
		t.Fatalf("scroll with the cursor on the first line = %d", got)
	}
	ev.CursorLine = 80 // "paragraph 40"
	ev.syncMarkdownSplitScroll()
	if got := view.ScrollTop(); got != 80 {
		t.Fatalf("scroll with the cursor on line 80 = %d, want row 80", got)
	}
	// And back: a click on the first visible row is that same line.
	_, ty1, _, _ := view.TextArea()
	tx1, _, _, _ := view.TextArea()
	ev.CursorLine, ev.TargetLine = 0, -1
	ev.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: int16(tx1 + 1), MouseY: int16(ty1), //nolint:gosec // bounded test screen positions
	})
	if ev.CursorLine != 80 && ev.TargetLine != 80 {
		t.Fatalf("click on the first visible row: cursor %d, target %d, want line 80", ev.CursorLine, ev.TargetLine)
	}
}
