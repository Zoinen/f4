package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestKeyBar_MouseClicks(t *testing.T) {
	SetDefaultPalette()

	// Подменяем глобальный FrameManager для корректной работы InjectEvents
	oldFm := FrameManager
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	kb := NewKeyBar()
	// Ширина 80, Y=24. Слот будет 80/12 = 6 символов.
	kb.SetPosition(0, 24, 79, 24)
	kb.SetVisible(true)
	fm.KeyBar = kb

	// 1. Клик по F1 (Слот 0, X=2)
	ev1 := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseX:          2,
		MouseY:          24,
		ControlKeyState: vtinput.ShiftPressed,
	}

	if !kb.ProcessMouse(ev1) {
		t.Error("KeyBar should handle click on F1 slot")
	}

	if len(fm.injectedEvents) != 1 {
		t.Fatal("Click did not inject event into FrameManager")
	}

	res := fm.injectedEvents[0]
	if res.VirtualKeyCode != vtinput.VK_F1 {
		t.Errorf("Expected VK_F1, got 0x%X", res.VirtualKeyCode)
	}
	if (res.ControlKeyState & vtinput.ShiftPressed) == 0 {
		t.Error("Injected event lost modifier state (Shift)")
	}

	// 2. Клик по F12 (Последний слот, X=75)
	fm.injectedEvents = nil
	ev2 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      75,
		MouseY:      24,
	}
	kb.ProcessMouse(ev2)
	if fm.injectedEvents[0].VirtualKeyCode != vtinput.VK_F12 {
		t.Errorf("Expected VK_F12, got 0x%X", fm.injectedEvents[0].VirtualKeyCode)
	}
}
