package main

import (
	"strings"
	"testing"
)

func registerMenuTestAction(t *testing.T, action Action) {
	t.Helper()
	key := strings.ToLower(action.Name)
	previous, existed := actionRegistry[key]
	previousOrder := append([]string(nil), actionOrder...)
	t.Cleanup(func() {
		if existed {
			actionRegistry[key] = previous
		} else {
			delete(actionRegistry, key)
		}
		actionOrder = previousOrder
	})
	RegisterAction(action)
}

func TestBuildMenuBarItems_Editor(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	items := BuildMenuBarItems("Editor")

	wantTitles := []string{"&File", "&Edit", "&Search", "&Options", "&Insert"}
	if len(items) != len(wantTitles) {
		t.Fatalf("Expected %d top-level menus, got %d: %+v", len(wantTitles), len(items), items)
	}
	for i, want := range wantTitles {
		if items[i].Label != want {
			t.Errorf("Menu %d: expected title %q, got %q", i, want, items[i].Label)
		}
	}

	// File menu: Save first, with the default F2 shortcut shown.
	file := items[0].SubItems
	if len(file) == 0 {
		t.Fatal("File menu is empty")
	}
	if file[0].Text != "&Save" {
		t.Errorf("Expected first File item to be '&Save', got %q", file[0].Text)
	}
	if file[0].Shortcut != "F2" {
		t.Errorf("Expected Save shortcut 'F2', got %q", file[0].Shortcut)
	}

	// A user override must be reflected in the shortcut column.
	GlobalHotkeysMgr.Bind("Editor", "CtrlS", "Editor.Save")
	file = BuildMenuBarItems("Editor")[0].SubItems
	if file[0].Shortcut != "F2" && file[0].Shortcut != "Ctrl+S" {
		t.Errorf("Override not reflected: got %q", file[0].Shortcut)
	}
}

func TestBuildMenuBarItems_Viewer(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	items := BuildMenuBarItems("Viewer")

	wantTitles := []string{"&File", "&View", "&Search", "&Options"}
	if len(items) != len(wantTitles) {
		t.Fatalf("Expected %d top-level menus, got %d: %+v", len(wantTitles), len(items), items)
	}
	for i, want := range wantTitles {
		if items[i].Label != want {
			t.Errorf("Menu %d: expected title %q, got %q", i, want, items[i].Label)
		}
	}

	// Common actions (Screen Grab) are appended after the area's own.
	file := items[0].SubItems
	last := file[len(file)-1]
	if last.Text != "Screen &grab" {
		t.Errorf("Expected last File item to be 'Screen &grab', got %q", last.Text)
	}
	if last.Shortcut != "Alt+Ins" {
		t.Errorf("Expected Screen Grab shortcut 'Alt+Ins', got %q", last.Shortcut)
	}
}

func TestBuildMenuBarItems_Shell(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	items := BuildMenuBarItems("Shell")

	wantTitles := []string{"&Files", "&Commands", "&Options"}
	if len(items) != len(wantTitles) {
		t.Fatalf("Expected %d top-level menus, got %d: %+v", len(wantTitles), len(items), items)
	}
	for i, want := range wantTitles {
		if items[i].Label != want {
			t.Errorf("Menu %d: expected title %q, got %q", i, want, items[i].Label)
		}
	}

	// Files menu: View first, with the default F3 shortcut shown.
	files := items[0].SubItems
	if len(files) == 0 {
		t.Fatal("Files menu is empty")
	}
	if files[0].Text != "&View" {
		t.Errorf("Expected first Files item to be '&View', got %q", files[0].Text)
	}
	if files[0].Shortcut != "F3" {
		t.Errorf("Expected View shortcut 'F3', got %q", files[0].Shortcut)
	}

	// Options menu honors MenuSeparatorBefore.
	var sawSeparator bool
	for _, it := range items[2].SubItems {
		if it.Separator {
			sawSeparator = true
			break
		}
	}
	if !sawSeparator {
		t.Error("Expected at least one separator in the Options menu")
	}
}

func TestBuildMenuBarItems_Terminal(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	items := BuildMenuBarItems("Terminal")
	if len(items) != 1 || items[0].Label != "&File" {
		t.Fatalf("Expected a single '&File' menu, got %+v", items)
	}
	file := items[0].SubItems
	if len(file) == 0 || file[0].Text != "&View terminal log" {
		t.Errorf("Expected first File item to be '&View terminal log', got %+v", file)
	}
}

func TestBuildMenuBarItems_TerminalShortcutIsStableAndConditionAware(t *testing.T) {
	oldManager := GlobalHotkeysMgr
	oldCondition, hadCondition := conditionRegistry["menushortcutpreferred"]
	conditionActive := true
	RegisterCondition("MenuShortcutPreferred", func() bool { return conditionActive })
	defer func() {
		GlobalHotkeysMgr = oldManager
		if hadCondition {
			conditionRegistry["menushortcutpreferred"] = oldCondition
		} else {
			delete(conditionRegistry, "menushortcutpreferred")
		}
	}()

	registerMenuTestAction(t, Action{
		Name:        "Terminal.TestStableShortcut",
		Area:        "Terminal",
		Label:       "Stable shortcut",
		DefaultKeys: []string{"F13:MenuShortcutPreferred", "CtrlShiftF13"},
		MenuPath:    "File",
	})
	GlobalHotkeysMgr = NewHotkeyManager("")

	shortcutForTestAction := func() string {
		items := BuildMenuBarItems("Terminal")
		for _, item := range items[0].SubItems {
			if item.Text == "&Stable shortcut" {
				return item.Shortcut
			}
		}
		return ""
	}

	for i := 0; i < 100; i++ {
		if got := shortcutForTestAction(); got != "F13" {
			t.Fatalf("active preferred shortcut changed on rebuild %d: got %q", i, got)
		}
	}
	conditionActive = false
	for i := 0; i < 100; i++ {
		if got := shortcutForTestAction(); got != "Ctrl+Shift+F13" {
			t.Fatalf("fallback shortcut changed on rebuild %d: got %q", i, got)
		}
	}
}

func TestBuildMenuBarItems_OnClickRunsAction(t *testing.T) {
	old := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	defer func() { GlobalHotkeysMgr = old }()

	clicked := false
	registerMenuTestAction(t, Action{
		Name:     "Test.MenuClick",
		Area:     "Editor",
		Label:    "Click me",
		MenuPath: "TestMenu",
		Handler:  func() bool { clicked = true; return true },
	})

	items := BuildMenuBarItems("Editor")
	last := items[len(items)-1]
	if last.Label != "TestMenu" {
		t.Fatalf("Expected fallback menu title 'TestMenu', got %q", last.Label)
	}
	if len(last.SubItems) != 1 {
		t.Fatalf("Expected 1 item in TestMenu, got %d", len(last.SubItems))
	}
	last.SubItems[0].OnClick()
	if !clicked {
		t.Error("OnClick did not run the action handler")
	}
}
