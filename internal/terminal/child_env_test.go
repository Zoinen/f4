package terminal

import (
	"os"
	"strings"
	"testing"
)

func TestTerminalChildEnvGUIAdvertisesInteractiveTerminal(t *testing.T) {
	oldMode, oldBackend := currentHostShellMode, ProbeGUIBackend
	t.Cleanup(func() {
		currentHostShellMode, ProbeGUIBackend = oldMode, oldBackend
	})
	tests := []struct {
		name    string
		mode    ShellMode
		backend string
		term    string
		want    string
	}{
		{name: "Qt dumb launcher", mode: ShellModeOwn, backend: "qt", term: "dumb", want: "xterm-256color"},
		{name: "Qt unknown launcher", mode: ShellModeOwn, backend: "qt", term: "unknown", want: "xterm-256color"},
		{name: "Qt mixed case", mode: ShellModeOwn, backend: "qt", term: "DUMB", want: "xterm-256color"},
		{name: "Qt existing terminal", mode: ShellModeOwn, backend: "qt", term: "screen-256color", want: "screen-256color"},
		{name: "console emulator", mode: ShellModeOwn, term: "dumb", want: "dumb"},
		{name: "host passthrough", mode: ShellModeHost, backend: "qt", term: "dumb", want: "dumb"},
		{name: "captured output", mode: ShellModeSimpleCaptured, backend: "qt", term: "dumb", want: "dumb"},
		{name: "inline output", mode: ShellModeSimpleInline, backend: "qt", term: "dumb", want: "dumb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", tt.term)
			t.Setenv("NO_COLOR", "1")
			currentHostShellMode = func() ShellMode { return tt.mode }
			ProbeGUIBackend = func() string { return tt.backend }
			env := TerminalChildEnv()
			values := make(map[string]string)
			for _, entry := range env {
				key, value, ok := strings.Cut(entry, "=")
				if ok {
					values[key] = value
				}
			}
			if values["TERM"] != tt.want {
				t.Errorf("child TERM = %q, want %q", values["TERM"], tt.want)
			}
			if values["NO_COLOR"] != "1" {
				t.Error("explicit NO_COLOR preference was changed")
			}
			if os.Getenv("TERM") != tt.term {
				t.Error("terminal capability normalization changed the parent environment")
			}
		})
	}
}
