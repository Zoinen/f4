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

// paintFrame fills its bounds with one rune, standing in for a frame's own
// drawing. It also records what one cell held just before it painted, the way
// a screen grabber snapshots what lies below it.
type paintFrame struct {
	mockFrame
	fill           rune
	probeX, probeY int
	probed         rune
}

func (f *paintFrame) Show(scr *ScreenBuf) {
	f.probed = rune(scr.GetCell(f.probeX, f.probeY).Char)
	scr.FillRect(f.X1, f.Y1, f.X2, f.Y2, f.fill, 0)
}

// TestRenderPhase_AfterFrameShowKeepsStackOrder covers f4 issue #378: a
// decoration a host draws for one frame has to be covered by frames above it
// and be on screen by the time the next frame paints, so a frame snapshotting
// the screen in its Show (f4's screen grabber) sees it.
func TestRenderPhase_AfterFrameShowKeepsStackOrder(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 20)
	fm := &frameManager{}
	fm.Init(scr)

	below := &paintFrame{fill: 'b'}
	below.SetPosition(0, 0, 39, 19)
	fm.Push(below)
	above := &paintFrame{fill: 'a', probeX: 15, probeY: 8}
	above.Modal = true
	above.SetPosition(10, 4, 30, 12)
	fm.Push(above)

	var shown []Frame
	fm.AfterFrameShow = func(s *ScreenBuf, frame Frame) {
		shown = append(shown, frame)
		if frame == Frame(below) {
			s.Write(2, 2, StringToCharInfo("d", 0))  // outside the frame above
			s.Write(15, 8, StringToCharInfo("d", 0)) // under the frame above
		}
	}

	fm.renderPhase()

	if len(shown) != 2 || shown[0] != Frame(below) || shown[1] != Frame(above) {
		t.Fatalf("AfterFrameShow saw %d frames in order %v, want below then above", len(shown), shown)
	}
	if above.probed != 'd' {
		t.Errorf("frame above found %q under itself, want the decoration drawn for the frame below", above.probed)
	}
	if got := rune(scr.GetCell(2, 2).Char); got != 'd' {
		t.Errorf("decoration outside the frame above = %q, want 'd'", got)
	}
	if got := rune(scr.GetCell(15, 8).Char); got != 'a' {
		t.Errorf("decoration under the frame above = %q, want it covered by 'a'", got)
	}
}
