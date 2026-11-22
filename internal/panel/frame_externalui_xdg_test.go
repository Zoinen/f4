//go:build (linux && !android) || freebsd || dragonfly || openbsd || netbsd || solaris || hurd

package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAssociatedPathUsesGIOWhenXDGOpenIsMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	output := filepath.Join(dir, "args")
	t.Setenv("F4_TEST_ASSOCIATION_ARGS", output)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$F4_TEST_ASSOCIATION_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "gio"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "folder with spaces #1")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := openAssociatedPathOS(path, dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if want := "open\n" + path + "\n"; string(got) != want {
		t.Fatalf("gio arguments = %q, want %q", got, want)
	}
}
