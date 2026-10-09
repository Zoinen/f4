package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// readRepoFile returns the working file's bytes as a string.
func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// discardConfirm presses Enter in a discarding HunkView and returns the
// confirmation dialog it must have pushed.
func discardConfirm(t *testing.T, v *HunkView) *vtui.Window {
	t.Helper()
	before := vtui.FrameManager.GetTopFrame()
	v.ProcessKey(key(vtinput.VK_RETURN))
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm == before || confirm.OnResult == nil {
		t.Fatalf("top frame after Enter is %T, want the *vtui.Window discard confirmation", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(func() { vtui.FrameManager.RemoveFrame(confirm) })
	if v.IsDone() {
		t.Fatal("the view closed before the discard was confirmed")
	}
	return confirm
}

// TestBuildPatchDiscardKeepsTheWorkingFileSide: discarding "+B" and "-25"
// of twoHunkRepo's diff. The patch is applied with -R to the working file,
// so its "+" side keeps every line of the file as it is ("+A" and "+X"
// turn into context) and the "-" side takes the picked lines only; the
// second hunk's "-" start moves back by the one line "+B" adds.
func TestBuildPatchDiscardKeepsTheWorkingFileSide(t *testing.T) {
	fp := &filePatch{
		mode:   modeDiscard,
		header: []string{"diff --git a/f b/f", "index 1..2 100644", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{
			testHunk(1, 5, 1, 7, " 1", " 2", "+A", "+B", " 3", " 4", " 5"),
			testHunk(22, 7, 24, 7, " 22", " 23", " 24", "-25", "+X", " 26", " 27", " 28"),
		},
	}
	pickLines(fp.hunks[0], 3)
	pickLines(fp.hunks[1], 3)
	want := "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n" +
		"@@ -1,6 +1,7 @@\n 1\n 2\n A\n+B\n 3\n 4\n 5\n" +
		"@@ -23,8 +24,7 @@\n 22\n 23\n 24\n-25\n X\n 26\n 27\n 28\n"
	if got := mustBuildPatch(t, fp); got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}
	if n := fp.pickedLineCount(); n != 2 {
		t.Errorf("pickedLineCount = %d, want 2", n)
	}
}

// TestHunkViewDiscardsSingleLines: F8's view over twoHunkRepo, "+B" and
// "-25" picked, Enter, the confirmation answered with "Discard". The
// working file loses "B" and gets "25" back -- the second hunk has to be
// found one line higher than git printed it -- while "A" and "X" stay;
// the index is not touched.
func TestHunkViewDiscardsSingleLines(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	path := filepath.Join(repo, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	if v.patch.mode != modeDiscard || len(v.patch.hunks) != 2 {
		t.Fatalf("mode = %v, hunks = %d; want modeDiscard, 2", v.patch.mode, len(v.patch.hunks))
	}

	moveTo(t, v, rowB)
	v.ProcessKey(key(vtinput.VK_INSERT))
	moveTo(t, v, rowMinus)
	v.ProcessKey(key(vtinput.VK_INSERT))
	confirm := discardConfirm(t, v)
	if got := readRepoFile(t, path); got != numbered(1, 2)+"A\nB\n"+numbered(3, 24)+"X\n"+numbered(26, 30) {
		t.Fatalf("the working file changed before the discard was confirmed:\n%s", got)
	}
	confirm.OnResult(0) // "Discard"
	if !v.IsDone() {
		t.Fatal("the view did not close after discarding")
	}

	if got := readRepoFile(t, path); got != halfPickedContent() {
		t.Errorf("working file after discarding lines:\n%s\nwant\n%s", got, halfPickedContent())
	}
	if got := runRealGit(t, repo, "show", ":f.txt"); got != numbered(1, 30) {
		t.Errorf("discarding changed the index:\n%s", got)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "\n+A\n") || !strings.Contains(unstaged, "\n+X\n") ||
		strings.Contains(unstaged, "+B") || strings.Contains(unstaged, "-25") {
		t.Errorf("worktree diff should hold just +A and +X:\n%s", unstaged)
	}
}

// TestHunkViewDiscardsTheSecondHunkWhole: only the second hunk is picked;
// the first one, two lines longer and above it, stays in the file.
func TestHunkViewDiscardsTheSecondHunkWhole(t *testing.T) {
	repo := twoHunkRepo(t, "sub/f.txt")
	path := filepath.Join(repo, "sub", "f.txt")
	p := openStatusPanelIn(t, filepath.Join(repo, "sub"))
	v := openHunksOf(t, p, modeDiscard)

	moveTo(t, v, 8) // the second hunk's @@ line
	v.ProcessKey(key(vtinput.VK_INSERT))
	discardConfirm(t, v).OnResult(0)
	if !v.IsDone() {
		t.Fatal("the view did not close after discarding")
	}
	if got, want := readRepoFile(t, path), numbered(1, 2)+"A\nB\n"+numbered(3, 30); got != want {
		t.Errorf("working file after discarding the second hunk:\n%s\nwant\n%s", got, want)
	}
}

// TestHunkViewDiscardGoesBackToTheIndexNotHead: "A" and "B" are staged,
// "X" is only in the working file. F8 offers just the "X" hunk, and
// discarding it leaves the file as the index has it -- with "A" and "B".
func TestHunkViewDiscardGoesBackToTheIndexNotHead(t *testing.T) {
	repo := realGitRepo(t)
	path := filepath.Join(repo, "f.txt")
	writeRepoFile(t, path, numbered(1, 30))
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	staged := numbered(1, 2) + "A\nB\n" + numbered(3, 30)
	writeRepoFile(t, path, staged)
	runRealGit(t, repo, "add", "-A")
	writeRepoFile(t, path, numbered(1, 2)+"A\nB\n"+numbered(3, 24)+"X\n"+numbered(26, 30))

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	if len(v.patch.hunks) != 1 {
		t.Fatalf("hunks = %d, want 1 (the unstaged X change only)", len(v.patch.hunks))
	}
	v.ProcessKey(key(vtinput.VK_INSERT))
	discardConfirm(t, v).OnResult(0)

	if got := readRepoFile(t, path); got != staged {
		t.Errorf("working file after discarding:\n%s\nwant the index content\n%s", got, staged)
	}
	if got := runRealGit(t, repo, "show", ":f.txt"); got != staged {
		t.Errorf("discarding changed the index:\n%s", got)
	}
}

// TestHunkViewDiscardCancelledChangesNothing: Cancel (or Esc, any code but
// 0) on the confirmation leaves the file, the index and the view as they
// were, with the picks still in place.
func TestHunkViewDiscardCancelledChangesNothing(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	path := filepath.Join(repo, "f.txt")
	before := readRepoFile(t, path)
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)

	v.ProcessKey(key(vtinput.VK_INSERT)) // the first hunk
	v.ProcessKey(key(vtinput.VK_INSERT)) // the second one
	for _, code := range []int{1, -1} {
		discardConfirm(t, v).OnResult(code)
		if v.IsDone() {
			t.Fatalf("code %d: the view closed without a confirmed discard", code)
		}
		if got := readRepoFile(t, path); got != before {
			t.Fatalf("code %d: the working file changed:\n%s", code, got)
		}
		if got := runRealGit(t, repo, "show", ":f.txt"); got != numbered(1, 30) {
			t.Fatalf("code %d: the index changed:\n%s", code, got)
		}
		if n := v.patch.selectedCount(); n != 2 {
			t.Fatalf("code %d: picked hunks = %d, want 2 still picked", code, n)
		}
	}
}

// TestHunkViewDiscardRestoresADeletedFile: a file deleted from the working
// tree is one deletion hunk; discarding it whole brings the file back. A
// part of it is asked about like any other pick (cancelled here, the
// picked lines go in TestHunkViewDiscardsPartOfADeletedFile).
func TestHunkViewDiscardRestoresADeletedFile(t *testing.T) {
	repo := realGitRepo(t)
	path := filepath.Join(repo, "f.txt")
	writeRepoFile(t, path, "one\ntwo\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	moveTo(t, v, 1) // "-one" alone
	v.ProcessKey(key(vtinput.VK_INSERT))
	discardConfirm(t, v).OnResult(1) // "Cancel"
	if _, err := os.Stat(path); err == nil {
		t.Fatal("cancelling brought the file back")
	}

	moveTo(t, v, 0)
	v.ProcessKey(key(vtinput.VK_INSERT)) // the whole hunk
	discardConfirm(t, v).OnResult(0)
	if got := readRepoFile(t, path); got != "one\ntwo\n" {
		t.Errorf("restored file = %q, want %q", got, "one\ntwo\n")
	}
}

// TestHunkViewDiscardNeedsAPick: Enter with nothing picked asks nothing.
func TestHunkViewDiscardNeedsAPick(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeDiscard)
	top := vtui.FrameManager.GetTopFrame()
	v.ProcessKey(key(vtinput.VK_F2))
	if vtui.FrameManager.GetTopFrame() != top || v.IsDone() {
		t.Fatal("Enter/F2 with nothing picked must neither ask nor close")
	}
}

// TestDiscardHunkViewScreenDump draws F8's view with "+B" and "-25" picked
// and the confirmation over it, in English and Russian; run it with -v to
// see the screens.
func TestDiscardHunkViewScreenDump(t *testing.T) {
	t.Cleanup(func() { i18n.InitLang("", "", "") })
	for _, lang := range []string{"en", "ru"} {
		i18n.InitLang(lang, "en", "")
		repo := twoHunkRepo(t, "f.txt")
		p := openStatusPanelIn(t, repo)
		v := openHunksOf(t, p, modeDiscard)

		scr := vtui.NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		vtui.FrameManager.Init(scr)
		v.SetPosition(0, 0, 79, 24)
		moveTo(t, v, rowB)
		v.ProcessKey(key(vtinput.VK_INSERT))
		moveTo(t, v, rowMinus)
		v.ProcessKey(key(vtinput.VK_INSERT))
		confirm := discardConfirm(t, v)

		v.Show(scr)
		confirm.Show(scr)
		var b strings.Builder
		scr.Dump(&b)
		text, _, _ := strings.Cut(b.String(), "--- CELL METADATA")
		t.Logf("%s:\n%s", lang, text)
		title := strings.Split(fmt.Sprintf(i18n.Msg("GitHunks.DiscardTitle"), "f.txt", 2, 2), " (")[0]
		for _, want := range []string{title, "@@ -1,5 +1,7 @@ [1/2]", "f.txt?", strings.ReplaceAll(i18n.Msg("GitHunks.DiscardButton"), "&", "")} {
			if !strings.Contains(text, want) {
				t.Errorf("%s screen lacks %q:\n%s", lang, want, text)
			}
		}
	}
}
