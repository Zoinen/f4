//go:build linux || freebsd || openbsd || netbsd || dragonfly

package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/goclip"
)

// f4#1890 (nz256): text that starts with "def" reached other programs as
// nothing, because wl-copy was left to guess its type and took it for a
// Python script. The text is now copied as text/plain.
func TestWaylandClipboardCopiesTextAsPlainText(t *testing.T) {
	dir := t.TempDir()
	argsFile, inputFile := filepath.Join(dir, "args"), filepath.Join(dir, "input")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > '" + argsFile + "'\ncat > '" + inputFile + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "wl-copy"), []byte(script), 0o700); err != nil { // #nosec G306 -- a test script that must run.
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	d, ok := goclip.GetDriver("wayland")
	if !ok {
		t.Fatal("no wayland clipboard driver")
	}
	if _, ours := d.(waylandTextClipboard); !ours {
		t.Fatalf("the wayland driver is %T, want f4's waylandTextClipboard", d)
	}
	const text = "def test():\n    pass\n"
	if err := d.WriteText(text); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(args)); len(got) != 2 || got[0] != "--type" || got[1] != waylandTextType {
		t.Errorf("wl-copy arguments = %q, want --type %s", got, waylandTextType)
	}
	input, err := os.ReadFile(inputFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(input) != text {
		t.Errorf("wl-copy got %q, want %q", input, text)
	}
}
