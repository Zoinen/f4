package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type issue561ViewportRenderer struct {
}

func (*issue561ViewportRenderer) Render([]vtui.CharInfo, []vtui.CharInfo, int, int, bool) {}
func (*issue561ViewportRenderer) SetCursor(int, int, bool, vtui.CursorShape)              {}
func (*issue561ViewportRenderer) SetPalette(*[256]uint32)                                 {}
func (*issue561ViewportRenderer) SetWindowTitle(string)                                   {}
func (*issue561ViewportRenderer) Flush()                                                  {}

func TestIssue561PanelSettingsRequestsViewportLargeEnoughForDialog(t *testing.T) {
	oldConfig := AppConfig
	t.Cleanup(func() { AppConfig = oldConfig })
	t.Cleanup(swapFrameManager(t))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	renderer := &issue561ViewportRenderer{}
	scr.Renderer = renderer
	vtui.FrameManager.Init(scr)

	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	vtui.FrameManager.Push(pf)
	actionPanelSettings(pf)

	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want *vtui.Window", vtui.FrameManager.GetTopFrame())
	}
	_, y1, _, y2 := top.GetPosition()
	if y1 < 0 || y2 >= 30 {
		t.Fatalf("Panel Settings viewport = (%d,%d), want it fully within 30 rows", y1, y2)
	}
	top.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	focused := top.GetFocusedItem()
	if focused == nil {
		t.Fatal("Panel Settings has no focused control after Page Down")
	}
	button, ok := focused.(*vtui.Button)
	if !ok || !button.IsDefault {
		t.Fatalf("Page Down focused %T, want the default action button", focused)
	}
	_, _, _, focusedY := focused.GetPosition()
	if focusedY >= 29 {
		t.Fatalf("default button remained below viewport at row %d", focusedY)
	}
}
