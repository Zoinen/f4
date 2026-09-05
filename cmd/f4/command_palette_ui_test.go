package main

import (
	"fmt"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newCommandPaletteUITestDialog(
	t *testing.T,
	width, height int,
	entries []commandPaletteEntry,
	onExecute func(commandPaletteEntry),
) (*commandPaletteDialog, *vtui.ScreenBuf) {
	t.Helper()
	screen := newCommandPaletteUITestScreen(t, width, height)
	dialog := newCommandPaletteDialog(entries, nil, onExecute)
	return dialog, screen
}

func newCommandPaletteUITestScreen(t *testing.T, width, height int) *vtui.ScreenBuf {
	t.Helper()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(width, height)
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() {
		reset := vtui.NewSilentScreenBuf()
		reset.AllocBuf(80, 25)
		vtui.FrameManager.Init(reset)
	})
	return screen
}

func commandPaletteKey(code uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: code,
	}
}

func TestCommandPaletteKeepsQueryFocusedWhileNavigatingAndExecutesSelection(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "alpha", Label: "Alpha", Description: "First command", Category: "Commands"},
		{Key: "bravo", Label: "Bravo", Description: "Second command", Category: "Commands"},
		{Key: "charlie", Label: "Charlie", Description: "Third command", Category: "Commands"},
	}
	var executed commandPaletteEntry
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, entries, func(entry commandPaletteEntry) {
		executed = entry
	})

	if !dialog.query.IsFocused() {
		t.Fatal("command palette did not initially focus its query editor")
	}
	if dialog.table.CanFocus() {
		t.Fatal("result table must not take focus away from the query editor")
	}
	if !dialog.table.AlwaysShowCursor {
		t.Fatal("unfocused result table does not keep its selection visible")
	}

	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_DOWN)) || dialog.table.SelectPos != 1 {
		t.Fatalf("Down did not select the second result: %d", dialog.table.SelectPos)
	}
	if !dialog.query.IsFocused() {
		t.Fatal("result navigation moved focus away from the query editor")
	}
	if got := dialog.description.GetText(); got != "Second command" {
		t.Fatalf("description after navigation = %q, want second command", got)
	}
	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_END)) || dialog.table.SelectPos != 2 {
		t.Fatalf("End did not select the final result: %d", dialog.table.SelectPos)
	}
	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_HOME)) || dialog.table.SelectPos != 0 {
		t.Fatalf("Home did not restore the first result: %d", dialog.table.SelectPos)
	}

	if !dialog.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_B, Char: 'b',
	}) {
		t.Fatal("query editor did not consume printable input")
	}
	if got := dialog.query.GetText(); got != "b" {
		t.Fatalf("query = %q, want b", got)
	}
	if len(dialog.filtered) != 1 || dialog.filtered[0].Key != "bravo" {
		t.Fatalf("filtered results = %#v, want only bravo", commandPaletteUIKeys(dialog.filtered))
	}
	if dialog.table.SelectPos != 0 || !dialog.query.IsFocused() {
		t.Fatal("filtering did not reset result selection while retaining editor focus")
	}

	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_RETURN)) {
		t.Fatal("Enter was not consumed")
	}
	if executed.Key != "bravo" {
		t.Fatalf("executed %q, want bravo", executed.Key)
	}
	if !dialog.IsDone() {
		t.Fatal("command palette did not close before executing the command")
	}
}

func commandPaletteUIKeys(entries []commandPaletteEntry) []string {
	keys := make([]string, len(entries))
	for index := range entries {
		keys[index] = entries[index].Key
	}
	return keys
}

func TestCommandPaletteEmptyResultDoesNotExecute(t *testing.T) {
	executions := 0
	dialog, _ := newCommandPaletteUITestDialog(t, 80, 25,
		[]commandPaletteEntry{{Key: "alpha", Label: "Alpha"}},
		func(commandPaletteEntry) { executions++ },
	)

	dialog.query.SetText("no such command")
	dialog.refilter(dialog.query.GetText())
	if len(dialog.filtered) != 0 || dialog.table.ItemCount != 1 {
		t.Fatalf("empty filter state = %d entries, %d rows; want zero entries and one message row",
			len(dialog.filtered), dialog.table.ItemCount)
	}
	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_RETURN)) {
		t.Fatal("Enter on the empty result was not consumed")
	}
	if executions != 0 || dialog.IsDone() {
		t.Fatalf("empty result executed %d commands or closed=%v", executions, dialog.IsDone())
	}

	if !dialog.ProcessKey(commandPaletteKey(vtinput.VK_ESCAPE)) || !dialog.IsDone() {
		t.Fatal("Esc did not close the command palette")
	}
}

func TestCommandPaletteFitsAllResultsWithoutUnnecessaryScrollbar(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "alpha", Label: "Alpha"},
		{Key: "bravo", Label: "Bravo"},
		{Key: "charlie", Label: "Charlie"},
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 100, 30, entries, nil)
	if dialog.table.ViewHeight < len(entries) {
		t.Fatalf("table shows %d of %d fitting results", dialog.table.ViewHeight, len(entries))
	}
	if dialog.table.ItemCount > dialog.table.ViewHeight {
		t.Fatal("palette would draw a scrollbar although every result fits")
	}
}

func TestCommandPaletteMouseScrollRefreshesDescriptionAndHeaderClickIsConsumed(t *testing.T) {
	entries := make([]commandPaletteEntry, 12)
	for index := range entries {
		entries[index] = commandPaletteEntry{
			Key:         fmt.Sprintf("command-%02d", index),
			Label:       fmt.Sprintf("Command %02d", index),
			Description: fmt.Sprintf("Description %02d", index),
		}
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 80, 14, entries, nil)
	if got := dialog.description.GetText(); got != "Description 00" {
		t.Fatalf("initial description = %q", got)
	}
	if !dialog.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseX: testInt16(dialog.table.X1), MouseY: testInt16(dialog.table.Y1 + 1),
		WheelDirection: -1,
	}) {
		t.Fatal("mouse wheel inside the result table was not consumed")
	}
	if dialog.table.SelectPos != 1 || dialog.description.GetText() != "Description 01" {
		t.Fatalf("wheel selection=%d description=%q", dialog.table.SelectPos, dialog.description.GetText())
	}

	originalX, originalY := dialog.X1, dialog.Y1
	if !dialog.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: testInt16(dialog.table.X1), MouseY: testInt16(dialog.table.Y1),
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}) {
		t.Fatal("table header click was not consumed")
	}
	dialog.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: testInt16(dialog.table.X1 + 5), MouseY: testInt16(dialog.table.Y1 + 2),
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseEventFlags: vtinput.MouseMoved,
	})
	if dialog.X1 != originalX || dialog.Y1 != originalY {
		t.Fatalf("table header click started dragging dialog: (%d,%d) -> (%d,%d)", originalX, originalY, dialog.X1, dialog.Y1)
	}
}

func TestCommandPaletteUsesCompactLayoutOnSmallScreensAndAfterResize(t *testing.T) {
	entries := []commandPaletteEntry{
		{Key: "alpha", Label: "Alpha"},
		{Key: "bravo", Label: "Bravo"},
	}
	dialog, _ := newCommandPaletteUITestDialog(t, 80, 25, entries, nil)
	dialog.ResizeConsole(40, 10)
	if dialog.X1 < 0 || dialog.Y1 < 0 || dialog.X2 >= 40 || dialog.Y2 >= 10 {
		t.Fatalf("resized dialog outside 40x10 screen: (%d,%d)-(%d,%d)", dialog.X1, dialog.Y1, dialog.X2, dialog.Y2)
	}
	if len(dialog.table.Columns) != 1 {
		t.Fatalf("compact width kept %d columns, want command only", len(dialog.table.Columns))
	}
	if dialog.table.X1 > dialog.table.X2 || dialog.table.Y1 > dialog.table.Y2 || dialog.table.ViewHeight < 1 {
		t.Fatalf("invalid compact table geometry: (%d,%d)-(%d,%d), view=%d",
			dialog.table.X1, dialog.table.Y1, dialog.table.X2, dialog.table.Y2, dialog.table.ViewHeight)
	}
	vtui.AssertLayout(t, dialog)

	dialog.ResizeConsole(48, 13)
	if dialog.X1 < 0 || dialog.Y1 < 0 || dialog.X2 >= 48 || dialog.Y2 >= 13 {
		t.Fatalf("resized dialog outside 48x13 screen: (%d,%d)-(%d,%d)", dialog.X1, dialog.Y1, dialog.X2, dialog.Y2)
	}
	if len(dialog.table.Columns) != 1 || dialog.table.ViewHeight < len(entries) {
		t.Fatalf("48x13 compact table columns=%d view=%d", len(dialog.table.Columns), dialog.table.ViewHeight)
	}
	vtui.AssertLayout(t, dialog)
}

func TestCommandPaletteDialogTracksLiveThemeWithEditSelectionBorderAndScrollbar(t *testing.T) {
	indices := []int{
		vtui.ColDialogText,
		vtui.ColDialogEdit,
		vtui.ColDialogEditSelected,
		vtui.ColDialogSelectedButton,
		vtui.ColDialogHighlightText,
		vtui.ColDialogHighlightSelectedButton,
		vtui.ColDialogBox,
		vtui.ColDialogBoxTitle,
	}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	t.Cleanup(func() {
		for index, attribute := range original {
			vtui.Palette[index] = attribute
		}
	})

	first := map[int]uint64{
		vtui.ColDialogText:                    vtui.SetRGBBoth(0, 0x101112, 0x202122),
		vtui.ColDialogEdit:                    vtui.SetRGBBoth(0, 0x131415, 0x232425),
		vtui.ColDialogEditSelected:            vtui.SetRGBBoth(0, 0x161718, 0x262728),
		vtui.ColDialogSelectedButton:          vtui.SetRGBBoth(0, 0x303132, 0x404142),
		vtui.ColDialogHighlightText:           vtui.SetRGBBoth(0, 0x505152, 0x606162),
		vtui.ColDialogHighlightSelectedButton: vtui.SetRGBBoth(0, 0x707172, 0x808182),
		vtui.ColDialogBox:                     vtui.SetRGBBoth(0, 0x909192, 0xa0a1a2),
		vtui.ColDialogBoxTitle:                vtui.SetRGBBoth(0, 0xb0b1b2, 0xc0c1c2),
	}
	entries := make([]commandPaletteEntry, 18)
	for index := range entries {
		entries[index] = commandPaletteEntry{
			Key: "command", Label: "Command", Description: "Description", Category: "Category",
		}
	}
	screen := newCommandPaletteUITestScreen(t, 80, 20)
	for index, attribute := range first {
		vtui.Palette[index] = attribute
	}
	dialog := newCommandPaletteDialog(entries, nil, nil)
	if dialog.table.ColorTextIdx != vtui.ColDialogText ||
		dialog.table.ColorSelectedTextIdx != vtui.ColDialogSelectedButton ||
		dialog.table.ColorItemSelectTextIdx != vtui.ColDialogHighlightText ||
		dialog.table.ColorItemSelectCursorIdx != vtui.ColDialogHighlightSelectedButton ||
		dialog.table.ColorTitleIdx != vtui.ColDialogHighlightText ||
		dialog.table.ColorBoxIdx != vtui.ColDialogBox {
		t.Fatal("command palette table is not mapped to semantic dialog palette indices")
	}
	if dialog.table.ScrollBar == nil || dialog.table.ScrollBar.ColorIdx != vtui.ColDialogBox {
		t.Fatal("command palette scrollbar does not use the dialog box palette entry")
	}

	dialog.Show(screen)
	assertCommandPaletteThemeCells(t, dialog, screen, first, vtui.ColDialogEdit)

	dialog.query.SetText("command")
	dialog.refilter(dialog.query.GetText())
	dialog.query.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_HOME, ControlKeyState: vtinput.ShiftPressed,
	})

	second := map[int]uint64{
		vtui.ColDialogText:                    vtui.SetRGBBoth(0, 0x111213, 0x212223),
		vtui.ColDialogEdit:                    vtui.SetRGBBoth(0, 0x141516, 0x242526),
		vtui.ColDialogEditSelected:            vtui.SetRGBBoth(0, 0x171819, 0x272829),
		vtui.ColDialogSelectedButton:          vtui.SetRGBBoth(0, 0x313233, 0x414243),
		vtui.ColDialogHighlightText:           vtui.SetRGBBoth(0, 0x515253, 0x616263),
		vtui.ColDialogHighlightSelectedButton: vtui.SetRGBBoth(0, 0x717273, 0x818283),
		vtui.ColDialogBox:                     vtui.SetRGBBoth(0, 0x919293, 0xa1a2a3),
		vtui.ColDialogBoxTitle:                vtui.SetRGBBoth(0, 0xb1b2b3, 0xc1c2c3),
	}
	for index, attribute := range second {
		vtui.Palette[index] = attribute
	}
	dialog.Show(screen)
	assertCommandPaletteThemeCells(t, dialog, screen, second, vtui.ColDialogEditSelected)

	vtui.AssertLayout(t, dialog)
}

func assertCommandPaletteThemeCells(
	t *testing.T,
	dialog *commandPaletteDialog,
	screen *vtui.ScreenBuf,
	want map[int]uint64,
	editPaletteIdx int,
) {
	t.Helper()
	checks := []struct {
		name       string
		x, y       int
		paletteIdx int
	}{
		{name: "query prompt", x: dialog.queryPrompt.X1, y: dialog.queryPrompt.Y1, paletteIdx: vtui.ColDialogText},
		{name: "edit", x: dialog.query.X1, y: dialog.query.Y1, paletteIdx: editPaletteIdx},
		{name: "table header", x: dialog.table.X1, y: dialog.table.Y1, paletteIdx: vtui.ColDialogHighlightText},
		{name: "table cursor", x: dialog.table.X1, y: dialog.table.Y1 + 1, paletteIdx: vtui.ColDialogSelectedButton},
		{name: "normal row", x: dialog.table.X1, y: dialog.table.Y1 + 2, paletteIdx: vtui.ColDialogText},
		{name: "description", x: dialog.description.X1, y: dialog.description.Y1, paletteIdx: vtui.ColDialogText},
		{name: "border", x: dialog.X1, y: dialog.Y1 + 1, paletteIdx: vtui.ColDialogBox},
		{name: "title", x: (dialog.X1 + dialog.X2) / 2, y: dialog.Y1, paletteIdx: vtui.ColDialogBoxTitle},
		{name: "scrollbar", x: dialog.table.ScrollBar.X1, y: dialog.table.ScrollBar.Y1, paletteIdx: vtui.ColDialogBox},
	}
	for _, check := range checks {
		if got := screen.GetCell(check.x, check.y).Attributes; got != want[check.paletteIdx] {
			t.Errorf("%s attr = %#x, want palette[%d] %#x", check.name, got, check.paletteIdx, want[check.paletteIdx])
		}
	}
}
