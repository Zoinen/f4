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

func TestSemanticKeyBarExportsIconForActiveModifier(t *testing.T) {
	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	kb.SetVisible(true)
	kb.Normal[0] = "Help"
	kb.NormalIcons[0] = "circle-question-mark"
	kb.Ctrl[0] = "Left"
	kb.CtrlIcons[0] = "panel-left"

	normal := semanticKeyBar(kb)
	normalItems := normal["items"].([]map[string]any)
	if normalItems[0]["icon"] != "circle-question-mark" {
		t.Fatalf("normal key-bar icon = %#v", normalItems[0]["icon"])
	}
	normalAlternatives, ok := normalItems[0]["alternatives"].([]map[string]any)
	if !ok || len(normalAlternatives) != 1 ||
		normalAlternatives[0]["modifier"] != "ctrl" ||
		normalAlternatives[0]["text"] != "Left" ||
		normalAlternatives[0]["icon"] != "panel-left" {
		t.Fatalf("normal key-bar alternatives = %#v", normalItems[0])
	}

	kb.SetModifiers(false, true, false)
	ctrl := semanticKeyBar(kb)
	ctrlItems := ctrl["items"].([]map[string]any)
	if ctrl["modifier"] != "ctrl" || ctrlItems[0]["icon"] != "panel-left" {
		t.Fatalf("ctrl key-bar metadata = %#v", ctrl)
	}
	if ctrlAlternatives, ok := ctrlItems[0]["alternatives"].([]map[string]any); !ok ||
		len(ctrlAlternatives) != 1 || ctrlAlternatives[0]["modifier"] != "normal" ||
		ctrlAlternatives[0]["text"] != "Help" {
		t.Fatalf("ctrl key-bar alternatives = %#v", ctrlItems[0])
	}
}
