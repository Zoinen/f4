package ap

// ModificationResult.Preview - one applied modification's own edit, for the
// patch review screen's per-row diff (f4#1606, docs/VTVIBE.md §7.3). Not
// ported from anywhere: the Python reference has no per-modification
// report at all.

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestModPreview(t *testing.T) {
	tests := []struct {
		name          string
		before, after string
		want          *Preview
	}{
		{"identical", "a\nb\n", "a\nb\n", nil},
		{"only the final newline differs", "a\nb", "a\nb\n", nil},
		{"both empty", "", "", nil},
		{
			"middle line, context trimmed to three",
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n", "1\n2\n3\n4\nFIVE\n6\n7\n8\n9\n",
			&Preview{StartLine: 2, Before: []string{"2", "3", "4", "5", "6", "7", "8"}, After: []string{"2", "3", "4", "FIVE", "6", "7", "8"}},
		},
		{
			"first line, no context above",
			"1\n2\n", "one\n2\n",
			&Preview{StartLine: 1, Before: []string{"1", "2"}, After: []string{"one", "2"}},
		},
		{
			"insertion at the end",
			"1\n2\n", "1\n2\n3\n",
			&Preview{StartLine: 1, Before: []string{"1", "2"}, After: []string{"1", "2", "3"}},
		},
		{
			"deletion",
			"1\n2\n3\n", "1\n3\n",
			&Preview{StartLine: 1, Before: []string{"1", "2", "3"}, After: []string{"1", "3"}},
		},
		{
			// The shared tail must not eat into the shared head: "a a" ->
			// "a" has one common line, not two.
			"repeated lines",
			"a\na\n", "a\n",
			&Preview{StartLine: 1, Before: []string{"a", "a"}, After: []string{"a"}},
		},
		{
			"new file",
			"", "x\ny\n",
			&Preview{StartLine: 1, Before: nil, After: []string{"x", "y"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modPreview(tt.before, tt.after); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("modPreview(%q, %q) = %+v, want %+v", tt.before, tt.after, got, tt.want)
			}
		})
	}
}

// TestModificationResultPreview runs a real (dry) Apply and checks what the
// review screen will get: one Preview per applied edit, each against the
// file as the earlier edits left it, nothing for rows with no edit, no CR
// from a CRLF file, and nothing written to disk.
func TestModificationResultPreview(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("setup %s: %v", name, err)
		}
	}
	const aTxt = "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\n"
	const crlf = "one\r\ntwo\r\nthree\r\n"
	write("a.txt", aTxt)
	write("crlf.txt", crlf)
	write("same.txt", "same\n")
	write("old.txt", "x\n")
	patch := "cc000001 AP 3.2\n\n" +
		"cc000001 FILE\na.txt\n\n" +
		"cc000001 REPLACE\ncc000001 snippet\nl2\ncc000001 content\nL2\n\n" +
		"cc000001 DELETE\ncc000001 snippet\nl4\n\n" +
		"cc000001 INSERT_AFTER\ncc000001 snippet\nl9\ncc000001 content\nl9.5\n\n" +
		"cc000001 REPLACE\ncc000001 snippet\nno such line\ncc000001 content\nX\n\n" +
		"cc000001 FILE\ncrlf.txt\n\n" +
		"cc000001 REPLACE\ncc000001 snippet\ntwo\ncc000001 content\nTWO\n\n" +
		"cc000001 FILE\nsame.txt\n\n" +
		"cc000001 REPLACE\ncc000001 snippet\nsame\ncc000001 content\nsame\n\n" +
		"cc000001 FILE\nnew.txt\n\n" +
		"cc000001 CREATE\ncc000001 content\nhello\n\n" +
		"cc000001 FILE\nold.txt\n\ncc000001 RENAME\nrenamed.txt\n"
	patchPath := filepath.Join(t.TempDir(), "p.ap")
	if err := os.WriteFile(patchPath, []byte(patch), 0o600); err != nil {
		t.Fatalf("setup patch: %v", err)
	}

	res := Apply(patchPath, dir, Options{DryRun: true, Silent: true})
	mods := res.ModificationResults

	checkPreview := func(file string, idx int, want *Preview) {
		t.Helper()
		got := findModResult(t, mods, file, idx)
		if !reflect.DeepEqual(got.Preview, want) {
			t.Fatalf("%s#%d (%s) Preview = %+v, want %+v", file, idx, got.Status, got.Preview, want)
		}
	}
	checkPreview("a.txt", 0, &Preview{StartLine: 1,
		Before: []string{"l1", "l2", "l3", "l4", "l5"},
		After:  []string{"l1", "L2", "l3", "l4", "l5"}})
	// Both sides of the DELETE's fragment already carry the first
	// REPLACE: each Preview is one edit, against the file as the edits
	// before it left it.
	checkPreview("a.txt", 1, &Preview{StartLine: 1,
		Before: []string{"l1", "L2", "l3", "l4", "l5", "l6", "l7"},
		After:  []string{"l1", "L2", "l3", "l5", "l6", "l7"}})
	// And line numbers count that file too: with l4 gone, l7 is line 6.
	checkPreview("a.txt", 2, &Preview{StartLine: 6,
		Before: []string{"l7", "l8", "l9", "l10"},
		After:  []string{"l7", "l8", "l9", "l9.5", "l10"}})
	checkPreview("a.txt", 3, nil) // failed
	checkPreview("crlf.txt", 0, &Preview{StartLine: 1,
		Before: []string{"one", "two", "three"},
		After:  []string{"one", "TWO", "three"}})
	checkPreview("same.txt", 0, nil) // already applied
	checkPreview("new.txt", 0, &Preview{StartLine: 1, After: []string{"hello"}})
	checkPreview("old.txt", -1, nil) // RENAME has no text of its own

	if st := findModResult(t, mods, "a.txt", 3).Status; st != ModFailed {
		t.Fatalf("a.txt#3 status = %s, want FAILED", st)
	}
	if st := findModResult(t, mods, "same.txt", 0).Status; st != ModSkipped {
		t.Fatalf("same.txt#0 status = %s, want SKIPPED", st)
	}

	// A dry run with previews still writes nothing.
	for name, want := range map[string]string{"a.txt": aTxt, "crlf.txt": crlf, "old.txt": "x\n"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q, %v; want untouched %q", name, b, err, want)
		}
	}
	for _, name := range []string{"new.txt", "renamed.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists after a dry run (err %v)", name, err)
		}
	}
}
