package multiarc

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSevenZipRealWrite(t *testing.T) {
	bin, ok := sevenZipTool()
	if !ok {
		t.Skip("no 7z, 7za or 7zr on PATH")
	}
	ctx := context.Background()
	src := t.TempDir()
	writeTree(t, src, map[string]string{
		"top.txt":       "top",
		"-dash.txt":     "dash",
		"dir/a.txt":     "a",
		"dir/sub/b.txt": "b",
	})
	arc := filepath.Join(t.TempDir(), "x.7z")
	runReal(t, src, bin, "a", arc, "top.txt", "dir", "--", "-dash.txt")
	t.Cleanup(closeSharedMultiArcTempDirs)
	v := openReal(t, arc)

	writeMember(t, v, "/dir/sub/new.txt", "new")
	writeMember(t, v, "/top.txt", "replaced")
	if err := v.MkDir(ctx, "/empty"); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	if err := v.Remove(ctx, "/-dash.txt"); err != nil {
		t.Fatalf("Remove -dash.txt: %v", err)
	}
	if err := v.Remove(ctx, "/dir/sub"); err != nil {
		t.Fatalf("Remove dir/sub: %v", err)
	}
	assertMembers(t, arc, "top.txt", "dir/", "dir/a.txt", "empty/")
	if got := readMember(t, v, "/top.txt"); got != "replaced" {
		t.Errorf("top.txt = %q, want replaced", got)
	}
	writeMember(t, v, "/@at.txt", "at")
	if got := readMember(t, v, "/@at.txt"); got != "at" {
		t.Errorf("@at.txt = %q: 7z must take a name starting with @ as a name, not a list file", got)
	}
	assertNoScratchLeft(t, arc)
}
