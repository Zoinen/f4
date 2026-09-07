package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestEventSink_ButtonCommand(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	oldFM := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	var received []UIEvent
	fm.SetEventSink(func(ev UIEvent) {
		received = append(received, ev)
	})

	dlg := NewDialog(0, 0, 40, 10, "Test Dlg")
	dlg.SetID("dlg")

	btn := NewButton(2, 2, "&Submit")
	btn.SetID("okBtn")
	btn.Command = 1000
	dlg.AddItem(btn)
	fm.Push(dlg)

	// Simulate pressing Enter on the button
	btn.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if len(received) != 1 {
		t.Fatalf("Expected exactly 1 UIEvent, got %d: %v", len(received), received)
	}

	ev := received[0]
	if ev.Kind != "command" || ev.Cmd != 1000 || ev.SrcID != "okBtn" {
		t.Errorf("Unexpected event payload: %+v", ev)
	}
}

func TestEventSink_DialogClosed(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	oldFM := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	var received []UIEvent
	fm.SetEventSink(func(ev UIEvent) {
		received = append(received, ev)
	})

	dlg := NewDialog(0, 0, 40, 10, "Close Test")
	dlg.SetID("closeDlg")
	fm.Push(dlg)

	dlg.SetExitCode(42)

	if len(received) != 1 {
		t.Fatalf("Expected 1 close event, got %d: %v", len(received), received)
	}
	if received[0].Kind != "closed" || received[0].SrcID != "closeDlg" || received[0].Index != 42 {
		t.Errorf("Unexpected close event: %+v", received[0])
	}
}

func TestEventSink_Resize(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	var received []UIEvent
	fm.SetEventSink(func(ev UIEvent) {
		received = append(received, ev)
	})

	fm.Resize(100, 30)

	if len(received) != 1 {
		t.Fatalf("Expected 1 resize event, got %d: %v", len(received), received)
	}
	if received[0].Kind != "resize" || received[0].Index != 100 || received[0].Value.I != 30 {
		t.Errorf("Unexpected resize event: %+v", received[0])
	}
}
