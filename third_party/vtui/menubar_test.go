package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestMenuBar_MouseMoveSwitchesActiveSubMenu(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFrameManager := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFrameManager }()

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "File", SubItems: []MenuItem{{Text: "Open"}}},
		{Label: "Edit", SubItems: []MenuItem{{Text: "Copy"}}},
	}
	mb.SetPosition(0, 0, 79, 0)
	mb.Active = true
	mb.ActivateSubMenu(0)
	firstSubMenu := mb.activeSubMenu

	if !mb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          int16(mb.GetItemX(1)),
		MouseY:          0,
		MouseEventFlags: vtinput.MouseMoved,
	}) {
		t.Fatal("mouse move over an active menu bar item was not handled")
	}
	if mb.SelectPos != 1 {
		t.Fatalf("hovered menu bar item = %d; want 1", mb.SelectPos)
	}
	if mb.activeSubMenu == nil || mb.activeSubMenu == firstSubMenu {
		t.Fatal("hover did not replace the active submenu")
	}
	if got := mb.activeSubMenu.GetTitle(); got != "Edit" {
		t.Fatalf("active submenu title = %q; want Edit", got)
	}
}

func TestMenuBar_MouseMoveDoesNotOpenInactiveMenu(t *testing.T) {
	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "File", SubItems: []MenuItem{{Text: "Open"}}},
		{Label: "Edit", SubItems: []MenuItem{{Text: "Copy"}}},
	}
	mb.SetPosition(0, 0, 79, 0)

	if mb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          int16(mb.GetItemX(1)),
		MouseY:          0,
		MouseEventFlags: vtinput.MouseMoved,
	}) {
		t.Fatal("hover unexpectedly activated an inactive menu bar")
	}
	if mb.Active || mb.activeSubMenu != nil {
		t.Fatal("inactive menu bar was opened by hover")
	}
}

func TestMenuBar_MouseMoveSelectsCommandItemWithoutExecuting(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	commandCount := 0
	fm.Push(&cmdMockFrame{onCmd: func(int, any) bool {
		commandCount++
		return true
	}})

	oldFrameManager := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFrameManager }()

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "File", SubItems: []MenuItem{{Text: "Open"}}},
		{Label: "Action", Command: 1234},
	}
	mb.SetPosition(0, 0, 79, 0)
	mb.Active = true
	mb.ActivateSubMenu(0)

	if !mb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          int16(mb.GetItemX(1)),
		MouseY:          0,
		MouseEventFlags: vtinput.MouseMoved,
	}) {
		t.Fatal("mouse move over a command item was not handled")
	}
	if mb.SelectPos != 1 || mb.activeSubMenu != nil {
		t.Fatal("command-only item was not selected cleanly")
	}
	if commandCount != 0 {
		t.Fatalf("hover executed %d command(s)", commandCount)
	}
}
