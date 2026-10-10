package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/panel"
)

func TestStartupDirsForArgumentForms(t *testing.T) {
	cwd := filepath.Join("workspace", "f4")
	if left, right := startupDirsFor(cwd, nil); left != cwd || right != "" {
		t.Fatalf("no startup dirs = %q, %q", left, right)
	}
	if left, right := startupDirsFor(cwd, []string{"left"}); left != filepath.Join(cwd, "left") || right != cwd {
		t.Fatalf("one startup dir = %q, %q", left, right)
	}
	if left, right := startupDirsFor(cwd, []string{"left", "right", "ignored"}); left != filepath.Join(cwd, "left") || right != filepath.Join(cwd, "right") {
		t.Fatalf("two startup dirs = %q, %q", left, right)
	}
}

func TestStartupDirsOverrideAndChoice(t *testing.T) {
	cwd := filepath.Join("workspace", "f4")
	if left, right, ok := startupDirsOverride(cwd, nil, false); ok || left != "" || right != "" {
		t.Fatalf("disabled plain override = %q, %q, %v", left, right, ok)
	}
	if left, right, ok := startupDirsOverride(cwd, nil, true); !ok || left != cwd || right != "" {
		t.Fatalf("enabled plain override = %q, %q, %v", left, right, ok)
	}
	if left, right, ok := startupDirsChoice(cwd, []string{"left"}, true); !ok || left != filepath.Join(cwd, "left") || right != cwd {
		t.Fatalf("current-folder choice = %q, %q, %v", left, right, ok)
	}
	if left, right, ok := startupDirsChoice(cwd, []string{"left"}, false); !ok || left != filepath.Join(cwd, "left") || right != panel.StartupKeepPanel {
		t.Fatalf("far choice = %q, %q, %v", left, right, ok)
	}
}

func TestFarStartupDirsArgumentForms(t *testing.T) {
	cwd := filepath.Join("workspace", "f4")
	if _, _, ok := farStartupDirs(cwd, nil); ok {
		t.Fatal("far startup without paths unexpectedly succeeded")
	}
	if left, right, ok := farStartupDirs(cwd, []string{"left"}); !ok || left != filepath.Join(cwd, "left") || right != panel.StartupKeepPanel {
		t.Fatalf("far one path = %q, %q, %v", left, right, ok)
	}
	if left, right, ok := farStartupDirs(cwd, []string{"left", "right"}); !ok || left != filepath.Join(cwd, "left") || right != filepath.Join(cwd, "right") {
		t.Fatalf("far two paths = %q, %q, %v", left, right, ok)
	}
}

func TestStartupDirArgsStopsAtSwitchAndSupportsSeparator(t *testing.T) {
	args := []string{"left", "right", "--gui", "wayland", "--", "after", "tail"}
	if got, want := startupDirArgs(args), []string{"left", "right", "after", "tail"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("startupDirArgs with separator = %#v, want %#v", got, want)
	}
	if got := startupDirArgs([]string{"--tty", "ansi", "ignored"}); len(got) != 0 {
		t.Fatalf("startupDirArgs after switch = %#v, want empty", got)
	}
}

func TestStartupViewFilesSelectsExistingFiles(t *testing.T) {
	cwd := t.TempDir()
	file := filepath.Join(cwd, "note.txt")
	if err := os.WriteFile(file, []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, want := startupViewFiles(cwd, []string{"note.txt", "missing.txt", "."}), []string{file}; len(got) != 1 || got[0] != want[0] {
		t.Fatalf("startup view files = %#v, want %#v", got, want)
	}
}

func TestResolveStartupPathCleansRelativeAndAbsolutePaths(t *testing.T) {
	cwd := filepath.Join("workspace", "f4")
	if got, want := resolveStartupPath(cwd, filepath.Join("notes", "..", "note.txt")), filepath.Join(cwd, "note.txt"); got != want {
		t.Fatalf("relative startup path = %q, want %q", got, want)
	}
	abs, err := filepath.Abs(filepath.Join(cwd, "..", "other", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resolveStartupPath(cwd, abs), filepath.Clean(abs); got != want {
		t.Fatalf("absolute startup path = %q, want %q", got, want)
	}
}

func TestSudoDispatcherPathForms(t *testing.T) {
	if got := sudoDispatcherPath([]string{"--sudo-dispatcher", "pipe"}); got != "pipe" {
		t.Fatalf("separate dispatcher path = %q", got)
	}
	if got := sudoDispatcherPath([]string{"--sudo-dispatcher=inline"}); got != "inline" {
		t.Fatalf("inline dispatcher path = %q", got)
	}
	if got := sudoDispatcherPath([]string{"--sudo-dispatcher"}); got != "" {
		t.Fatalf("missing dispatcher path = %q", got)
	}
}

func TestSudoStartupModePrefersDispatcher(t *testing.T) {
	if dispatcher, askpass := sudoStartupMode([]string{"--sudo-dispatcher", "pipe"}, true); dispatcher != "pipe" || askpass {
		t.Fatalf("dispatcher mode = %q, %v", dispatcher, askpass)
	}
	if dispatcher, askpass := sudoStartupMode(nil, true); dispatcher != "" || !askpass {
		t.Fatalf("askpass mode = %q, %v", dispatcher, askpass)
	}
}

func TestFormatVersionSHAShortsStandaloneSequences(t *testing.T) {
	if got, want := formatVersionSHA("build 12345678 deadbeefz"), "build 1234567 deadbeez"; got != want {
		t.Fatalf("formatted version = %q, want %q", got, want)
	}
	if got := formatVersionSHA("abc12345678"); got != "abc12345678" {
		t.Fatalf("embedded SHA sequence was shortened: %q", got)
	}
}

func TestHexHelpers(t *testing.T) {
	if !isHexSequence([]rune("deadbeef")) || isHexSequence([]rune("deadbee!")) {
		t.Fatal("hex sequence classification is incorrect")
	}
	for _, r := range []rune{'0', '9', 'a', 'f'} {
		if !isHexChar(r) {
			t.Fatalf("%q was not recognized as hex", r)
		}
	}
	for _, r := range []rune{'A', 'g', ' '} {
		if isHexChar(r) {
			t.Fatalf("%q was incorrectly recognized as hex", r)
		}
	}
}
