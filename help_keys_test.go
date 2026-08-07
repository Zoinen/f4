package main

import (
	"strings"
	"testing"
)

func TestGenerateKeysHelpTopic(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	topic := generateKeysHelpTopic("ViewerEditor", "Viewer & Editor Keys", []string{"Editor", "Viewer", "Common"}, "ViewerNav")

	if topic.Name != "ViewerEditor" {
		t.Errorf("Unexpected topic name %q", topic.Name)
	}
	if topic.StickyRows != 1 || len(topic.Lines) == 0 || topic.Lines[0] != "Viewer & Editor Keys" {
		t.Error("Sticky title not set correctly")
	}

	joined := strings.Join(topic.Lines, "\n")

	// Every bound editor action appears with its default key and description.
	for _, want := range []string{
		"F2             - Save file",
		"Ctrl+Z         - Undo last change",
		"Ctrl+V / Shift+Ins - Paste text from clipboard",
		"F4             - Toggle hex view",
		"Alt+Ins        - Select and copy a screen region",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("Generated topic missing line %q\n---\n%s", want, joined)
		}
	}

	// The navigation link is registered.
	if len(topic.Links) != 1 || topic.Links[0].Target != "ViewerNav" {
		t.Errorf("Expected one link to ViewerNav, got %+v", topic.Links)
	}
}

func TestGenerateKeysHelpTopic_PanelNav(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	topic := generateKeysHelpTopic("PanelNav", "Panel Keys", []string{"Shell", "Terminal", "Common"}, "ShellNav")
	joined := strings.Join(topic.Lines, "\n")

	for _, want := range []string{
		"F3             - Open file in viewer",
		// Panel.Toggle picked up Del and NumDel as aliases (#351), and
		// keysFor sorts + joins them alphabetically, so the emitted
		// prefix is now "Ctrl+O / Del / Esc / NumDel".
		"Ctrl+O / Del / Esc / NumDel - Show or hide panels",
		// Alt+F1 gained Ctrl+Shift+Left as an alias (same PR).
		"Alt+F1 / Ctrl+Shift+Left - Show the drive menu for the left panel",
		"Ctrl+PgUp      - Go to parent directory",
		"Shift+Enter    - Open current file in the system file manager",
		"Ctrl+Shift+F3 / F3 - Open terminal log in viewer",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("Generated PanelNav topic missing line %q\n---\n%s", want, joined)
		}
	}
}

func TestGenerateKeysHelpTopic_ReflectsOverrides(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	// Unbind F2: the Save line must disappear from the topic.
	GlobalHotkeysMgr.Bind("Editor", "F2", "None")
	topic := generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if strings.Contains(strings.Join(topic.Lines, "\n"), "Save file") {
		t.Error("Unbound action must not appear in the generated topic")
	}

	// Rebind to Ctrl+S: the new key must be shown.
	GlobalHotkeysMgr.Bind("Editor", "CtrlS", "Editor.Save")
	topic = generateKeysHelpTopic("ViewerEditor", "t", []string{"Editor"}, "")
	if !strings.Contains(strings.Join(topic.Lines, "\n"), "Ctrl+S") {
		t.Error("Rebound key must appear in the generated topic")
	}
}
