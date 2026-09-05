package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestDialog_BuildMsgBox(t *testing.T) {
	vtui.SetDefaultPalette()
	var res dialogResult
	cfg := dialogConfig{
		title:   "Test Info",
		msgbox:  "Hello from test",
		okLabel: "&Ok",
	}

	win := buildDialog(cfg, 80, 25, &res)
	if win == nil {
		t.Fatal("buildDialog returned nil for msgbox")
	}

	vtui.AssertLayout(t, win)
}

func TestDialog_BuildInputBox(t *testing.T) {
	vtui.SetDefaultPalette()
	var res dialogResult
	cfg := dialogConfig{
		title:       "Question",
		inputbox:    "Enter value:",
		okLabel:     "&Submit",
		cancelLabel: "Cancel",
		width:       44,
		height:      9,
	}

	win := buildDialog(cfg, 80, 25, &res)
	if win == nil {
		t.Fatal("buildDialog returned nil for inputbox")
	}

	vtui.AssertLayout(t, win)
}

func TestDialog_BuildMenu(t *testing.T) {
	vtui.SetDefaultPalette()
	var res dialogResult
	cfg := dialogConfig{
		title:       "Select Option",
		menuTitle:   "Menu",
		menuItems:   []string{"opt1", "Option 1", "opt2", "Option 2"},
		okLabel:     "&Select",
		cancelLabel: "Cancel",
		width:       40,
		height:      12,
	}

	win := buildDialog(cfg, 80, 25, &res)
	if win == nil {
		t.Fatal("buildDialog returned nil for menu")
	}

	vtui.AssertLayout(t, win)
}

func TestDialog_BuildVui(t *testing.T) {
	vtui.SetDefaultPalette()
	var res dialogResult

	tmpDir := t.TempDir()
	vuiPath := filepath.Join(tmpDir, "form.vui")
	vuiContent := `{
		"vuiVersion": 1,
		"root": {
			"type": "Dialog",
			"id": "formDlg",
			"props": { "title": " User Form " },
			"children": [
				{ "type": "Edit", "id": "username", "props": { "text": "alice" } },
				{ "type": "Checkbox", "id": "isAdmin", "props": { "text": "Administrator", "state": 1 } },
				{ "type": "Button", "id": "submitBtn", "props": { "text": "&Save", "command": 2 } }
			]
		}
	}`
	_ = os.WriteFile(vuiPath, []byte(vuiContent), 0644)

	cfg := dialogConfig{
		vuiPath: vuiPath,
	}

	win := buildDialog(cfg, 80, 25, &res)
	if win == nil {
		t.Fatal("buildDialog returned nil for .vui")
	}

	// Simulate dialog close
	win.SetExitCode(0)

	if res.Values["username"] != "alice" {
		t.Errorf("Expected username 'alice', got %v", res.Values["username"])
	}
	if res.Values["isAdmin"] != 1 {
		t.Errorf("Expected isAdmin 1, got %v", res.Values["isAdmin"])
	}
}
