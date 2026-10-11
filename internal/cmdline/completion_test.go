package cmdline

import (
	"fmt"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type completionCommandFrame struct {
	vtui.BaseFrame
	executions        int
	lastModifiedEnter *vtinput.InputEvent
}

func (frame *completionCommandFrame) ProcessKey(event *vtinput.InputEvent) bool {
	if event.Type == vtinput.FocusEventType {
		frame.SetFocus(event.SetFocus)
	}
	if event.Type == vtinput.KeyEventType && event.KeyDown && event.VirtualKeyCode == vtinput.VK_RETURN {
		if event.ControlKeyState == 0 {
			frame.executions++
		} else {
			copy := *event
			frame.lastModifiedEnter = &copy
		}
	}
	return true
}

func (frame *completionCommandFrame) Show(*vtui.ScreenBuf)                  {}
func (frame *completionCommandFrame) GetType() vtui.FrameType               { return vtui.TypeUser }
func (frame *completionCommandFrame) ProcessMouse(*vtinput.InputEvent) bool { return false }

func TestCompletionEnterPolicy(t *testing.T) {
	previousPalette := vtui.Palette
	t.Cleanup(func() { vtui.Palette = previousPalette })
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	for _, preview := range []bool{false, true} {
		for _, selected := range []bool{false, true} {
			for _, key := range []vtinput.InputEvent{
				{},
				{ControlKeyState: vtinput.ShiftPressed},
				{ControlKeyState: vtinput.LeftCtrlPressed},
				{ControlKeyState: vtinput.RightCtrlPressed},
				{ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed},
			} {
				modifier := key.ControlKeyState
				t.Run(fmt.Sprintf("preview=%v/selected=%v/modifier=%d", preview, selected, modifier), func(t *testing.T) {
					t.Cleanup(testutil.SwapFrameManager(t))
					scr := vtui.NewSilentScreenBuf()
					scr.AllocBuf(80, 25)
					vtui.FrameManager.Init(scr)
					frame := &completionCommandFrame{}
					vtui.FrameManager.Push(frame)
					cl := NewCommandLine(">")
					cl.Edit.AutoCompletePreview = preview
					cl.Edit.AutoCompleteModifiedEnterPassthrough = false
					cl.SetPosition(0, 23, 79, 23)
					cl.Edit.History = []string{"echo first", "echo second"}
					cl.Edit.SetText("echo")
					menu := NewCompletionMenu(cl.Edit)
					if !menu.HasMatches() {
						t.Fatal("missing suggestions")
					}
					vtui.FrameManager.Push(menu)
					want := "echo first"
					if selected {
						menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
						menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
						want = "echo second"
					}
					ctrl := modifier&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
					if !selected && !ctrl {
						want = "echo"
					}
					wantExecutions := 0
					if modifier == 0 {
						wantExecutions = 1
					}
					vtui.FrameManager.PostEvent(vtinput.InputEvent{
						Type: vtinput.KeyEventType, KeyDown: true,
						VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: modifier,
					})
					for range 8 {
						vtui.FrameManager.Step(time.Millisecond)
					}
					if got := cl.Edit.GetText(); got != want {
						t.Errorf("selected=%v modifier=%d: text=%q, want %q", selected, modifier, got, want)
					}
					if frame.executions != wantExecutions {
						t.Errorf("selected=%v modifier=%d: executed %d commands, want %d", selected, modifier, frame.executions, wantExecutions)
					}
					if !menu.IsDone() {
						t.Error("accepted suggestion did not close the menu")
					}
				})
			}
		}
	}
}

func TestCompletionShiftEnterReplaysOriginalKey(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	previousConfig := config.App
	config.App.CommandLineMultiline = true
	t.Cleanup(func() { config.App = previousConfig })
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	cl := NewCommandLine(">")
	cl.SetPosition(0, 20, 79, 23)
	cl.Edit.History = []string{"echo first"}
	cl.Edit.SetText("echo")
	frame := &completionCommandFrame{}
	vtui.FrameManager.Push(frame)
	menu := NewCompletionMenu(cl.Edit)
	vtui.FrameManager.Push(menu)
	menu.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN,
	})
	vtui.FrameManager.PostEvent(vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
		ControlKeyState: vtinput.ShiftPressed,
	})
	for range 8 {
		vtui.FrameManager.Step(time.Millisecond)
	}
	if got := cl.Edit.GetText(); got != "echo first" {
		t.Fatalf("Shift+Enter lost the current preview: %q", got)
	}
	if event := frame.lastModifiedEnter; event == nil || event.VirtualKeyCode != vtinput.VK_RETURN ||
		event.ControlKeyState != vtinput.ShiftPressed || !event.KeyDown {
		t.Fatalf("Shift+Enter was not replayed to the multiline command handler: %+v", event)
	}
	if frame.executions != 0 || !menu.IsDone() {
		t.Fatalf("Shift+Enter executions=%d menuDone=%v", frame.executions, menu.IsDone())
	}
}
