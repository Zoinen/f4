package fishplus

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestGrepOptionsMode(t *testing.T) {
	for _, tc := range []struct {
		opts GrepOptions
		want string
	}{
		{GrepOptions{}, "e"},
		{GrepOptions{Fixed: true}, "f"},
		{GrepOptions{IgnoreCase: true}, "ei"},
		{GrepOptions{Fixed: true, IgnoreCase: true}, "fi"},
	} {
		if got := tc.opts.mode(); got != tc.want {
			t.Errorf("mode(%+v) = %q, want %q", tc.opts, got, tc.want)
		}
	}
}

// TestGrepAgainstLocalShell checks the offsets against the ones the same
// content has in memory, because an offset that is off by a line is exactly
// the kind of mistake a parser written against captured output survives.
func TestGrepAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanGrep() {
		t.Skip("no grep and awk on this host")
	}

	var body strings.Builder
	for i := 0; i < 500; i++ {
		body.WriteString("filler line to push the offsets past a single block\n")
	}
	needleAt := int64(body.Len()) + 6
	body.WriteString("here: NEEDLE and nothing else\n")
	for i := 0; i < 500; i++ {
		body.WriteString("more filler, this time after the interesting line\n")
	}
	lowerAt := int64(body.Len()) + 6
	body.WriteString("here: needle in lower case\n")
	content := body.String()

	file := filepath.Join(t.TempDir(), "a big log.txt")
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	got, err := c.Grep(ctx, file, "NEEDLE", GrepOptions{Fixed: true})
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if len(got) != 1 || got[0] != needleAt {
		t.Fatalf("offsets = %v, want [%d]", got, needleAt)
	}
	if content[got[0]:got[0]+6] != "NEEDLE" {
		t.Errorf("the offset does not point at the match: %q", content[got[0]:got[0]+6])
	}

	got, err = c.Grep(ctx, file, "needle", GrepOptions{Fixed: true, IgnoreCase: true})
	if err != nil {
		t.Fatalf("grep ignoring case: %v", err)
	}
	if len(got) != 2 || got[0] != needleAt || got[1] != lowerAt {
		t.Fatalf("offsets = %v, want [%d %d]", got, needleAt, lowerAt)
	}

	// A regular expression, and the limit that keeps a match-everything
	// pattern from flooding the session.
	got, err = c.Grep(ctx, file, "^more filler", GrepOptions{})
	if err != nil {
		t.Fatalf("grep with a regexp: %v", err)
	}
	if len(got) != 500 {
		t.Errorf("regexp matches = %d, want 500", len(got))
	}
	got, err = c.Grep(ctx, file, "^more filler", GrepOptions{Limit: 7})
	if err != nil {
		t.Fatalf("grep with a limit: %v", err)
	}
	if len(got) != 7 {
		t.Errorf("limited matches = %d, want 7", len(got))
	}

	// A pattern with spaces, which only survives because it travels on a
	// line of its own rather than as a request argument.
	got, err = c.Grep(ctx, file, "push the offsets", GrepOptions{Fixed: true})
	if err != nil {
		t.Fatalf("grep with spaces: %v", err)
	}
	if len(got) != 500 {
		t.Errorf("matches for a phrase = %d, want 500", len(got))
	}

	if got, err = c.Grep(ctx, file, "this string is not there", GrepOptions{Fixed: true}); err != nil {
		t.Errorf("grep without matches: %v", err)
	} else if len(got) != 0 {
		t.Errorf("offsets = %v, want none", got)
	}

	if _, err := c.Grep(ctx, file, "", GrepOptions{}); err == nil {
		t.Error("an empty pattern was accepted")
	}
	if _, err := c.Grep(ctx, filepath.Dir(file), "x", GrepOptions{}); err == nil {
		t.Error("a directory was searched")
	}
	if _, err := c.Grep(ctx, file+".missing", "x", GrepOptions{}); err == nil {
		t.Error("a missing file was searched")
	}
	if got, err := c.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("session out of sync after grep: %q %v", got, err)
	}
}

// TestLinesAgainstLocalShell checks the offsets against the ones the same
// content has in memory, including the last line and the end of the file,
// which is where an off-by-one in the awk arithmetic would show up.
func TestLinesAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanIndexLines() {
		t.Skip("no awk on this host")
	}

	var body strings.Builder
	var want []int64
	for i := 0; i < 300; i++ {
		want = append(want, int64(body.Len()))
		body.WriteString(strings.Repeat("x", i%37) + " line\n")
	}
	content := body.String()
	dir := t.TempDir()
	file := filepath.Join(dir, "a log.txt")
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	idx, err := c.Lines(ctx, file, 1, 5)
	if err != nil {
		t.Fatalf("lines: %v", err)
	}
	if idx.Total != 300 {
		t.Errorf("total = %d, want 300", idx.Total)
	}
	if len(idx.Offsets) != 5 {
		t.Fatalf("offsets = %v, want 5 of them", idx.Offsets)
	}
	for i, off := range idx.Offsets {
		if off != want[i] {
			t.Fatalf("offset of line %d = %d, want %d", i+1, off, want[i])
		}
	}

	// A window in the middle, which is what scrolling asks for.
	idx, err = c.Lines(ctx, file, 100, 3)
	if err != nil {
		t.Fatalf("lines in the middle: %v", err)
	}
	if len(idx.Offsets) != 3 || idx.Offsets[0] != want[99] || idx.Offsets[2] != want[101] {
		t.Fatalf("offsets = %v, want %v", idx.Offsets, want[99:102])
	}
	if content[idx.Offsets[0]:idx.Offsets[0]+1] != "x" && want[99] != idx.Offsets[0] {
		t.Errorf("the offset does not point at a line start")
	}

	// The tail of the file: fewer lines than asked for, and the last one
	// still has to point at the right byte.
	idx, err = c.Lines(ctx, file, 298, 10)
	if err != nil {
		t.Fatalf("lines at the end: %v", err)
	}
	if len(idx.Offsets) != 3 || idx.Offsets[2] != want[299] {
		t.Fatalf("offsets at the end = %v, want the last three of %v", idx.Offsets, want[297:])
	}

	// Past the end is an empty answer, not an error: the viewer asks for a
	// screenful without knowing where the file stops.
	idx, err = c.Lines(ctx, file, 5000, 10)
	if err != nil {
		t.Fatalf("lines past the end: %v", err)
	}
	if len(idx.Offsets) != 0 || idx.Total != 300 {
		t.Errorf("past the end = %v (total %d)", idx.Offsets, idx.Total)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if idx, err = c.Lines(ctx, empty, 1, 10); err != nil {
		t.Fatalf("lines of an empty file: %v", err)
	}
	if idx.Total != 0 || len(idx.Offsets) != 0 {
		t.Errorf("empty file = %v (total %d)", idx.Offsets, idx.Total)
	}

	if _, err := c.Lines(ctx, file, 0, 10); err == nil {
		t.Error("line 0 was accepted")
	}
	if _, err := c.Lines(ctx, file, 1, MaxLineIndexCount+1); err == nil {
		t.Error("an oversized count was accepted")
	}
	if _, err := c.Lines(ctx, dir, 1, 10); err == nil {
		t.Error("a directory was indexed")
	}
	if got, err := c.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("session out of sync after lidx: %q %v", got, err)
	}
}

func findNames(t *testing.T, entries []Entry, root string) []string {
	t.Helper()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		rel, err := filepath.Rel(root, e.Name)
		if err != nil {
			t.Fatalf("hit %q is not below %q", e.Name, root)
		}
		names = append(names, rel)
	}
	sort.Strings(names)
	return names
}

func TestFindAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "sub dir", "deeper"), 0700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"a.txt":                "hello world\n",
		"b.txt":                "nothing here\n",
		"c.log":                "HELLO again\n",
		"sub dir/deeper/d.txt": "hello nested\n",
		"sub dir/skip me.bin":  "hello binary\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	for _, mode := range ListingModes {
		if err := c.SetListingMode(ctx, mode); err != nil {
			t.Logf("listing backend %q unavailable here: %v", mode, err)
			continue
		}
		t.Run(mode, func(t *testing.T) {
			hits, err := c.Find(ctx, root, FindOptions{Masks: []string{"*.txt"}})
			if err != nil {
				t.Fatalf("find by mask: %v", err)
			}
			got := findNames(t, hits, root)
			want := []string{"a.txt", "b.txt", filepath.Join("sub dir", "deeper", "d.txt")}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("mask search found %v, want %v", got, want)
			}
			for _, e := range hits {
				if e.Size <= 0 || e.MTime.IsZero() {
					t.Errorf("hit %q came back without metadata: %+v", e.Name, e)
				}
			}

			hits, err = c.Find(ctx, root, FindOptions{Masks: []string{"*.log", "*.bin"}})
			if err != nil {
				t.Fatalf("find by two masks: %v", err)
			}
			got = findNames(t, hits, root)
			want = []string{"c.log", filepath.Join("sub dir", "skip me.bin")}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("two mask search found %v, want %v", got, want)
			}

			if !c.CanGrep() {
				t.Skip("no remote grep on this host")
			}

			hits, err = c.Find(ctx, root, FindOptions{
				Masks: []string{"*"}, Text: "hello", Fixed: true, IgnoreCase: true,
			})
			if err != nil {
				t.Fatalf("content search: %v", err)
			}
			got = findNames(t, hits, root)
			want = []string{
				"a.txt", "c.log",
				filepath.Join("sub dir", "deeper", "d.txt"),
				filepath.Join("sub dir", "skip me.bin"),
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("content search found %v, want %v", got, want)
			}

			hits, err = c.Find(ctx, root, FindOptions{
				Masks: []string{"*"}, Text: "HELLO", Fixed: true,
			})
			if err != nil {
				t.Fatalf("case sensitive search: %v", err)
			}
			if got = findNames(t, hits, root); !reflect.DeepEqual(got, []string{"c.log"}) {
				t.Errorf("case sensitive search found %v, want [c.log]", got)
			}

			hits, err = c.Find(ctx, root, FindOptions{Masks: []string{"*"}, Limit: 2})
			if err != nil {
				t.Fatalf("limited search: %v", err)
			}
			if len(hits) != 2 {
				t.Errorf("a limit of 2 returned %d hits", len(hits))
			}
		})
	}

	if _, err := c.Find(ctx, root, FindOptions{}); err == nil {
		t.Error("a search without a mask was accepted")
	}
	if _, err := c.Find(ctx, filepath.Join(root, "a.txt"), FindOptions{Masks: []string{"*"}}); err == nil {
		t.Error("a regular file was accepted as a search root")
	}
	// A desynchronized stream is what a variable number of path lines is
	// most likely to cause, so make sure the session still answers.
	if err := c.Session().Noop(ctx); err != nil {
		t.Fatalf("session out of sync after the searches: %v", err)
	}
}
