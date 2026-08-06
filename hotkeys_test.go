package main

import (
	"os"
	"path/filepath"
	"testing"
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
	os.WriteFile(iniPath, []byte(content), 0644)

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
func TestHotkeyManager_ShellDefaults_Issue289(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	cases := []struct {
		key      string
		expected string
	}{
		{"CtrlH", "Panel.ToggleHidden"},
		{"ShiftDel", "File.Delete"},
		{"ShiftNumDel", "File.Delete"},
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
