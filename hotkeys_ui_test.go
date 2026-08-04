package main

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestDialogTableUsesThemePalette(t *testing.T) {
	table := vtui.NewTable(0, 0, 20, 5, []vtui.TableColumn{{Title: "Value", Width: 20}})
	useDialogTableColors(table)

	if table.ColorTextIdx != vtui.ColDialogText ||
		table.ColorSelectedTextIdx != vtui.ColDialogSelectedButton ||
		table.ColorItemSelectTextIdx != vtui.ColDialogHighlightText ||
		table.ColorItemSelectCursorIdx != vtui.ColDialogHighlightSelectedButton ||
		table.ColorTitleIdx != vtui.ColDialogHighlightText ||
		table.ColorBoxIdx != vtui.ColDialogBox {
		t.Fatal("dialog table does not use the dialog theme palette")
	}
}

func TestHotkeyRow(t *testing.T) {
	row := hotkeyRow{
		Action:    "Test.Action",
		Label:     "Test Label",
		Area:      "Common",
		Key:       "F12",
		Condition: "EmptyCommandLine",
		Desc:      "Description",
	}

	if row.GetCellText(0) != "Test Label" {
		t.Errorf("Expected Test Label")
	}
	if row.GetCellText(1) != "F12" {
		t.Errorf("Expected F12")
	}
	if row.GetCellText(2) != "Common" {
		t.Errorf("Expected Common")
	}
	if row.GetCellText(3) != "EmptyCommandLine" {
		t.Errorf("Expected EmptyCommandLine")
	}
	if row.GetCellText(4) != "Description" {
		t.Errorf("Expected Description")
	}
}
