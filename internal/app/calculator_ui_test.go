package app

import (
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

func TestCalculatorDialogEvaluatesExpression(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	showCalculatorDialog()
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want calculator dialog", vtui.FrameManager.GetTopFrame())
	}
	vtui.AssertLayout(t, dlg)
	t.Cleanup(func() { vtui.FrameManager.RemoveFrame(dlg) })

	resultPlaceholder := i18n.Msg("Calculator.ResultEmpty")
	var edit *vtui.Edit
	var result *vtui.Text
	walkUI(dlg, func(el vtui.UIElement) bool {
		switch v := el.(type) {
		case *vtui.Edit:
			edit = v
		case *vtui.Text:
			if v.GetText() == resultPlaceholder {
				result = v
			}
		}
		return true
	})
	if edit == nil {
		t.Fatal("dialog has no expression field")
	}
	if result == nil {
		t.Fatal("dialog has no result label")
	}

	edit.SetText("2+2*3")
	if edit.OnAction == nil {
		t.Fatal("expression field has no Enter handler")
	}
	edit.OnAction()
	if got := result.GetText(); got != "= 8" {
		t.Fatalf("result label = %q, want %q", got, "= 8")
	}

	edit.SetText("not an expression")
	edit.OnAction()
	if got := result.GetText(); got == "= 8" {
		t.Fatalf("result label did not update on an invalid expression, still %q", got)
	}
}

func TestCalculatorInsertWithoutPanelsFrameDoesNotPanic(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	// No panels frame registered: FindPanelsFrameAnyScreen() returns nil, and
	// insertTextIntoCommandLine must simply do nothing rather than panic.
	insertTextIntoCommandLine("42")
}
