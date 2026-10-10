package app

import (
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func commandPaletteCtrlK() *vtinput.InputEvent {
	e := commandPaletteKey(vtinput.VK_K)
	e.ControlKeyState = vtinput.LeftCtrlPressed
	return e
}

func TestCommandPaletteCtrlKOpensKeyDialogForAnAction(t *testing.T) {
	old := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = old })

	entries := []commandPaletteEntry{
		{Key: "act:file.attributes", Label: "Attributes", ID: "File.Attributes", source: commandPaletteSourceAction},
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, entries, nil)
	vtui.FrameManager.Push(dialog)

	if !dialog.ProcessKey(commandPaletteCtrlK()) {
		t.Fatal("Ctrl+K was not consumed")
	}
	if vtui.FrameManager.GetTopFrame() == vtui.Frame(dialog) {
		t.Fatal("Ctrl+K did not open the key assignment dialog over the palette")
	}
	if dialog.query.GetText() != "" {
		t.Fatalf("Ctrl+K typed into the query: %q", dialog.query.GetText())
	}
}

func TestCommandPaletteCtrlKOnPlainEntryOnlyExplains(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "workspace:1", Label: "Workspace 1", run: func() bool { return true }},
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, entries, nil)
	vtui.FrameManager.Push(dialog)

	if !dialog.ProcessKey(commandPaletteCtrlK()) {
		t.Fatal("Ctrl+K was not consumed")
	}
	if _, ok := commandPaletteAssignTarget(entries[0]); ok {
		t.Fatal("an entry without an action offers a key")
	}
}

func TestCommandPaletteAssignChordIsOnlyPlainCtrlK(t *testing.T) {
	for name, state := range map[string]uint32{
		"plain K":    0,
		"Ctrl+Shift": uint32(vtinput.LeftCtrlPressed | vtinput.ShiftPressed),
		"Ctrl+Alt":   uint32(vtinput.LeftCtrlPressed | vtinput.LeftAltPressed),
	} {
		e := commandPaletteKey(vtinput.VK_K)
		e.ControlKeyState = vtinput.ControlKeyState(state)
		if commandPaletteAssignKey(e) {
			t.Errorf("%s was taken for the assign chord", name)
		}
	}
	if !commandPaletteAssignKey(commandPaletteCtrlK()) {
		t.Fatal("Ctrl+K is not the assign chord")
	}
}

func TestCommandPaletteReloadAfterAssignKeepsCursor(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "a", Label: "Alpha"},
		{Key: "b", Label: "Bravo"},
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, entries, nil)
	dialog.rebuild = func() []commandPaletteEntry {
		return []commandPaletteEntry{
			{Key: "a", Label: "Alpha"},
			{Key: "b", Label: "Bravo", Shortcut: "Ctrl+Q"},
		}
	}
	dialog.table.SetSelectPos(1)
	dialog.reloadAfterAssign("b")
	if dialog.table.SelectPos != 1 || dialog.filtered[1].Shortcut != "Ctrl+Q" {
		t.Fatalf("cursor %d, shortcut %q", dialog.table.SelectPos, dialog.filtered[1].Shortcut)
	}
}
