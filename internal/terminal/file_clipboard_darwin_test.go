//go:build darwin

package terminal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// The real pasteboard through the real reader. Finder offers copied files as
// file URLs and f4 used to read only the plain text beside them, so Ctrl+V of a
// Finder copy put the file name on the command line (f4#2007). The test
// overwrites the user's clipboard, so it runs only on CI runners.
func TestReadDarwinFileClipboardPasteboard(t *testing.T) {
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Skip("overwrites the system clipboard; runs on CI runners only")
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skipf("osascript is not available: %v", err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	one, two := filepath.Join(dir, "one.txt"), filepath.Join(dir, "two words.txt")
	for _, p := range []string{one, two} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	set := func(t *testing.T, script string) {
		t.Helper()
		if out, err := exec.Command("osascript", "-e", script).CombinedOutput(); err != nil { // #nosec G204 -- fixed test script.
			t.Fatalf("%s: %v: %s", script, err, out)
		}
	}

	t.Run("files", func(t *testing.T) {
		// Finder puts every copied file on the pasteboard as an item of its
		// own; AppleScript's "set the clipboard to {file, file}" does not.
		names, err := json.Marshal([]string{one, two})
		if err != nil {
			t.Fatal(err)
		}
		script := `ObjC.import('AppKit');
var pb = $.NSPasteboard.generalPasteboard;
pb.clearContents;
var items = [];
` + "JSON.parse('" + string(names) + "')" + `.forEach(function (p) { items.push($.NSURL.fileURLWithPath(p)); });
var wrote = pb.writeObjects($(items));
// Read the items back before exiting: a writer that quits at once can leave
// the second item unresolved for the next process.
var back = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), $.NSDictionary.dictionaryWithObjectForKey($(true), $.NSPasteboardURLReadingFileURLsOnlyKey));
'items=' + items.length + ' wrote=' + wrote + ' pasteboard items=' + pb.pasteboardItems.count + ' back=' + back.count;`
		out, err := exec.Command("osascript", "-l", "JavaScript", "-e", script).CombinedOutput() // #nosec G204 -- fixed test script; the paths are JSON-encoded.
		if err != nil {
			t.Fatalf("writing two files to the pasteboard: %v: %s", err, out)
		}
		t.Logf("pasteboard: %s", out)
		got, cut, err := readFileClipboard(context.Background())
		if err != nil {
			t.Fatalf("reading copied files failed: %v", err)
		}
		sort.Strings(got)
		if want := []string{one, two}; !reflect.DeepEqual(got, want) || cut {
			t.Fatalf("files = %q cut = %v, want %q", got, cut, want)
		}
	})
	t.Run("one file", func(t *testing.T) {
		set(t, `set the clipboard to (POSIX file "`+one+`")`)
		got, _, err := readFileClipboard(context.Background())
		if err != nil || !reflect.DeepEqual(got, []string{one}) {
			t.Fatalf("files = %q, err = %v, want %q", got, err, []string{one})
		}
	})
	t.Run("text is not a file", func(t *testing.T) {
		set(t, `set the clipboard to "`+one+`"`)
		if got, _, err := readFileClipboard(context.Background()); err == nil {
			t.Fatalf("plain text with a path read as files %q", got)
		}
	})
	t.Run("empty", func(t *testing.T) {
		set(t, `set the clipboard to ""`)
		if got, _, err := readFileClipboard(context.Background()); err == nil {
			t.Fatalf("an empty clipboard read as files %q", got)
		}
	})
}
