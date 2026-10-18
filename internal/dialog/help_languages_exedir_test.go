package dialog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/unxed/f4/internal/config"
)

// f4#1976: the help files shipped beside the binary were looked for beside
// os.Args[0], so a symlink (or a launch through $PATH) listed English only.
func TestListAvailableHelpLanguagesFindsHelpBesideTheRealBinaryThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs a privilege on Windows")
	}
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	linkDir := filepath.Join(root, "bin")
	for _, d := range []string{filepath.Join(installDir, "help"), linkDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	real := filepath.Join(installDir, "f4")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "help", "zz.hlf"), []byte(".Language=zz\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "f4")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	oldExecutable := config.Executable
	config.Executable = func() (string, error) { return link, nil }
	t.Cleanup(func() { config.Executable = oldExecutable })
	t.Setenv("F4HOME", root)

	for _, l := range ListAvailableHelpLanguages() {
		if l.Code == "zz" {
			return
		}
	}
	t.Errorf("help language zz, shipped in %s, is not listed when f4 is started through %s", filepath.Join(installDir, "help"), link)
}
