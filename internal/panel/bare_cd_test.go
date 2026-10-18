package panel

import (
	"runtime"
	"testing"
)

// A bare "cd" goes to the home directory in a Unix shell, so the panel has to
// follow it the way it follows "cd ~" (f4#1980): the shell used to change
// directory while the panel stayed where it was.
func TestParseDirChangeCommand_BareCdGoesHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cmd.exe only prints the current directory for a bare cd")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, cmd := range []string{"cd", "CD"} {
		target, ok := parseDirChangeCommand(cmd)
		if !ok {
			t.Fatalf("parseDirChangeCommand(%q) is not a directory change: the panel would not follow the shell", cmd)
		}
		if got := ExpandPathEnv(target); got != home {
			t.Fatalf("parseDirChangeCommand(%q) leads to %q, want the home directory %q", cmd, got, home)
		}
	}
	if _, ok := parseDirChangeCommand("cdx"); ok {
		t.Fatal(`"cdx" was taken for a directory change`)
	}
}
