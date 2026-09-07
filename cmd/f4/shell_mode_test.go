package main

import (
	"testing"
)

func TestResolveShellMode_Matrix(t *testing.T) {
	oldProbeGUI := probeGUIBackend
	oldProbeTTY := probeHostTTY
	oldProbePTY := probePTYUsable
	oldProbeGOOS := probeGOOS
	defer func() {
		probeGUIBackend = oldProbeGUI
		probeHostTTY = oldProbeTTY
		probePTYUsable = oldProbePTY
		probeGOOS = oldProbeGOOS
	}()

	tests := []struct {
		name        string
		cfg         ShellModeConfig
		ptyUsable   bool
		hostTTY     bool
		guiBackend  string
		goos        string
		wantMode    ShellMode
		wantModeStr string
	}{
		{
			name:        "PTY unusable + Host TTY + Windows -> SimpleInline",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   false,
			hostTTY:     true,
			guiBackend:  "",
			goos:        "windows",
			wantMode:    ShellModeSimpleInline,
			wantModeStr: "simple-inline",
		},
		{
			name:        "PTY unusable + Host TTY + Linux -> SimpleCaptured",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   false,
			hostTTY:     true,
			guiBackend:  "",
			goos:        "linux",
			wantMode:    ShellModeSimpleCaptured,
			wantModeStr: "simple-captured",
		},
		{
			name:        "PTY unusable + GUI -> SimpleCaptured",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   false,
			hostTTY:     false,
			guiBackend:  "x11",
			goos:        "linux",
			wantMode:    ShellModeSimpleCaptured,
			wantModeStr: "simple-captured",
		},
		{
			name:        "PTY usable + Config own + Host TTY -> Own",
			cfg:         ShellModeConfig{ConsoleMode: "own"},
			ptyUsable:   true,
			hostTTY:     true,
			guiBackend:  "",
			goos:        "linux",
			wantMode:    ShellModeOwn,
			wantModeStr: "own",
		},
		{
			name:        "PTY usable + Config host + GUI backend -> Own",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   true,
			hostTTY:     true,
			guiBackend:  "gogpu",
			goos:        "linux",
			wantMode:    ShellModeOwn,
			wantModeStr: "own",
		},
		{
			name:        "PTY usable + Config host + No Host TTY -> Own",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   true,
			hostTTY:     false,
			guiBackend:  "",
			goos:        "linux",
			wantMode:    ShellModeOwn,
			wantModeStr: "own",
		},
		{
			name:        "PTY usable + Config host + Host TTY + No GUI -> Host",
			cfg:         ShellModeConfig{ConsoleMode: "host"},
			ptyUsable:   true,
			hostTTY:     true,
			guiBackend:  "",
			goos:        "linux",
			wantMode:    ShellModeHost,
			wantModeStr: "host",
		},
		{
			name:        "PTY usable + Config host (case insensitive) + Host TTY -> Host",
			cfg:         ShellModeConfig{ConsoleMode: "HoSt"},
			ptyUsable:   true,
			hostTTY:     true,
			guiBackend:  "",
			goos:        "windows",
			wantMode:    ShellModeHost,
			wantModeStr: "host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probePTYUsable = func() bool { return tt.ptyUsable }
			probeHostTTY = func() bool { return tt.hostTTY }
			probeGUIBackend = func() string { return tt.guiBackend }
			probeGOOS = func() string { return tt.goos }

			got := resolveShellMode(tt.cfg)
			if got != tt.wantMode {
				t.Errorf("resolveShellMode() = %v (%s), want %v (%s)", got, got.String(), tt.wantMode, tt.wantModeStr)
			}
			if got.String() != tt.wantModeStr {
				t.Errorf("ShellMode.String() = %q, want %q", got.String(), tt.wantModeStr)
			}
		})
	}
}

func TestConsoleViewStyleOf(t *testing.T) {
	cases := []struct {
		name string
		cfg  ShellModeConfig
		want string
	}{
		{"default", ShellModeConfig{ConsoleMode: "own"}, ConsoleViewOwn},
		{"empty", ShellModeConfig{}, ConsoleViewOwn},
		{"far", ShellModeConfig{ConsoleMode: "far"}, ConsoleViewFar},
		{"mc", ShellModeConfig{ConsoleMode: "mc"}, ConsoleViewMc},
		{"legacy host with overlay", ShellModeConfig{ConsoleMode: "host", ConsoleOverlayUI: true}, ConsoleViewFar},
		{"legacy host without overlay", ShellModeConfig{ConsoleMode: "host"}, ConsoleViewMc},
		{"case insensitive", ShellModeConfig{ConsoleMode: "FAR"}, ConsoleViewFar},
	}
	for _, c := range cases {
		if got := consoleViewStyleOf(c.cfg); got != c.want {
			t.Errorf("%s: consoleViewStyleOf() = %q, want %q", c.name, got, c.want)
		}
	}
}

// Without a PTY the "own terminal" choice cannot be honoured, and leaving it in
// place is what used to give users a blank screen on Ctrl+O.
func TestConsoleViewStyleFor_OwnDegradesWithoutPTY(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.ConsoleMode = ConsoleViewOwn
	AppConfig.ConsoleOverlayUI = false

	if got := consoleViewStyleFor(ShellModeSimpleInline); got != ConsoleViewFar {
		t.Errorf("consoleViewStyleFor(SimpleInline) = %q, want %q", got, ConsoleViewFar)
	}
	if got := consoleViewStyleFor(ShellModeOwn); got != ConsoleViewOwn {
		t.Errorf("consoleViewStyleFor(Own) = %q, want %q", got, ConsoleViewOwn)
	}
}
func TestResolveShellMode_LegacyWindowsDegradesToSimpleInlineAndFarOverlay(t *testing.T) {
	oldProbeGUI := probeGUIBackend
	oldProbeTTY := probeHostTTY
	oldProbePTY := probePTYUsable
	oldProbeGOOS := probeGOOS
	defer func() {
		probeGUIBackend = oldProbeGUI
		probeHostTTY = oldProbeTTY
		probePTYUsable = oldProbePTY
		probeGOOS = oldProbeGOOS
	}()

	probeGUIBackend = func() string { return "" }
	probeHostTTY = func() bool { return true }
	probePTYUsable = func() bool { return false } // Simulates Windows 7 / 8 / 8.1 without ConPTY
	probeGOOS = func() string { return "windows" }

	mode := resolveShellMode(ShellModeConfig{ConsoleMode: "own"})
	if mode != ShellModeSimpleInline {
		t.Fatalf("resolveShellMode on legacy Windows = %v, want ShellModeSimpleInline", mode)
	}

	style := consoleViewStyleFor(mode)
	if style != ConsoleViewFar {
		t.Fatalf("consoleViewStyleFor(SimpleInline) = %q, want %q (Far overlay)", style, ConsoleViewFar)
	}
}
