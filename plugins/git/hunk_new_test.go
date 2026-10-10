package git

import (
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
)

// TestBuildPatchUnstagesPartOfANewFile: unstaging or discarding one line of
// a new file writes the reverse patch as a modification of it; staging part
// of the same patch (an intent-to-add file) keeps the creation, with just
// the picked line.
func TestBuildPatchUnstagesPartOfANewFile(t *testing.T) {
	mk := func(mode hunkMode) *filePatch {
		fp := &filePatch{
			mode:   mode,
			header: []string{"diff --git a/n b/n", "new file mode 100644", "index 0000000..1", "--- /dev/null", "+++ b/n"},
			hunks:  []*diffHunk{testHunk(0, 0, 1, 3, "+1", "+2", "+3")},
		}
		pickLines(fp.hunks[0], 1)
		return fp
	}
	want := "diff --git a/n b/n\nindex 0000000..1\n--- a/n\n+++ b/n\n@@ -1,2 +1,3 @@\n 1\n+2\n 3\n"
	if got := mustBuildPatch(t, mk(modeUnstage)); got != want {
		t.Errorf("unstaging part of a new file:\n%s\nwant\n%s", got, want)
	}
	if got := mustBuildPatch(t, mk(modeDiscard)); got != want {
		t.Errorf("discarding part of a new file:\n%s\nwant\n%s", got, want)
	}
	wantStage := "diff --git a/n b/n\nnew file mode 100644\nindex 0000000..1\n--- /dev/null\n+++ b/n\n@@ -0,0 +1,1 @@\n+2\n"
	if got := mustBuildPatch(t, mk(modeStage)); got != wantStage {
		t.Errorf("staging part of a new file:\n%s\nwant\n%s", got, wantStage)
	}
}

// TestHunkViewUnstagesPartOfANewFile: Shift+F4 on a staged new file with
// "2" and "3" picked leaves the index with the other lines; the working
// file is untouched.
func TestHunkViewUnstagesPartOfANewFile(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "n.txt"), numbered(1, 5))
	runRealGit(t, repo, "add", "n.txt")

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)
	pickRows(t, v, 2, 3)
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}
	if got, want := runRealGit(t, repo, "show", ":n.txt"), "1\n4\n5\n"; got != want {
		t.Errorf("index n.txt = %q, want %q", got, want)
	}
	if got, want := readRepoFile(t, filepath.Join(repo, "n.txt")), numbered(1, 5); got != want {
		t.Errorf("working file = %q, want it untouched", got)
	}
}

func intentToAddRepo(t *testing.T) string {
	t.Helper()
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "n.txt"), numbered(1, 5))
	runRealGit(t, repo, "add", "-N", "n.txt")
	return repo
}

// TestHunkViewStagesPartOfAnIntentToAddFile: F4 on a file added with
// `git add -N` and lines "2" and "3" picked puts just those lines into the
// index; the working file is untouched.
func TestHunkViewStagesPartOfAnIntentToAddFile(t *testing.T) {
	repo := intentToAddRepo(t)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeStage)
	pickRows(t, v, 2, 3)
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after staging")
	}
	if got, want := runRealGit(t, repo, "show", ":n.txt"), "2\n3\n"; got != want {
		t.Errorf("index n.txt = %q, want %q", got, want)
	}
	if got, want := readRepoFile(t, filepath.Join(repo, "n.txt")), numbered(1, 5); got != want {
		t.Errorf("working file = %q, want it untouched", got)
	}
}

// TestHunkViewDiscardsPartOfAnIntentToAddFile: F8 with "2" and "3" picked
// removes just those lines from the working file; the file stays known to
// the index.
func TestHunkViewDiscardsPartOfAnIntentToAddFile(t *testing.T) {
	repo := intentToAddRepo(t)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	pickRows(t, v, 2, 3)
	discardConfirm(t, v).OnResult(0)
	if !v.IsDone() {
		t.Fatal("the view did not close after discarding")
	}
	if got, want := readRepoFile(t, filepath.Join(repo, "n.txt")), "1\n4\n5\n"; got != want {
		t.Errorf("working file = %q, want %q", got, want)
	}
}
