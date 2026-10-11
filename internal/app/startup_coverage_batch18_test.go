package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/panel"
)

func TestStartupDirsForNoArguments(t *testing.T) {
	left, right := startupDirsFor("/work", nil)
	if left != "/work" || right != "" {
		t.Fatalf("startupDirsFor without arguments = %q, %q", left, right)
	}
}

func TestStartupDirsForOneRelativeArgument(t *testing.T) {
	left, right := startupDirsFor("/work", []string{"sub/../files"})
	if left != filepath.Join("/work", "files") || right != "/work" {
		t.Fatalf("startupDirsFor with one relative argument = %q, %q", left, right)
	}
}

func TestStartupDirsForTwoArguments(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("other", "path"))
	if err != nil {
		t.Fatal(err)
	}
	left, right := startupDirsFor("/work", []string{"left", abs, "ignored"})
	if left != filepath.Join("/work", "left") || right != filepath.Clean(abs) {
		t.Fatalf("startupDirsFor with two arguments = %q, %q", left, right)
	}
}

func TestStartupDirsOverridePlainStart(t *testing.T) {
	if left, right, ok := startupDirsOverride("/work", nil, false); ok || left != "" || right != "" {
		t.Fatalf("plain start with the style disabled = %q, %q, %v", left, right, ok)
	}
	left, right, ok := startupDirsOverride("/work", nil, true)
	if !ok || left != "/work" || right != "" {
		t.Fatalf("plain start with the style enabled = %q, %q, %v", left, right, ok)
	}
}

func TestFarStartupDirsKeepsPassivePanel(t *testing.T) {
	left, right, ok := farStartupDirs("/work", []string{"docs"})
	if !ok || left != filepath.Join("/work", "docs") || right != panel.StartupKeepPanel {
		t.Fatalf("far startup with one directory = %q, %q, %v", left, right, ok)
	}
	left, right, ok = farStartupDirs("/work", []string{"left", "right"})
	if !ok || left != filepath.Join("/work", "left") || right != filepath.Join("/work", "right") {
		t.Fatalf("far startup with two directories = %q, %q, %v", left, right, ok)
	}
}

func TestStartupDirsChoiceSelectsStyle(t *testing.T) {
	left, right, ok := startupDirsChoice("/work", []string{"left"}, true)
	if !ok || left != filepath.Join("/work", "left") || right != "/work" {
		t.Fatalf("current-folder startup choice = %q, %q, %v", left, right, ok)
	}
	left, right, ok = startupDirsChoice("/work", []string{"left"}, false)
	if !ok || left != filepath.Join("/work", "left") || right != panel.StartupKeepPanel {
		t.Fatalf("far startup choice = %q, %q, %v", left, right, ok)
	}
}

func TestStartupDirArgsHandlesSwitchesAndSeparator(t *testing.T) {
	args := []string{"left", "--gui", "tty", "right", "--", "after", "second"}
	want := []string{"left", "after", "second"}
	got := startupDirArgs(args)
	if len(got) != len(want) {
		t.Fatalf("startupDirArgs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("startupDirArgs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolveStartupPathCleansRelativePath(t *testing.T) {
	if got := resolveStartupPath("/work", "./dir/../file.txt"); got != filepath.Join("/work", "file.txt") {
		t.Fatalf("resolveStartupPath relative = %q", got)
	}
	abs, err := filepath.Abs(filepath.Join("tmp", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolveStartupPath("/work", abs); got != filepath.Clean(abs) {
		t.Fatalf("resolveStartupPath absolute = %q", got)
	}
}

func TestStartupViewFilesSelectsExistingRelativeFile(t *testing.T) {
	tmp := t.TempDir()
	file := filepath.Join(tmp, "note.txt")
	dir := filepath.Join(tmp, "folder")
	if err := os.WriteFile(file, []byte("note"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	got := startupViewFiles(tmp, []string{"note.txt", "folder", "missing.txt"})
	if len(got) != 1 || got[0] != file {
		t.Fatalf("startupViewFiles = %#v, want %#v", got, []string{file})
	}
}

func TestPanelsFrameVisualSidesAndMenus(t *testing.T) {
	pf := settingsCoveragePanel(t)
	if pf.VisualLeftFSP() == nil || pf.VisualRightFSP() == nil {
		t.Fatal("panels frame did not expose both visual filesystem panels")
	}
	if len(pf.LeftMenu().SubItems) == 0 || len(pf.RightMenu().SubItems) == 0 {
		t.Fatal("panels frame side menus are empty")
	}
	if len(pf.BuildMenuItems()) == 0 || pf.GetKeyLabels() == nil {
		t.Fatal("panels frame did not build its main menu and key labels")
	}
}
