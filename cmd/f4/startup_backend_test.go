package main

import (
	"path/filepath"
	"testing"
)

func TestParseStartupModeRoundTrip(t *testing.T) {
	for _, mode := range []StartupMode{StartupModeAuto, StartupModeTTY, StartupModeGui} {
		if got := ParseStartupMode(mode.String()); got != mode {
			t.Errorf("ParseStartupMode(%q) = %v, want %v", mode.String(), got, mode)
		}
	}

	for _, tt := range []struct {
		value string
		want  StartupMode
	}{
		{value: "TTY", want: StartupModeTTY},
		{value: " console ", want: StartupModeTTY},
		{value: "terminal", want: StartupModeTTY},
		{value: "GUI", want: StartupModeGui},
		{value: "graphics", want: StartupModeGui},
		{value: "window", want: StartupModeGui},
		{value: "", want: StartupModeAuto},
		{value: "auto", want: StartupModeAuto},
		// A value this build does not know must degrade to auto-detection
		// rather than leave f4 with no way to start.
		{value: "holographic", want: StartupModeAuto},
	} {
		if got := ParseStartupMode(tt.value); got != tt.want {
			t.Errorf("ParseStartupMode(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestNormalizeStartupGuiBackend(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  string
	}{
		{value: "", want: ""},
		{value: "auto", want: ""},
		{value: " AUTO ", want: ""},
		{value: "gogpu", want: "gogpu"},
		{value: " GoGPU ", want: "gogpu"},
		{value: "x11", want: "x11"},
		{value: "wayland", want: "wayland"},
		{value: "ebiten", want: "ebiten"},
		// Every documented Win32 spelling collapses to one stored name.
		{value: "win32", want: "win32"},
		{value: "winapi", want: "win32"},
		{value: "gdi", want: "win32"},
		{value: "win32gui", want: "win32"},
		// External UI integrations are not part of the dialog's list, but a
		// hand-written config naming one must keep working.
		{value: "qt", want: "qt"},
		{value: "ext:myui", want: "ext:myui"},
		// Unknown names fall back to detection.
		{value: "vulkan", want: ""},
		{value: "ansi", want: ""},
	} {
		if got := normalizeStartupGuiBackend(tt.value); got != tt.want {
			t.Errorf("normalizeStartupGuiBackend(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestNormalizeStartupTTYBackend(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  string
	}{
		{value: "", want: ""},
		{value: "auto", want: ""},
		{value: "ansi", want: "ansi"},
		{value: " ANSI ", want: "ansi"},
		{value: "winapi", want: "winapi"},
		{value: "win32", want: "winapi"},
		{value: "gogpu", want: ""},
		{value: "nonsense", want: ""},
	} {
		if got := normalizeStartupTTYBackend(tt.value); got != tt.want {
			t.Errorf("normalizeStartupTTYBackend(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestResolveStartupBackendPrecedence(t *testing.T) {
	for _, tt := range []struct {
		name       string
		flagValue  string
		flagGiven  bool
		configured string
		want       string
	}{
		{name: "flag wins over config", flagValue: "x11", flagGiven: true, configured: "gogpu", want: "x11"},
		{name: "config fills in when no flag", flagValue: "", flagGiven: false, configured: "gogpu", want: "gogpu"},
		{name: "config alias is canonicalized", flagValue: "", flagGiven: false, configured: "gdi", want: "win32"},
		{name: "explicit auto overrides config", flagValue: "auto", flagGiven: true, configured: "gogpu", want: ""},
		{name: "explicit auto is case insensitive", flagValue: "Auto", flagGiven: true, configured: "gogpu", want: ""},
		{name: "nothing anywhere means detect", flagValue: "", flagGiven: false, configured: "", want: ""},
		// A mistyped flag reaches RunGui unchanged so the user sees an error,
		// while the same typo in settings.ini would have been dropped.
		{name: "flag typo is not swallowed", flagValue: "gogpuu", flagGiven: true, configured: "", want: "gogpuu"},
		{name: "config typo falls back to detect", flagValue: "", flagGiven: false, configured: "gogpuu", want: ""},
	} {
		got := resolveStartupBackend(tt.flagValue, tt.flagGiven, tt.configured, normalizeStartupGuiBackend)
		if got != tt.want {
			t.Errorf("%s: resolveStartupBackend(%q, %v, %q) = %q, want %q",
				tt.name, tt.flagValue, tt.flagGiven, tt.configured, got, tt.want)
		}
	}
}

func TestStartupChoiceHelpers(t *testing.T) {
	if got := startupModeChoiceIndex(StartupModeGui); startupModeChoices[got] != StartupModeGui {
		t.Errorf("startupModeChoiceIndex(gui) = %d, which maps back to %v", got, startupModeChoices[got])
	}
	if got := startupModeChoiceIndex(StartupMode(42)); got != 0 {
		t.Errorf("startupModeChoiceIndex(unknown) = %d, want 0 (auto)", got)
	}

	guiChoices := startupBackendChoices(startupGuiBackends)
	if len(guiChoices) != len(startupGuiBackends)+1 || guiChoices[0] != "" {
		t.Fatalf("startupBackendChoices = %q, want a leading auto entry", guiChoices)
	}
	// The helper must not alias the package-level slice it copies from.
	guiChoices[1] = "mutated"
	if startupGuiBackends[0] == "mutated" {
		t.Fatal("startupBackendChoices aliases startupGuiBackends")
	}

	choices := startupBackendChoices(startupTTYBackends)
	if got := startupBackendChoiceIndex(choices, "winapi"); choices[got] != "winapi" {
		t.Errorf("startupBackendChoiceIndex(winapi) = %d, which maps back to %q", got, choices[got])
	}
	if got := startupBackendChoiceIndex(choices, "no-such-backend"); got != 0 {
		t.Errorf("startupBackendChoiceIndex(unknown) = %d, want 0 (auto)", got)
	}

	if got := startupChoiceAt(choices, -1); got != "" {
		t.Errorf("startupChoiceAt(-1) = %q, want the auto entry", got)
	}
	if got := startupChoiceAt(choices, len(choices)); got != "" {
		t.Errorf("startupChoiceAt(out of range) = %q, want the auto entry", got)
	}
	if got := startupChoiceAt(startupModeChoices, 1); got != StartupModeTTY {
		t.Errorf("startupChoiceAt(modes, 1) = %v, want tty", got)
	}
}

func TestStartupSettingsConfigRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()

	oldCfg := AppConfig
	oldGetConfig := getUserConfigIniPath
	defer func() {
		AppConfig = oldCfg
		getUserConfigIniPath = oldGetConfig
	}()
	getUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}

	AppConfig.StartupMode = StartupModeGui
	AppConfig.GuiBackend = "gogpu"
	AppConfig.TTYBackend = "winapi"
	SaveConfig()

	LoadConfig()
	if AppConfig.StartupMode != StartupModeGui {
		t.Errorf("StartupMode = %v, want gui", AppConfig.StartupMode)
	}
	if AppConfig.GuiBackend != "gogpu" {
		t.Errorf("GuiBackend = %q, want gogpu", AppConfig.GuiBackend)
	}
	if AppConfig.TTYBackend != "winapi" {
		t.Errorf("TTYBackend = %q, want winapi", AppConfig.TTYBackend)
	}

	// Clearing the backends back to automatic selection has to survive the
	// round trip too, otherwise a user could never undo a pinned backend.
	AppConfig.StartupMode = StartupModeTTY
	AppConfig.GuiBackend = ""
	AppConfig.TTYBackend = ""
	SaveConfig()

	LoadConfig()
	if AppConfig.StartupMode != StartupModeTTY {
		t.Errorf("StartupMode = %v, want tty", AppConfig.StartupMode)
	}
	if AppConfig.GuiBackend != "" || AppConfig.TTYBackend != "" {
		t.Errorf("backends = (%q, %q), want both empty", AppConfig.GuiBackend, AppConfig.TTYBackend)
	}
}

// TestStartupConfigDefaultsAreAuto pins the out-of-the-box behavior: a config
// with no [Startup] section must behave exactly as f4 did before the section
// existed.
func TestStartupConfigDefaultsAreAuto(t *testing.T) {
	tmpDir := t.TempDir()

	oldCfg := AppConfig
	oldGetConfig := getUserConfigIniPath
	defer func() {
		AppConfig = oldCfg
		getUserConfigIniPath = oldGetConfig
	}()
	getUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}

	AppConfig.StartupMode = StartupModeGui
	AppConfig.GuiBackend = "x11"
	AppConfig.TTYBackend = "ansi"

	LoadConfig()
	if AppConfig.StartupMode != StartupModeAuto {
		t.Errorf("StartupMode = %v, want auto for an absent [Startup] section", AppConfig.StartupMode)
	}
	if AppConfig.GuiBackend != "" || AppConfig.TTYBackend != "" {
		t.Errorf("backends = (%q, %q), want both empty for an absent [Startup] section",
			AppConfig.GuiBackend, AppConfig.TTYBackend)
	}
}
