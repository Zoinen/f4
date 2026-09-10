package terminal

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestTerminalViewSemanticModelPrimaryScreen(t *testing.T) {
	tv := NewTerminalView(4, 3)
	defer tv.Close()
	tv.Title = "shell"
	tv.SetVisible(true)
	tv.SetFocus(true)
	tv.SetMuted(true)
	tv.Lines[1][0] = vtui.CharInfo{Char: 'A', Attributes: 1}
	tv.Lines[1][1] = vtui.CharInfo{Char: 'B', Attributes: 2}
	tv.CursorX = 2
	tv.CursorY = 1

	model := tv.SemanticModel(nil)
	if model.Title != "shell" || !model.Visible || !model.Focused || !model.Busy {
		t.Fatalf("unexpected terminal state: %#v", model)
	}
	if model.AltScreen || model.CursorX != 2 || model.CursorY != 2 {
		t.Fatalf("unexpected primary-screen cursor state: %#v", model)
	}
	if len(model.Rows) != 3 || model.Rows[0].Index != 0 || model.Rows[2].Index != 2 {
		t.Fatalf("unexpected primary-screen rows: %#v", model.Rows)
	}
	if len(model.Rows[2].Runs) < 2 || model.Rows[2].Runs[0].Text != "A" || model.Rows[2].Runs[1].Text != "B" {
		t.Fatalf("unexpected semantic runs: %#v", model.Rows[2].Runs)
	}
}

func TestTerminalViewSemanticModelAltScreen(t *testing.T) {
	tv := NewTerminalView(3, 2)
	defer tv.Close()
	tv.UseAltScreen = true
	tv.AltLines[0][0] = vtui.CharInfo{Char: 'X', Attributes: 3}
	tv.CursorX = 1
	tv.CursorY = 0

	model := tv.SemanticModel(nil)
	if !model.AltScreen || model.CursorX != 1 || model.CursorY != 0 {
		t.Fatalf("unexpected alt-screen state: %#v", model)
	}
	if len(model.Rows) != 2 || model.Rows[0].Index != 0 || model.Rows[1].Index != 1 {
		t.Fatalf("unexpected alt-screen rows: %#v", model.Rows)
	}
	if len(model.Rows[0].Runs) == 0 || model.Rows[0].Runs[0].Text != "X" {
		t.Fatalf("unexpected alt-screen runs: %#v", model.Rows[0].Runs)
	}
}
