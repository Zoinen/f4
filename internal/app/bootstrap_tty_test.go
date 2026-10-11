//go:build tty_only

package app

import (
	"testing"

	"github.com/unxed/f4/internal/terminal"
)

func TestTTYOnlyAutoStartupUsesTerminalWithDisplay(t *testing.T) {
	oldProbeTTY := terminal.ProbeHostTTY
	t.Cleanup(func() { terminal.ProbeHostTTY = oldProbeTTY })
	terminal.ProbeHostTTY = func() bool { return false }
	t.Setenv("DISPLAY", ":0")
	if shouldTryGui() {
		t.Fatal("TTY-only build attempted GUI startup")
	}
}
