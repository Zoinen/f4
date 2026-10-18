//go:build darwin

package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/goclip"
)

// f4#2057 (sogonov): f4 --gui pasted Cyrillic as "?", because pbpaste ran
// with no UTF-8 locale. pbpaste and pbcopy now run with LANG=en_US.UTF-8 and
// no other locale variable.
func TestDarwinClipboardRunsPbcopyWithAUTF8Locale(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "env")
	pbpaste := "#!/bin/sh\nenv | grep -E '^(LANG|LC_[A-Z]+)=' > '" + envFile + "'\nprintf 'Лунобот'\n"
	pbcopy := "#!/bin/sh\ncat > '" + filepath.Join(dir, "input") + "'\n"
	for name, script := range map[string]string{"pbpaste": pbpaste, "pbcopy": pbcopy} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil { // #nosec G306 -- a test script that must run.
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	t.Setenv("LC_MESSAGES", "C")

	d, ok := goclip.GetDriver("darwin")
	if !ok {
		t.Fatal("no darwin clipboard driver")
	}
	if _, ours := d.(darwinTextClipboard); !ours {
		t.Fatalf("the darwin driver is %T, want f4's darwinTextClipboard", d)
	}
	if order := goclip.RegisteredDrivers(); len(order) == 0 || order[0] != "darwin" {
		t.Fatalf("goclip asks %q first, want darwin", order)
	}
	text, err := d.ReadText()
	if err != nil || text != "Лунобот" {
		t.Fatalf("ReadText = %q, %v; want Лунобот", text, err)
	}
	env, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(env)); got != "LANG="+pbcopyLocale {
		t.Errorf("the locale of pbpaste = %q, want only LANG=%s", got, pbcopyLocale)
	}
	if err := d.WriteText("Лунобот"); err != nil {
		t.Fatal(err)
	}
	if input, err := os.ReadFile(filepath.Join(dir, "input")); err != nil || string(input) != "Лунобот" {
		t.Errorf("pbcopy got %q, %v; want Лунобот", input, err)
	}
}
