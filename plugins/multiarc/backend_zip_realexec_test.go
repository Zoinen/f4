package multiarc

import (
	"context"
	"path/filepath"
	"testing"
)

// zipRealWriteRoundTrip drives every write a panel can make on an opened
// zip through whichever tool zipWriteTool picks from PATH, and checks the
// archive itself afterwards.
func zipRealWriteRoundTrip(t *testing.T, arc string) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(closeSharedMultiArcTempDirs)
	v := openReal(t, arc)

	writeMember(t, v, "/dir/new.txt", "new")
	writeMember(t, v, "/top.txt", "replaced")
	if err := v.MkDir(ctx, "/empty"); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	// "[ab].txt" is a literal name: with wildcards on, zip would also
	// delete "a.txt".
	if err := v.Remove(ctx, "/dir/[ab].txt"); err != nil {
		t.Fatalf("Remove [ab].txt: %v", err)
	}
	if err := v.Remove(ctx, "/dir/sub"); err != nil {
		t.Fatalf("Remove dir/sub: %v", err)
	}
	assertMembers(t, arc, "top.txt", "dir/", "dir/a.txt", "dir/new.txt", "empty/")
	if got := readMember(t, v, "/top.txt"); got != "replaced" {
		t.Errorf("top.txt = %q, want replaced", got)
	}
	if got := readMember(t, v, "/dir/new.txt"); got != "new" {
		t.Errorf("dir/new.txt = %q, want new", got)
	}

	// Deleting the last members leaves an empty zip that still lists.
	for _, p := range []string{"/top.txt", "/dir", "/empty"} {
		if err := v.Remove(ctx, p); err != nil {
			t.Fatalf("Remove %s: %v", p, err)
		}
	}
	assertMembers(t, arc)
	assertNoScratchLeft(t, arc)
}

func zipRealFixture(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"top.txt":         "top",
		"dir/a.txt":       "a",
		"dir/[ab].txt":    "brackets",
		"dir/sub/b.txt":   "b",
		"dir/sub/deeper/": "",
	})
	return src
}

// Viewing "dir/[ab].txt" used to extract dir/a.txt instead (unzip read the
// name as a pattern), and viewing "-dash.txt" failed outright.
func TestZipRealOpensLiteralNames(t *testing.T) {
	requireRealTool(t, "zip", "unzip")
	src := t.TempDir()
	writeTree(t, src, map[string]string{"dir/[ab].txt": "brackets", "dir/a.txt": "a", "-dash.txt": "dash"})
	arc := filepath.Join(t.TempDir(), "x.zip")
	runReal(t, src, "zip", "-q", "-r", "-nw", arc, "--", "dir", "-dash.txt")
	t.Cleanup(closeSharedMultiArcTempDirs)
	v := openReal(t, arc)
	if got := readMember(t, v, "/dir/[ab].txt"); got != "brackets" {
		t.Errorf("dir/[ab].txt = %q, want brackets", got)
	}
	if got := readMember(t, v, "/-dash.txt"); got != "dash" {
		t.Errorf("-dash.txt = %q, want dash", got)
	}
}

func TestZipRealWriteWithInfoZip(t *testing.T) {
	requireRealTool(t, "zip", "unzip")
	src := zipRealFixture(t)
	arc := filepath.Join(t.TempDir(), "x.zip")
	runReal(t, src, "zip", "-q", "-r", arc, "top.txt", "dir")
	zipRealWriteRoundTrip(t, arc)
}

// With no zip on PATH, a zip is changed through 7z (or 7za), which handles
// the format too.
func TestZipRealWriteWithSevenZipFallback(t *testing.T) {
	requireRealTool(t, "unzip")
	sevenZip := ""
	for _, name := range []string{"7z", "7za"} {
		if toolAvailable(name) {
			sevenZip = name
			break
		}
	}
	if sevenZip == "" {
		t.Skip("neither 7z nor 7za is on PATH")
	}
	src := zipRealFixture(t)
	arc := filepath.Join(t.TempDir(), "x.zip")
	runReal(t, src, sevenZip, "a", "-tzip", arc, "top.txt", "dir")
	pathOnly(t, map[string]string{"unzip": "unzip", sevenZip: sevenZip})
	if bin, sz, ok := zipWriteTool(); !ok || !sz || bin != sevenZip {
		t.Fatalf("zipWriteTool = (%q, %v, %v), want %s", bin, sz, ok, sevenZip)
	}
	zipRealWriteRoundTrip(t, arc)
}
