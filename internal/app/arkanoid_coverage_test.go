package app

import (
	"testing"

	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestArkanoidProcessKeyAndMouseControls(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	af := NewArkanoidFrame()
	af.Close()
	af.paddleX = 10

	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}) || !af.leftPressed {
		t.Fatal("left key-down was not consumed")
	}
	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_LEFT}) || af.leftPressed {
		t.Fatal("left key-up did not clear the pressed state")
	}
	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) || !af.rightPressed {
		t.Fatal("right key-down was not consumed")
	}
	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_RIGHT}) || af.rightPressed {
		t.Fatal("right key-up did not clear the pressed state")
	}

	modifiers := vtinput.LeftCtrlPressed | vtinput.LeftAltPressed
	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, ControlKeyState: modifiers}) || !af.autoPlay {
		t.Fatal("Ctrl+Alt+A did not enable auto-play")
	}
	if !af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, ControlKeyState: modifiers}) || af.autoPlay {
		t.Fatal("Ctrl+Alt+A did not toggle auto-play off")
	}

	for i := 0; i < 7; i++ {
		af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '+'})
	}
	if af.autoSpeed != 5 {
		t.Fatalf("plus key autoSpeed = %d, want clamp at 5", af.autoSpeed)
	}
	for i := 0; i < 12; i++ {
		af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SUBTRACT})
	}
	if af.autoSpeed != -5 {
		t.Fatalf("subtract key autoSpeed = %d, want clamp at -5", af.autoSpeed)
	}
	if af.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_F1}) {
		t.Fatal("unhandled key-up was consumed")
	}

	if af.ProcessMouse(&vtinput.InputEvent{Type: vtinput.KeyEventType}) {
		t.Fatal("non-mouse event was consumed as mouse input")
	}
	if !af.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      int16(af.X2), //nolint:gosec // the test frame coordinates are bounded screen positions
		MouseY:      int16(af.Y2), //nolint:gosec // the test frame coordinates are bounded screen positions
	}) {
		t.Fatal("bottom-right resize click was not consumed")
	}
	if af.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: 1, MouseY: 1}) {
		t.Fatal("irrelevant mouse event was consumed")
	}
}

func TestArkanoidShowCoversAnimatedStates(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	af := NewArkanoidFrame()
	af.Close()
	af.autoPlay = true
	af.levelClearTimer = 12
	af.message = "LEVEL 1 CLEARED!"
	af.popup = scorePopup{val: 120, colors: []uint64{vtui.SetRGBBoth(0, 0xFF00FF, 0), vtui.SetRGBBoth(0, 0x00FFFF, 0)}, timer: 18}
	af.combo = 13
	af.multiplier = 3
	af.autoSpeed = 2
	af.flashTimer = 6
	af.gameOver = true
	af.Show(vtui.NewSilentScreenBuf())

	if af.flashTimer != 5 {
		t.Fatalf("Show() flashTimer = %d, want one frame consumed", af.flashTimer)
	}
}
