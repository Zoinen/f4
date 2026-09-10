package editor

import (
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestEditorView_PendingRestoreSurvivesAnIdleKey covers the FISH+ report that
// the editor opened at the top of the file instead of where it was left. The
// background index needs seconds to reach a position saved deep in a remote
// file, and a key pressed while waiting used to cancel the restore whatever the
// key was. Up at the top of a file moves nothing, so cancelling on it left the
// user at line 0 with the saved position quietly discarded.
func TestEditorView_PendingRestoreSurvivesAnIdleKey(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	Pt := piecetable.New([]byte("line1\nline2\nline3\n"))
	ev := NewEditorView(Pt, nil, "restore.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 79, 24)

	ev.TargetLine = 2
	ev.TargetPos = 0
	ev.TargetTopRow = 0
	ev.TargetLeft = 0

	ev.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_UP,
	})

	if ev.TargetLine != 2 {
		t.Fatalf("Up at the top of the file dropped the pending restore, targetLine = %d", ev.TargetLine)
	}
	if ev.CursorLine != 0 || ev.CursorPos != 0 {
		t.Fatalf("Up at the top of the file moved the cursor to %d:%d", ev.CursorLine, ev.CursorPos)
	}
}

// TestEditorView_PendingRestoreYieldsToTheUser is the other half of the rule: a
// key that really moves the cursor is the user taking over, and the restore
// must not yank the cursor away from where they went.
func TestEditorView_PendingRestoreYieldsToTheUser(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	Pt := piecetable.New([]byte("line1\nline2\nline3\n"))
	ev := NewEditorView(Pt, nil, "restore.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 79, 24)

	ev.TargetLine = 2
	ev.TargetPos = 0
	ev.TargetTopRow = 0
	ev.TargetLeft = 0

	ev.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_DOWN,
	})

	if ev.TargetLine != -1 {
		t.Fatal("moving the cursor did not cancel the pending restore")
	}
	if ev.CursorLine != 1 {
		t.Fatalf("cursor at line %d, want line 1", ev.CursorLine)
	}
}

// TestEditorView_PendingRestoreSurvivesTyping makes sure the editing path still
// takes over, since an edit invalidates the saved position outright.
func TestEditorView_PendingRestoreCancelledByTyping(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	Pt := piecetable.New([]byte("line1\nline2\nline3\n"))
	ev := NewEditorView(Pt, nil, "restore.txt")
	defer ev.Close()
	ev.SetPosition(0, 0, 79, 24)

	ev.TargetLine = 2
	ev.TargetPos = 0
	ev.TargetTopRow = 0
	ev.TargetLeft = 0

	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'x'})

	if ev.TargetLine != -1 {
		t.Fatal("typing did not cancel the pending restore")
	}
}
