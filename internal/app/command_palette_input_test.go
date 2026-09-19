package app

import (
	"fmt"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestCommandPaletteNativeSelectionAndWordEditing(t *testing.T) {
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, []commandPaletteEntry{
		{Key: "alpha", Label: "alpha beta"},
	}, nil)
	dialog.query.SetText("alpha beta")
	dialog.refilter(dialog.query.GetText())
	selectRange := func(anchor, cursor int) {
		t.Helper()
		if !dialog.HandleSemanticAction(map[string]any{
			"action": "control.select", "target": vtui.SemanticID(dialog.query),
			"anchor": anchor, "cursor": cursor,
		}) {
			t.Fatal("query selection action was not handled")
		}
	}
	selectRange(10, 10)
	event := commandPaletteKey(vtinput.VK_LEFT)
	event.ControlKeyState = vtinput.LeftCtrlPressed | vtinput.ShiftPressed
	dialog.ProcessKey(event)
	state := dialog.query.SemanticNode(nil)
	if state["cursor"] != 6 || state["selectionStart"] != 6 || state["selectionEnd"] != 10 {
		t.Fatalf("word selection did not reach query: %#v", state)
	}
	dialog.ProcessKey(commandPaletteKey(vtinput.VK_BACK))
	if dialog.query.GetText() != "alpha " || dialog.lastQuery != "alpha " {
		t.Fatalf("selection deletion/query refresh = %q / %q", dialog.query.GetText(), dialog.lastQuery)
	}
	selectRange(0, 0)
	event = commandPaletteKey(vtinput.VK_DELETE)
	event.ControlKeyState = vtinput.LeftCtrlPressed
	dialog.ProcessKey(event)
	if dialog.query.GetText() != "" || dialog.lastQuery != "" {
		t.Fatalf("word deletion/query refresh = %q / %q", dialog.query.GetText(), dialog.lastQuery)
	}
}

func TestCommandPaletteRelativeWheelAccumulatesBeforeSceneUpdate(t *testing.T) {
	entries := make([]commandPaletteEntry, 100)
	for index := range entries {
		entries[index] = commandPaletteEntry{
			Key: fmt.Sprintf("command-%02d", index), Label: fmt.Sprintf("Command %02d", index),
			Description: fmt.Sprintf("Description %02d", index),
		}
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 24, entries, nil)
	scroll := func(delta int) {
		t.Helper()
		if !dialog.HandleSemanticAction(map[string]any{
			"action": "control.scroll", "target": vtui.SemanticID(dialog.table), "delta": delta,
		}) {
			t.Fatal("relative scroll was not handled")
		}
	}
	scroll(9) // A single event carrying three three-line notches.
	for range 12 {
		scroll(3) // No intervening semantic snapshot acknowledgement.
	}
	if dialog.table.SelectPos != 45 || dialog.table.TopPos != 45 {
		t.Fatalf("wheel motion was lost: cursor=%d top=%d", dialog.table.SelectPos, dialog.table.TopPos)
	}
	if dialog.description.GetText() != "Description 45" || !dialog.query.IsFocused() {
		t.Fatalf("wheel changed query focus or left stale description: %q", dialog.description.GetText())
	}
	scroll(-1000)
	if dialog.table.SelectPos != 0 || dialog.table.TopPos != 0 {
		t.Fatal("upward scroll did not clamp at the first result")
	}
	scroll(1000)
	if dialog.table.SelectPos != 99 || dialog.table.TopPos != 100-dialog.table.ViewHeight {
		t.Fatal("downward scroll did not clamp at the last result")
	}
}
