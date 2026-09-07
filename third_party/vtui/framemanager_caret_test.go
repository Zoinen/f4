package vtui

import "testing"

// caretFrame is a frame that unconditionally claims the caret while it is
// drawn, the way EditorView and the panels command line do: they set the
// screen cursor from their own state on every render, with no idea whether
// something has been pushed on top of them.
type caretFrame struct {
	mockFrame
	caretX, caretY int
	claimCaret     bool
}

func (f *caretFrame) Show(scr *ScreenBuf) {
	if f.claimCaret {
		scr.SetCursorPos(f.caretX, f.caretY)
		scr.SetCursorVisible(true)
	}
}

// TestRenderPhase_OnlyTopFrameOwnsCaret covers f4 issue #518: a dialog
// pushed over the editor, with focus on a control that has no caret of its
// own (a checkbox, a button, a DropdownOnly combobox), used to leave the
// editor's caret painted in the text underneath the dialog.
func TestRenderPhase_OnlyTopFrameOwnsCaret(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 20)
	fm := &frameManager{}
	fm.Init(scr)

	below := &caretFrame{caretX: 3, caretY: 5, claimCaret: true}
	below.SetPosition(0, 0, 39, 19)
	fm.Push(below)

	// Alone on the stack, the frame keeps its caret.
	fm.renderPhase()
	if _, _, visible, _ := scr.GetCursorStateForTesting(); !visible {
		t.Fatal("topmost frame lost its caret")
	}

	// A dialog on top that claims no caret must not leave the one below
	// showing through it.
	dialog := &caretFrame{claimCaret: false}
	dialog.Modal = true
	dialog.SetPosition(10, 4, 30, 12)
	fm.Push(dialog)

	fm.renderPhase()
	if _, _, visible, _ := scr.GetCursorStateForTesting(); visible {
		t.Error("caret from the frame below stayed visible under the top frame")
	}

	// A dialog that does claim a caret keeps its own, at its own position.
	dialog.claimCaret = true
	dialog.caretX, dialog.caretY = 15, 6

	fm.renderPhase()
	x, y, visible, _ := scr.GetCursorStateForTesting()
	if !visible {
		t.Error("top frame's own caret was discarded")
	}
	if x != 15 || y != 6 {
		t.Errorf("caret at (%d,%d), want the top frame's (15,6)", x, y)
	}
}
