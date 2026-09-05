package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtui"
)

func TestHotkeyManager(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "hotkeys.ini")

	// Pre-populate INI file
	content := `[Shell]
F5=My.Custom.Copy
F9=Custom.Action
CtrlU=None
`
	if err := os.WriteFile(iniPath, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	hm := NewHotkeyManager(iniPath)

	// Test default existing
	if action := hm.GetAction("Shell", "F3"); action != "File.View" {
		t.Errorf("Expected F3 to be File.View, got %q", action)
	}

	// Test override
	if action := hm.GetAction("Shell", "F5"); action != "My.Custom.Copy" {
		t.Errorf("Expected F5 to be overridden as My.Custom.Copy, got %q", action)
	}

	// Test addition
	if action := hm.GetAction("Shell", "F9"); action != "Custom.Action" {
		t.Errorf("Expected F9 to be Custom.Action, got %q", action)
	}

	// Test removal
	if action := hm.GetAction("Shell", "CtrlU"); action != "None" {
		t.Errorf("Expected CtrlU to be None, got %q", action)
	}

	// Modify and save
	hm.Bind("Terminal", "AltF1", "Terminal.ShowMenu")
	hm.Save()

	// Load into a new manager
	hm2 := NewHotkeyManager(iniPath)
	if action := hm2.GetAction("Terminal", "AltF1"); action != "Terminal.ShowMenu" {
		t.Errorf("Expected saved action Terminal.ShowMenu, got %q", action)
	}
	if action := hm2.GetAction("Shell", "CtrlU"); action != "None" {
		t.Errorf("Expected CtrlU to still be None after reload, got %q", action)
	}
}

func TestHotkeyManager_CloneForEditIsTransactional(t *testing.T) {
	iniPath := filepath.Join(t.TempDir(), "hotkeys.ini")
	original := NewHotkeyManager(iniPath)
	draft := original.CloneForEdit()

	draft.Bind("Shell", "F8", "None")
	draft.Bind("Shell", "CtrlAltT", "Panel.Toggle")

	if got := original.GetAction("Shell", "F8"); got != "File.Delete" {
		t.Fatalf("editing the draft changed the runtime binding: got %q", got)
	}
	if got := original.GetAction("Shell", "CtrlAltT"); got != "" {
		t.Fatalf("editing the draft added a runtime binding: got %q", got)
	}
	if _, err := os.Stat(iniPath); !os.IsNotExist(err) {
		t.Fatalf("editing the draft persisted hotkeys before Save: stat error = %v", err)
	}

	original.ReplaceBindingsFrom(draft)
	original.Save()
	reloaded := NewHotkeyManager(iniPath)
	if got := reloaded.GetAction("Shell", "F8"); got != "None" {
		t.Errorf("committed removal = %q, want None", got)
	}
	if got := reloaded.GetAction("Shell", "CtrlAltT"); got != "Panel.Toggle" {
		t.Errorf("committed addition = %q, want Panel.Toggle", got)
	}
}

func TestHotkeyManager_GetActiveBindings(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()
	hm.Bindings = map[string]map[string]string{
		"Shell": {
			"F5":    "My.Copy",
			"CtrlO": "None", // Unbind default
		},
		"Editor": {
			"CtrlS": "File.Save",
		},
	}

	active := hm.GetActiveBindings()
	if active["Shell"]["F5"] != "My.Copy" {
		t.Errorf("Expected F5 to be My.Copy, got %v", active["Shell"]["F5"])
	}
	if _, ok := active["Shell"]["CtrlO"]; ok {
		t.Errorf("Expected CtrlO to be unbound in Shell")
	}
	if active["Editor"]["CtrlS"] != "File.Save" {
		t.Errorf("Expected Editor CtrlS to be File.Save")
	}
}
func TestFormatKeyForUI(t *testing.T) {
	tests := []struct {
		in  string
		out string
	}{
		{"F3", "F3"},
		{"CtrlO", "Ctrl+O"},
		{"ShiftF4", "Shift+F4"},
		{"CtrlShiftF5", "Ctrl+Shift+F5"},
		{"AltF12", "Alt+F12"},
		{"CtrlVK_C", "Ctrl+C"},
		{"CtrlVK_DB", "Ctrl+["},
		{"VK_C0", "`"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := FormatKeyForUI(tc.in); got != tc.out {
			t.Errorf("FormatKeyForUI(%q) = %q, expected %q", tc.in, got, tc.out)
		}
	}
}

func TestHotkeyManager_GetKeyForAction(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()
	hm.Bind("Shell", "CtrlT", "Panel.Test")

	if key := hm.GetKeyForAction("Shell", "Panel.Test"); key != "CtrlT" {
		t.Errorf("Expected CtrlT, got %q", key)
	}

	if key := hm.GetKeyForAction("Shell", "File.View"); key != "F3" {
		t.Errorf("Expected F3, got %q", key)
	}
}

func TestHotkeyManager_GetKeyForActionIsDeterministic(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.Bindings["Shell"]["CtrlShiftLeft"] = "Panel.LeftDriveMenu"
	hm.Bindings["Shell"]["AltF1"] = "Panel.LeftDriveMenu"

	for i := 0; i < 20; i++ {
		if key := hm.GetKeyForAction("Shell", "Panel.LeftDriveMenu"); key != "AltF1" {
			t.Fatalf("iteration %d returned %q, want stable AltF1", i, key)
		}
	}
}
func TestHotkeyManager_ShellDefaults_Issue289(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	cases := []struct {
		key      string
		expected string
	}{
		{"CtrlH", "Panel.ToggleHidden"},
		{"F8", "File.Delete"},
		{"ShiftDel", "File.DeletePermanent"},
		{"ShiftNumDel", "File.DeletePermanent"},
	}
	for _, tc := range cases {
		if got := hm.GetAction("Shell", tc.key); got != tc.expected {
			t.Errorf("Shell/%s: expected %q, got %q", tc.key, tc.expected, got)
		}
	}
	for _, key := range []string{"Del", "NumDel"} {
		if got := hm.Defaults["Shell"][key]; got != "Panel.Toggle:EscToggle" {
			t.Errorf("Shell/%s default: expected %q, got %q", key, "Panel.Toggle:EscToggle", got)
		}
	}
}

func TestHotkeyManager_Conditions(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	// Register a mock condition
	condValue := true
	conditionRegistry["testcond"] = func() bool { return condValue }

	hm.Bind("Shell", "F12", "Test.Action:TestCond")

	// Condition is true -> should return action
	if act := hm.GetAction("Shell", "F12"); act != "Test.Action" {
		t.Errorf("Expected Test.Action, got %q", act)
	}

	// Flip condition to false -> should return empty
	condValue = false
	if act := hm.GetAction("Shell", "F12"); act != "" {
		t.Errorf("Expected empty string when condition failed, got %q", act)
	}
}

// TestNoAltScreenApp_SimpleInline_IgnoresBackgroundTermView pins the bug from
// the Wine debug log: a second Ctrl+O while the Far-style console view was up
// did nothing at all, because the NoAltScreenApp condition consulted
// pf.termView.UseAltScreen — a leftover background object in this shell mode,
// unrelated to what's actually on screen — and treated it as a foreign
// full-screen app that should keep the key. ShellModeSimpleInline has no PTY
// and therefore no way for a foreign app to own the console view, so the
// condition must return true unconditionally once panels are hidden.
func TestNoAltScreenApp_SimpleInline_IgnoresBackgroundTermView(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.shellMode = ShellModeSimpleInline
	pf.showPanels = false
	pf.termView.UseAltScreen = true // the stray flip seen in the wild
	vtui.FrameManager.Push(pf)

	if GlobalHotkeysMgr == nil {
		GlobalHotkeysMgr = NewHotkeyManager("")
	}
	if got := GlobalHotkeysMgr.GetAction("Terminal", "CtrlO"); got != "Panel.Toggle" {
		t.Errorf("Terminal CtrlO in SimpleInline with stray UseAltScreen=true: got %q, want Panel.Toggle", got)
	}

	// The same background flag must not swallow the other Terminal-area keys.
	if got := GlobalHotkeysMgr.GetAction("Terminal", "F10"); got != "App.Quit" {
		t.Errorf("Terminal F10 in SimpleInline with stray UseAltScreen=true: got %q, want App.Quit", got)
	}

	// ShellModeOwn keeps the original gating: a real AltScreen app must still
	// win the key.
	pf.shellMode = ShellModeOwn
	if got := GlobalHotkeysMgr.GetAction("Terminal", "CtrlO"); got != "" {
		t.Errorf("Terminal CtrlO in ShellModeOwn with AltScreen app active: got %q, want empty (must fall through to app)", got)
	}
}
