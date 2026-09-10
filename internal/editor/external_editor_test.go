package editor

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/terminal"
	"os"
	"path/filepath"
	"testing"
)

func TestConfiguredExternalEditorCommand(t *testing.T) {
	oldConfig := config.App
	oldRunningGUI := gui.Running
	t.Cleanup(func() {
		config.App = oldConfig
		gui.Running = oldRunningGUI
	})

	config.App.ExternalEditorCommand = "legacy-editor"
	config.App.ExternalEditorConsole = "micro"
	config.App.ExternalEditorGUI = "gedit"

	gui.Running = false
	if got := ConfiguredExternalEditorCommand(); got != "micro" {
		t.Fatalf("console editor = %q, want micro", got)
	}

	gui.Running = true
	if got := ConfiguredExternalEditorCommand(); got != "gedit" {
		t.Fatalf("GUI editor = %q, want gedit", got)
	}

	config.App.ExternalEditorGUI = ""
	if got := ConfiguredExternalEditorCommand(); got != "legacy-editor" {
		t.Fatalf("GUI legacy fallback = %q, want legacy-editor", got)
	}

	config.App.ExternalEditorCommand = ""
	config.App.ExternalEditorConsole = ""
	gui.Running = false
	if got := ConfiguredExternalEditorCommand(); got != "" {
		t.Fatalf("empty editor configuration = %q, want empty", got)
	}
}

func TestConfiguredExternalEditorCommandIgnoresDisplayBackendInTTY(t *testing.T) {
	oldConfig := config.App
	oldRunningGUI := gui.Running
	oldProbe := terminal.ProbeGUIBackend
	t.Cleanup(func() {
		config.App = oldConfig
		gui.Running = oldRunningGUI
		terminal.ProbeGUIBackend = oldProbe
	})

	config.App.ExternalEditorConsole = "micro"
	config.App.ExternalEditorGUI = "gedit"
	gui.Running = false
	terminal.ProbeGUIBackend = func() string { return "x11" }

	if got := ConfiguredExternalEditorCommand(); got != "micro" {
		t.Fatalf("TTY editor with an available display = %q, want micro", got)
	}
}

func TestConfig_ExternalEditorCommandsRoundTripAndLegacyFallback(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "settings.ini")
	oldConfig := config.App
	oldUserPath := config.GetUserConfigIniPath
	oldConfigPaths := config.GetConfigIniPaths
	t.Cleanup(func() {
		config.App = oldConfig
		config.GetUserConfigIniPath = oldUserPath
		config.GetConfigIniPaths = oldConfigPaths
	})
	config.GetUserConfigIniPath = func() string { return iniPath }
	config.GetConfigIniPaths = func() []string { return []string{iniPath} }

	config.App.ExternalEditorCommand = "legacy-editor"
	config.App.ExternalEditorConsole = "micro"
	config.App.ExternalEditorGUI = "gedit"
	config.SaveConfig()

	config.App.ExternalEditorCommand = ""
	config.App.ExternalEditorConsole = ""
	config.App.ExternalEditorGUI = ""
	config.LoadConfig()
	if config.App.ExternalEditorConsole != "micro" || config.App.ExternalEditorGUI != "gedit" {
		t.Fatalf("split editor commands after round trip: console=%q GUI=%q", config.App.ExternalEditorConsole, config.App.ExternalEditorGUI)
	}

	if err := os.WriteFile(iniPath, []byte("[Editor]\nExternalEditorCommand = old-editor\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config.LoadConfig()
	if config.App.ExternalEditorConsole != "old-editor" || config.App.ExternalEditorGUI != "old-editor" {
		t.Fatalf("legacy editor command migration: console=%q GUI=%q", config.App.ExternalEditorConsole, config.App.ExternalEditorGUI)
	}
}
