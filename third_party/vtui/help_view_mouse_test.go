package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestHelpView_MouseNavigation(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)

	engine := NewHelpEngine(&mockHelpVFS{})
	topic := &HelpTopic{
		Name: "TestTopic",
		Lines: []string{
			"Welcome to help.",
			"Link to ~Next Topic~NextTopic@",
		},
	}
	engine.AddTopic(topic)

	hv := NewHelpView(engine, "TestTopic")
	hv.ResizeConsole(80, 25)
	FrameManager.Push(hv)

	// 1. Имитируем клик по ссылке на второй строке (координата Y = Y1 + 2)
	// Кликаем по координатам mx = X1 + 11 (внутри слова Next)
	evClick := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      int16(hv.X1 + 11),
		MouseY:      int16(hv.Y1 + 2),
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}

	if !hv.ProcessMouse(evClick) {
		t.Error("Expected HelpView to handle mouse click on link")
	}

	if hv.selectedIdx != 0 {
		t.Errorf("Expected link at index 0 to be selected, got %d", hv.selectedIdx)
	}

	// 2. Имитируем двойной клик для перехода
	evDblClick := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          int16(hv.X1 + 11),
		MouseY:          int16(hv.Y1 + 2),
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		KeyDown:         true,
		MouseEventFlags: vtinput.DoubleClick,
	}

	// Добавляем целевой топик в кэш движка
	nextTopic := &HelpTopic{Name: "NextTopic", Lines: []string{"You arrived."}}
	engine.AddTopic(nextTopic)

	if !hv.ProcessMouse(evDblClick) {
		t.Error("Expected HelpView to handle mouse double-click on link")
	}

	if hv.current.Name != "NextTopic" {
		t.Errorf("Double-click failed to navigate: expected 'NextTopic', got %q", hv.current.Name)
	}

	// 3. Имитируем средний клик ВНЕ ссылки (по пустому месту)
	// Должен симулироваться Enter на текущей выделенной ссылке.
	hv.SwitchTopic("TestTopic") // Возвращаемся
	hv.selectedIdx = 0          // Ссылка выделена

	evMiddleClick := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      int16(hv.X1 + 1), // Пустое место
		MouseY:      int16(hv.Y1 + 1),
		ButtonState: vtinput.FromLeft2ndButtonPressed,
		KeyDown:     true,
	}

	if !hv.ProcessMouse(evMiddleClick) {
		t.Error("Expected HelpView to handle middle click on empty space")
	}

	if hv.current.Name != "NextTopic" {
		t.Errorf("Middle-click failed to navigate: expected 'NextTopic', got %q", hv.current.Name)
	}
}
