package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// deletedFileRepo commits d.txt with the lines 1..6 and removes it from the
// working tree (git rm when staged is true), so the status panel has one
// entry: the deleted file.
func deletedFileRepo(t *testing.T, staged bool) string {
	t.Helper()
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "d.txt"), numbered(1, 6))
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	if staged {
		runRealGit(t, repo, "rm", "-q", "d.txt")
	} else if err := os.Remove(filepath.Join(repo, "d.txt")); err != nil {
		t.Fatal(err)
	}
	return repo
}

// pickRows toggles the given rows of the hunk view (row 0 is the @@ line,
// the deleted file's lines "-1".."-6" are rows 1..6) one by one.
func pickRows(t *testing.T, v *HunkView, rows ...int) {
	t.Helper()
	for _, r := range rows {
		moveTo(t, v, r)
		v.ProcessKey(key(vtinput.VK_INSERT))
	}
}

// TestBuildPatchDeletedFileInParts: the patch of a deleted file may be
// picked in part. Staging writes it as a modification that removes just the
// picked line (the others stay as context); unstaging keeps the deletion
// header and lists only the picked line, which the reverse apply re-creates.
func TestBuildPatchDeletedFileInParts(t *testing.T) {
	header := []string{"diff --git a/d b/d", "deleted file mode 100644", "index 1..0000000", "--- a/d", "+++ /dev/null"}
	mk := func(mode hunkMode) *filePatch {
		fp := &filePatch{
			mode:   mode,
			header: append([]string(nil), header...),
			hunks:  []*diffHunk{testHunk(1, 4, 0, 0, "-1", "-2", "-3", "-4")},
		}
		pickLines(fp.hunks[0], 1)
		return fp
	}
	want := "diff --git a/d b/d\nindex 1..0000000\n--- a/d\n+++ b/d\n@@ -1,4 +1,3 @@\n 1\n-2\n 3\n 4\n"
	if got := mustBuildPatch(t, mk(modeStage)); got != want {
		t.Errorf("staging part of a deleted file:\n%s\nwant\n%s", got, want)
	}
	want = "diff --git a/d b/d\ndeleted file mode 100644\nindex 1..0000000\n--- a/d\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-2\n"
	for _, mode := range []hunkMode{modeUnstage, modeDiscard} {
		if got := mustBuildPatch(t, mk(mode)); got != want {
			t.Errorf("mode %v, part of a deleted file:\n%s\nwant\n%s", mode, got, want)
		}
	}

	// Picked whole, the deletion stays a deletion whatever the mode.
	fp := mk(modeStage)
	fp.hunks[0].pickAll(true)
	if got := mustBuildPatch(t, fp); got != "diff --git a/d b/d\ndeleted file mode 100644\nindex 1..0000000\n--- a/d\n+++ /dev/null\n@@ -1,4 +0,0 @@\n-1\n-2\n-3\n-4\n" {
		t.Errorf("a deleted file picked whole:\n%s", got)
	}

	// A quoted path (a name git has to escape) keeps its quotes.
	fp = mk(modeStage)
	fp.header = []string{"diff --git \"a/d\\t\" \"b/d\\t\"", "deleted file mode 100644", "index 1..0", "--- \"a/d\\t\"", "+++ /dev/null"}
	got := mustBuildPatch(t, fp)
	if want := "--- \"a/d\\t\"\n+++ \"b/d\\t\"\n"; !strings.Contains(got, want) {
		t.Errorf("quoted path: patch\n%s\nwant it to hold %q", got, want)
	}
}

// TestHunkViewStagesPartOfADeletedFile: F4 on a file deleted from the
// working tree, "2" and "3" picked: the index keeps the file without those
// two lines, and the working tree stays without the file.
func TestHunkViewStagesPartOfADeletedFile(t *testing.T) {
	repo := deletedFileRepo(t, false)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeStage)
	pickRows(t, v, 2, 3)
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after staging")
	}
	if got, want := runRealGit(t, repo, "show", ":d.txt"), "1\n4\n5\n6\n"; got != want {
		t.Errorf("index d.txt = %q, want %q", got, want)
	}
	if got := runRealGit(t, repo, "status", "--porcelain"); got != "MD d.txt\n" {
		t.Errorf("status = %q, want the file changed in the index and still deleted in the working tree", got)
	}
}

// TestHunkViewUnstagesPartOfADeletedFile: the deletion is staged, Shift+F4
// with "2" and "3" picked brings just those two lines back into the index.
func TestHunkViewUnstagesPartOfADeletedFile(t *testing.T) {
	repo := deletedFileRepo(t, true)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)
	pickRows(t, v, 2, 3)
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}
	if got, want := runRealGit(t, repo, "show", ":d.txt"), "2\n3\n"; got != want {
		t.Errorf("index d.txt = %q, want %q", got, want)
	}
}

// TestHunkViewDiscardsPartOfADeletedFile: F8 on the deleted file with "2"
// and "3" picked puts just those lines back into the working tree; the
// index is not touched.
func TestHunkViewDiscardsPartOfADeletedFile(t *testing.T) {
	repo := deletedFileRepo(t, false)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	pickRows(t, v, 2, 3)
	discardConfirm(t, v).OnResult(0)
	if !v.IsDone() {
		t.Fatal("the view did not close after discarding")
	}
	if got, want := readRepoFile(t, filepath.Join(repo, "d.txt")), "2\n3\n"; got != want {
		t.Errorf("working file = %q, want %q", got, want)
	}
	if got := runRealGit(t, repo, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("discarding changed the index: %q", got)
	}
}
