package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/sheet"
	"github.com/unxed/vtui"
)

func sheetDialogWindow(t *testing.T) *vtui.Window {
	t.Helper()
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want a dialog", vtui.FrameManager.GetTopFrame())
	}
	return dlg
}

func sheetDialogControls(dlg *vtui.Window) (edits []*vtui.Edit, radios []*vtui.RadioGroup, checks []*vtui.Checkbox, buttons []*vtui.Button) {
	for _, child := range dlg.GetChildren() {
		switch child := child.(type) {
		case *vtui.Edit:
			edits = append(edits, child)
		case *vtui.RadioGroup:
			radios = append(radios, child)
		case *vtui.Checkbox:
			checks = append(checks, child)
		case *vtui.Button:
			buttons = append(buttons, child)
		}
	}
	return edits, radios, checks, buttons
}

func sheetDialogButton(t *testing.T, buttons []*vtui.Button, defaultButton bool) *vtui.Button {
	t.Helper()
	for _, button := range buttons {
		if button.IsDefault == defaultButton {
			return button
		}
	}
	t.Fatalf("dialog has no button with IsDefault=%v", defaultButton)
	return nil
}

func TestSheetPathAndExportDialogs(t *testing.T) {
	sf := newSheetFrameForTest(t)
	sf.Document().SetText(0, 0, "value")
	dir := t.TempDir()

	showSheetSaveAsDialog(sf)
	saveAs := sheetDialogWindow(t)
	edits, _, _, buttons := sheetDialogControls(saveAs)
	var radios []*vtui.RadioGroup
	if len(edits) != 1 {
		t.Fatalf("Save As edits = %d, want 1", len(edits))
	}
	savePath := filepath.Join(dir, "book.f4s.sqlite")
	edits[0].SetText(savePath)
	sheetDialogButton(t, buttons, true).OnClick()
	if sf.Path() != savePath {
		t.Fatalf("saved sheet path = %q, want %q", sf.Path(), savePath)
	}
	if _, err := os.Stat(savePath); err != nil {
		t.Fatalf("Save As did not create %q: %v", savePath, err)
	}
	vtui.FrameManager.RemoveFrame(saveAs)

	showSheetOpenDialog(sf)
	open := sheetDialogWindow(t)
	_, _, _, buttons = sheetDialogControls(open)
	sheetDialogButton(t, buttons, false).OnClick()
	vtui.FrameManager.RemoveFrame(open)

	for selected, extension := range []string{".txt", ".csv", ".xlsx"} {
		showSheetExportDialog(sf)
		export := sheetDialogWindow(t)
		edits, radios, _, buttons = sheetDialogControls(export)
		if len(edits) != 1 || len(radios) != 1 {
			t.Fatalf("Export controls = edits %d, radios %d, want 1/1", len(edits), len(radios))
		}
		radios[0].Selected = selected
		if radios[0].OnChange == nil {
			t.Fatal("Export format radio group has no change handler")
		}
		radios[0].OnChange(selected)
		sheetDialogButton(t, buttons, true).OnClick()
		path := filepath.Join(dir, "book"+extension)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("export %s did not create %q: %v", extension, path, err)
		}
		vtui.FrameManager.RemoveFrame(export)
	}
}

func TestSheetGotoWidthAndFormatDialogs(t *testing.T) {
	sf := newSheetFrameForTest(t)
	sf.Document().SetText(0, 0, "value")

	showSheetGotoDialog(sf)
	gotoDlg := sheetDialogWindow(t)
	edits, _, _, buttons := sheetDialogControls(gotoDlg)
	var radios []*vtui.RadioGroup
	var checks []*vtui.Checkbox
	edits[0].SetText("B2")
	sheetDialogButton(t, buttons, true).OnClick()
	vtui.FrameManager.RemoveFrame(gotoDlg)
	if got := sf.Cursor(); got != (sheet.Point{Col: 1, Row: 1}) {
		t.Fatalf("goto cursor = %#v, want B2", got)
	}

	showSheetGotoDialog(sf)
	badGoto := sheetDialogWindow(t)
	edits, _, _, buttons = sheetDialogControls(badGoto)
	edits[0].SetText("A0")
	sheetDialogButton(t, buttons, true).OnClick()
	message := sheetDialogWindow(t)
	vtui.FrameManager.RemoveFrame(message)
	vtui.FrameManager.RemoveFrame(badGoto)

	sf.cur = sheet.Point{Col: 2, Row: 0}
	sf.Document().SetText(sf.cur.Col, sf.cur.Row, "value")
	oldWidth := sf.Document().ColumnWidth(sf.cur.Col)
	showSheetWidthDialog(sf)
	width := sheetDialogWindow(t)
	edits, _, _, buttons = sheetDialogControls(width)
	edits[0].SetText("12")
	sheetDialogButton(t, buttons, true).OnClick()
	vtui.FrameManager.RemoveFrame(width)
	if got := sf.Document().ColumnWidth(sf.cur.Col); got != 12 || got == oldWidth {
		t.Fatalf("column width = %d, want 12 (was %d)", got, oldWidth)
	}

	for selected := 0; selected < 8; selected++ {
		showSheetFormatDialog(sf)
		format := sheetDialogWindow(t)
		edits, radios, checks, buttons = sheetDialogControls(format)
		if len(edits) != 1 || len(radios) != 2 || len(checks) != 1 {
			t.Fatalf("Format controls = edits %d, radios %d, checks %d", len(edits), len(radios), len(checks))
		}
		radios[0].Selected = selected
		radios[1].Selected = selected % 3
		if selected == 0 {
			edits[0].SetText("not a number")
		} else {
			edits[0].SetText("4")
		}
		checks[0].State = 1
		sheetDialogButton(t, buttons, true).OnClick()
		vtui.FrameManager.RemoveFrame(format)
	}
	cell := sf.Document().Cell(sf.cur.Col, sf.cur.Row)
	if cell == nil || cell.Display != sheet.DisplayHidden || !cell.Protected {
		t.Fatalf("formatted cell = %+v, want hidden and protected", cell)
	}
}

func TestSheetFindAndMenuDialogs(t *testing.T) {
	sf := newSheetFrameForTest(t)
	sf.Document().SetText(0, 0, "alpha beta")
	sf.search = sheet.SearchOptions{Pattern: "old", ByValue: true, CaseSensitive: true, WholeWords: true}

	showSheetFindDialog(sf, false)
	find := sheetDialogWindow(t)
	edits, radios, checks, buttons := sheetDialogControls(find)
	if len(edits) != 1 || len(radios) != 1 || len(checks) != 2 {
		t.Fatalf("Find controls = edits %d, radios %d, checks %d", len(edits), len(radios), len(checks))
	}
	edits[0].SetText("alpha")
	radios[0].Selected = 0
	checks[0].State = 0
	checks[1].State = 0
	sheetDialogButton(t, buttons, true).OnClick()
	vtui.FrameManager.RemoveFrame(find)

	showSheetFindDialog(sf, true)
	replace := sheetDialogWindow(t)
	edits, radios, checks, buttons = sheetDialogControls(replace)
	edits[0].SetText("alpha")
	edits[1].SetText("omega")
	radios[0].Selected = 0
	checks[0].State = 0
	checks[1].State = 0
	sheetDialogButton(t, buttons, true).OnClick()
	vtui.FrameManager.RemoveFrame(replace)
	if got := sf.Document().Cell(0, 0).Text; got != "omega beta" {
		t.Fatalf("replaced cell = %q, want omega beta", got)
	}

	showSheetMenu(sf)
	menu := sheetDialogWindow(t)
	var list *vtui.ListBox
	for _, child := range menu.GetChildren() {
		if candidate, ok := child.(*vtui.ListBox); ok {
			list = candidate
			break
		}
	}
	if list == nil || list.OnAction == nil {
		t.Fatal("sheet menu list or action is missing")
	}
	list.OnAction(-1)
	list.OnAction(len(list.Items))
	list.OnAction(0)
	vtui.FrameManager.RemoveFrame(menu)
}
