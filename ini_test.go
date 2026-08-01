package main

import (
	"os"
	"testing"
)

func TestIniFile_LoadAndGet(t *testing.T) {
	tmpFile := "test_config.ini"
	defer os.Remove(tmpFile)
	content := `
[Settings]
Theme = Dark
ShowHidden = 1

[Colors]
PanelText = F_WHITE | B_BLACK
`
	os.WriteFile(tmpFile, []byte(content), 0644)

	ini := LoadIni(tmpFile)
	if ini.GetString("Settings", "Theme", "Light") != "Dark" {
		t.Errorf("Expected Theme=Dark, got %s", ini.GetString("Settings", "Theme", "Light"))
	}
	if ini.GetString("Settings", "ShowHidden", "0") != "1" {
		t.Errorf("Expected ShowHidden=1, got %s", ini.GetString("Settings", "ShowHidden", "0"))
	}
	if ini.GetString("Colors", "PanelText", "") != "F_WHITE | B_BLACK" {
		t.Errorf("Expected PanelText=F_WHITE | B_BLACK, got %s", ini.GetString("Colors", "PanelText", ""))
	}
	if ini.GetString("Missing", "Key", "Default") != "Default" {
		t.Errorf("Expected default value for missing key")
	}
}

func TestIniFile_MissingFile(t *testing.T) {
	// Should not panic, just return empty config
	ini := LoadIni("non_existent_file.ini")
	if ini.GetString("Any", "Key", "Fallback") != "Fallback" {
		t.Errorf("Expected fallback value on missing file")
	}
}

func TestIniFile_EnvOverride(t *testing.T) {
	ini := newIniFile()

	// 1. Set environment variables
	t.Setenv("F4_PANEL_SHOW_HIDDEN_FILES", "0")
	t.Setenv("f4_system_confirm_exit", "1")

	// 2. Read values (should prioritize env)
	val1 := ini.GetString("Panel", "ShowHiddenFiles", "default")
	if val1 != "0" {
		t.Errorf("Expected env override '0', got %q", val1)
	}

	val2 := ini.GetString("System", "ConfirmExit", "default")
	if val2 != "1" {
		t.Errorf("Expected env override '1', got %q", val2)
	}

	// 3. Fallback to default/file if env is empty
	val3 := ini.GetString("Panel", "HighlightDir", "default_val")
	if val3 != "default_val" {
		t.Errorf("Expected default 'default_val', got %q", val3)
	}
}
