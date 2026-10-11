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

func TestHelpView_MouseWheelScrollsWithoutChangingSelectedLink(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 12)
	FrameManager.Init(scr)

	engine := NewHelpEngine(&mockHelpVFS{})
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "Help text"
	}
	lines[1] = "~First~First@"
	lines[20] = "~Second~Second@"
	engine.AddTopic(&HelpTopic{
		Name:  "Wheel",
		Lines: lines,
		Links: []HelpLink{{Line: 1, Target: "First"}, {Line: 20, Target: "Second"}},
	})

	hv := NewHelpView(engine, "Wheel")
	hv.ResizeConsole(80, 12)
	selected := hv.selectedIdx

	if !hv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1}) {
		t.Fatal("wheel down was not handled")
	}
	if hv.scrollTop != 1 {
		t.Fatalf("wheel down scrollTop = %d, want 1", hv.scrollTop)
	}
	if hv.selectedIdx != selected {
		t.Fatalf("wheel down selected link = %d, want unchanged %d", hv.selectedIdx, selected)
	}

	hv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if hv.scrollTop != 0 {
		t.Fatalf("wheel up scrollTop = %d, want 0", hv.scrollTop)
	}
	if hv.selectedIdx != selected {
		t.Fatalf("wheel up selected link = %d, want unchanged %d", hv.selectedIdx, selected)
	}

	// The viewport stays clamped at the top and bottom even if the wheel
	// keeps producing events there.
	hv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if hv.scrollTop != 0 {
		t.Fatalf("wheel scrolled above top to %d", hv.scrollTop)
	}
	for range 100 {
		hv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	}
	viewHeight := (hv.Y2 - hv.Y1 + 1) - 2 - hv.current.StickyRows
	wantBottom := len(lines) - hv.current.StickyRows - viewHeight
	if hv.scrollTop != wantBottom {
		t.Fatalf("wheel bottom scrollTop = %d, want %d", hv.scrollTop, wantBottom)
	}
	if hv.selectedIdx != selected {
		t.Fatalf("wheel scrolling changed selected link to %d, want %d", hv.selectedIdx, selected)
	}
}

func TestHelpView_ScrollTopAndSourceRow(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)

	lines := make([]string, 60)
	for i := range lines {
		lines[i] = "line"
	}
	engine := NewHelpEngine(&mockHelpVFS{})
	engine.AddTopic(&HelpTopic{Name: "Long", Lines: lines})
	hv := NewHelpView(engine, "Long")
	hv.ResizeConsole(80, 25)

	if hv.ScrollTop() != 0 {
		t.Fatalf("ScrollTop at the start = %d, want 0", hv.ScrollTop())
	}
	hv.SetScrollTop(10)
	if hv.ScrollTop() != 10 {
		t.Fatalf("ScrollTop after SetScrollTop(10) = %d", hv.ScrollTop())
	}
	hv.SetScrollTop(1000)
	if got := hv.ScrollTop(); got <= 10 || got >= len(lines) {
		t.Errorf("SetScrollTop(1000) = %d, want the last scrollable position", got)
	}
	hv.SetScrollTop(-5)
	if hv.ScrollTop() != 0 {
		t.Errorf("SetScrollTop(-5) = %d, want 0", hv.ScrollTop())
	}
	if src, ok := hv.SourceRow(3); !ok || src != 3 {
		t.Errorf("SourceRow(3) = %d, %v", src, ok)
	}
	if _, ok := hv.SourceRow(len(hv.CurrentTopic().Lines)); ok {
		t.Error("SourceRow past the end reported ok")
	}
	if _, ok := hv.SourceRow(-1); ok {
		t.Error("SourceRow(-1) reported ok")
	}
}
