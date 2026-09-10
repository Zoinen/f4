package app

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"testing"
)

type semanticMenuControlTestFrame struct {
	*vtui.VMenu
	returnActivations int
	selectedOnReturn  int
	inputSource       string
}

func (f *semanticMenuControlTestFrame) ProcessKey(event *vtinput.InputEvent) bool {
	if event != nil && event.Type == vtinput.KeyEventType && event.KeyDown &&
		event.VirtualKeyCode == vtinput.VK_RETURN {
		f.returnActivations++
		f.selectedOnReturn = f.SelectPos
		f.inputSource = event.InputSource
		return true
	}
	return f.VMenu.ProcessKey(event)
}

func TestSemanticMenuActivationUsesActualProviderFrame(t *testing.T) {
	control := vtui.NewVMenu("Wrapped")
	control.AddItem(vtui.MenuItem{ID: "first", Text: "First"})
	control.AddItem(vtui.MenuItem{ID: "second", Text: "Second"})
	frame := &semanticMenuControlTestFrame{VMenu: control}

	if !handleSemanticFrameAction(frame, vtui.SemanticID(frame), map[string]any{
		"action": "menu.activate",
		"index":  1,
	}) {
		t.Fatal("wrapped menu activation was not handled")
	}
	if frame.returnActivations != 1 || frame.selectedOnReturn != 1 {
		t.Fatalf("wrapped activation count/selection = %d/%d, want 1/1",
			frame.returnActivations, frame.selectedOnReturn)
	}
	if frame.inputSource != "qt_semantic" {
		t.Fatalf("wrapped activation input source = %q, want qt_semantic",
			frame.inputSource)
	}
}

func TestSemanticMenuScrollAcceptsAbsoluteNativeTop(t *testing.T) {
	oldFrameManager := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFrameManager }()

	screen := vtui.NewScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)

	control := vtui.NewVMenu("Wrapped")
	for i := 0; i < 20; i++ {
		control.AddItem(vtui.MenuItem{Text: "Row"})
	}
	control.SetPosition(0, 0, 20, 5)
	frame := &semanticMenuControlTestFrame{VMenu: control}
	target := vtui.SemanticID(frame)

	if !handleSemanticFrameAction(frame, target, map[string]any{
		"action": "menu.scroll",
		"top":    7,
	}) {
		t.Fatal("absolute native menu scroll was not handled")
	}
	if control.TopPos != 7 || control.SelectPos != 7 {
		t.Fatalf("absolute scroll top/selection = %d/%d, want 7/7",
			control.TopPos, control.SelectPos)
	}

	if !handleSemanticFrameAction(frame, target, map[string]any{
		"action": "menu.scroll",
		"delta":  -2,
	}) {
		t.Fatal("relative menu scroll was not handled")
	}
	if control.TopPos != 5 || control.SelectPos != 5 {
		t.Fatalf("relative scroll top/selection = %d/%d, want 5/5",
			control.TopPos, control.SelectPos)
	}
}
