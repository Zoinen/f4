package envman

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestManagerInitialSizeShrinksThroughFrameManager(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{{Kind: KindProfile, Name: "one", Enabled: true}},
	})
	window := controller.dialog
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 60)
	x1, y1, x2, y2 := window.GetPosition()
	send := func(x, y int, down bool, flags uint32) {
		t.Helper()
		buttons := uint32(0)
		if down {
			buttons = vtinput.FromLeft1stButtonPressed
		}
		vtui.FrameManager.PostEvent(vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: down, ButtonState: buttons,
			MouseEventFlags: flags, MouseX: managerMouseCoordinate(x), MouseY: managerMouseCoordinate(y),
		})
		vtui.FrameManager.Step(0)
		window.Show(screen)
	}
	send(x2, y2, true, 0)
	for step := 1; step <= 6; step++ {
		send(x2-step, y2-step*2, true, vtinput.MouseMoved)
		_, _, right, bottom := window.GetPosition()
		if right != x2-step || bottom != y2-step*2 {
			t.Fatalf("initial held shrink step %d ignored: right/bottom=%d/%d, want %d/%d (minimum=%dx%d)",
				step, right, bottom, x2-step, y2-step*2, window.MinW, window.MinH)
		}
	}
	// Reach the compact two-pane minimum, release, then start another drag.
	send(x1+77, y1+22, true, vtinput.MouseMoved)
	send(x1+77, y1+22, false, 0)
	if window.IsMouseCaptured() {
		t.Fatal("release retained capture")
	}
	for _, child := range window.GetChildren() {
		if !child.IsVisible() {
			continue
		}
		left, top, right, bottom := child.GetPosition()
		if left <= x1 || top <= y1 || right >= x1+77 || bottom >= y1+22 {
			t.Errorf("child %T escapes compact dialog: (%d,%d)-(%d,%d)", child, left, top, right, bottom)
		}
	}
	send(x1+77, y1+22, true, 0)
	send(x2, y2, true, vtinput.MouseMoved)
	send(x2, y2, false, 0)
	_, _, right, bottom := window.GetPosition()
	if right != x2 || bottom != y2 {
		t.Fatalf("second drag failed to restore original size: %d/%d, want %d/%d", right, bottom, x2, y2)
	}
}

func TestManagerWideDialogControlsFitAfterShrink(t *testing.T) {
	_, plugin, app, _ := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{{Kind: KindProfile, Name: "one", Enabled: true}},
	})
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(180, 60)
	vtui.FrameManager.Init(screen)
	window := plugin.openManagerDialog(app)
	window.controller.beginInlineEdit(0, window.controller.config.Entries[0])
	x1, y1, _, _ := window.GetPosition()
	window.ChangeSize(78, 23)
	window.Show(screen)
	for _, child := range window.GetChildren() {
		left, top, right, bottom := child.GetPosition()
		if left <= x1 || top <= y1 || right >= x1+77 || bottom >= y1+22 {
			t.Errorf("child %T escapes shrunk wide dialog: (%d,%d)-(%d,%d)", child, left, top, right, bottom)
		}
	}
	_, _, nameRight, _ := window.controller.nameEdit.GetPosition()
	enabledLeft, _, _, _ := window.controller.enabledEdit.GetPosition()
	if nameRight >= enabledLeft {
		t.Fatal("profile name overlaps enabled checkbox")
	}
}

func TestManagerResizeKeepsHeldMovesInsidePreview(t *testing.T) {
	controller, _, _, _ := newManagerControllerForTest(t, Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{{Kind: KindProfile, Name: "one", Enabled: true}},
	})
	window := controller.dialog
	x1, y1, x2, y2 := window.GetPosition()
	window.ChangeSize(x2-x1+21, y2-y1+13)
	x1, y1, x2, y2 = window.GetPosition()
	send := func(x, y int, down bool, flags uint32) {
		t.Helper()
		buttons := uint32(0)
		if down {
			buttons = vtinput.FromLeft1stButtonPressed
		}
		window.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: down, ButtonState: buttons,
			MouseEventFlags: flags, MouseX: managerMouseCoordinate(x), MouseY: managerMouseCoordinate(y),
		})
	}
	send(x2, y2, true, 0)
	for step := 1; step <= 5; step++ {
		send(x2-step*2, y2-step, true, vtinput.MouseMoved)
		_, _, right, bottom := window.GetPosition()
		if right != x2-step*2 || bottom != y2-step {
			t.Fatalf("held shrink step %d ignored: right/bottom=%d/%d, want %d/%d",
				step, right, bottom, x2-step*2, y2-step)
		}
	}
	// Release inside the read-only pane must end capture, even away from the border.
	send(x2-12, y2-7, false, 0)
	_, _, right, bottom := window.GetPosition()
	send(x2-14, y2-9, false, vtinput.MouseMoved)
	_, _, afterRight, afterBottom := window.GetPosition()
	if afterRight != right || afterBottom != bottom {
		t.Fatal("release inside preview left resize captured")
	}
	if !controller.config.Entries[0].Enabled || controller.editing {
		t.Fatal("resizing changed profile state")
	}
}
