package app

import (
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestSelectGroupDialogLayout(t *testing.T) {
	for _, actionName := range []string{"Panel.SelectGroup", "Panel.DeselectGroup"} {
		t.Run(actionName, func(t *testing.T) {
			checkMaskDialogLayout(t, actionName)
		})
	}
}

func checkMaskDialogLayout(t *testing.T, actionName string) {
	t.Helper()
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()
	useStubHistory(t)
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	defer vtui.FrameManager.Pop()

	if !RunAction(actionName) {
		t.Fatalf("%s did not run", actionName)
	}
	dlg := vtui.FrameManager.GetTopFrame().(interface {
		vtui.Container
		vtui.Frame
		GetPosition() (int, int, int, int)
		ChangeSize(int, int)
	})
	defer vtui.FrameManager.Pop()
	assertLayout := func() {
		t.Helper()
		x1, y1, x2, y2 := dlg.GetPosition()
		if y2-y1+1 != 6 {
			t.Errorf("dialog height=%d, want fixed height 6", y2-y1+1)
		}
		var edit *vtui.Edit
		var sep *vtui.Separator
		var buttons []*vtui.Button
		for _, child := range dlg.GetChildren() {
			switch c := child.(type) {
			case *vtui.Edit:
				edit = c
			case *vtui.Separator:
				sep = c
			case *vtui.Button:
				buttons = append(buttons, c)
			}
		}
		if edit == nil || len(buttons) != 2 {
			t.Fatal("expected a mask field and two buttons")
		}
		ex1, _, ex2, _ := edit.GetPosition()
		_, editY, _, _ := edit.GetPosition()
		if editY != y1+2 {
			t.Error("mask field must immediately follow the prompt without blank rows")
		}
		if ex1 != x1+2 || ex2 != x2-2 {
			t.Errorf("mask field %d..%d does not fill dialog %d..%d", ex1, ex2, x1, x2)
		}
		bx1, by1, _, _ := buttons[0].GetPosition()
		_, by2, bx2, _ := buttons[1].GetPosition()
		if gap := (bx1 - x1) - (x2 - bx2); gap < -1 || gap > 1 {
			t.Errorf("buttons are not centered: left=%d right=%d", bx1-x1, x2-bx2)
		}
		if by1 != by2 || by1 != y2-1 {
			t.Error("buttons must share the last row inside the frame")
		}
		if sep == nil {
			t.Error("missing separator above buttons")
		} else if sx1, sy1, sx2, sy2 := sep.GetPosition(); sx1 != x1 || sx2 != x2 || sy1 != by1-1 || sy2 != sy1 {
			t.Error("separator must fill the frame immediately above the buttons")
		}
	}
	assertLayout()
	resizer := dlg.(interface{ ChangeSize(int, int) })
	for _, height := range []int{3, 6, 9, 15} {
		resizer.ChangeSize(40, height)
		assertLayout()
	}
	for _, width := range []int{60, 30, 20, 40} {
		resizer.ChangeSize(width, 9)
		x1, _, x2, _ := dlg.GetPosition()
		if want := max(width, 30); x2-x1+1 != want {
			t.Errorf("dialog width=%d, want %d", x2-x1+1, want)
		}
		assertLayout()
	}
	for _, size := range [][2]int{{120, 35}, {60, 25}, {80, 25}} {
		dlg.ResizeConsole(size[0], size[1])
		x1, _, x2, _ := dlg.GetPosition()
		if x2-x1+1 != max(30, size[0]/2) {
			t.Error("dialog width must follow screen resizing")
		}
		assertLayout()
	}
	_, _, x2, y2 := dlg.GetPosition()
	dlg.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: int16(x2), MouseY: int16(y2)}) //nolint:gosec // bounded test screen coordinates
	dlg.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: int16(x2 + 10), MouseY: int16(y2 + 2)}) //nolint:gosec // bounded test screen coordinates
	assertLayout()
	_, _, resizedX2, resizedY2 := dlg.GetPosition()
	if resizedX2 != x2+10 || resizedY2 != y2 {
		t.Error("corner drag must change width only")
	}
	dlg.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: int16(x2 + 15), MouseY: int16(y2 - 4)}) //nolint:gosec // bounded test screen coordinates
	assertLayout()
	_, _, resizedX2, resizedY2 = dlg.GetPosition()
	if resizedX2 != x2+15 || resizedY2 != y2 {
		t.Error("upward corner drag must also change width only")
	}
}
