package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// realGitRepo creates an empty repository in a temporary directory, with
// just enough local configuration for commits to work anywhere: an
// identity, no signing, and no CRLF conversion (Git for Windows turns it
// on globally, and its warnings would only add noise to these tests).
func realGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	runRealGit(t, dir, "init", "-q")
	runRealGit(t, dir, "config", "user.name", "f4 test")
	runRealGit(t, dir, "config", "user.email", "f4@example.invalid")
	runRealGit(t, dir, "config", "commit.gpgsign", "false")
	runRealGit(t, dir, "config", "core.autocrlf", "false")
	return dir
}

func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeRepoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// numbered returns lines "1".."n", each followed by a newline.
func numbered(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

// twoHunkRepo commits a 30-line file at rel inside a fresh repository and
// then changes it in two places far enough apart to give two hunks: two
// lines inserted after line 2 (the first hunk grows the file) and line 25
// replaced by "X".
func twoHunkRepo(t *testing.T, rel string) string {
	t.Helper()
	repo := realGitRepo(t)
	path := filepath.Join(repo, filepath.FromSlash(rel))
	writeRepoFile(t, path, numbered(1, 30))
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, path, numbered(1, 2)+"A\nB\n"+numbered(3, 24)+"X\n"+numbered(26, 30))
	return repo
}

func openStatusPanelIn(t *testing.T, dir string) *statusPanel {
	t.Helper()
	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: dir}, Bounds: [4]int{0, 0, 59, 19}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close() })
	return controller.(*statusPanel)
}

func key(vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
}

func openHunksForOnlyEntry(t *testing.T, p *statusPanel) *HunkView {
	t.Helper()
	return openHunksOf(t, p, modeStage)
}

// openHunksOf opens the HunkView F4 (modeStage), Shift+F4 (modeUnstage)
// or F8 (modeDiscard) would open for the entry under the cursor.
func openHunksOf(t *testing.T, p *statusPanel, mode hunkMode) *HunkView {
	t.Helper()
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("status panel has no entry under the cursor")
	}
	v, err := p.openHunkView(entry, mode)
	if err != nil {
		t.Fatalf("openHunkView: %v", err)
	}
	v.SetPosition(0, 0, 59, 19)
	return v
}

func TestParseFilePatchSkipsLeadingNoiseAndKeepsLinesVerbatim(t *testing.T) {
	diff := "warning: in the working copy of 'f', LF will be replaced by CRLF\n" +
		"diff --git a/f b/f\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1 +1,2 @@ func main\n" +
		" one\r\n" +
		"+two\n" +
		"@@ -9,3 +10,2 @@\n" +
		" a\n" +
		"-b\n" +
		" c\n" +
		"\\ No newline at end of file\n"
	fp, err := parseFilePatch(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.header) != 4 || fp.header[0] != "diff --git a/f b/f" {
		t.Fatalf("header = %q", fp.header)
	}
	if len(fp.hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(fp.hunks))
	}
	h := fp.hunks[0]
	if h.oldStart != 1 || h.oldCount != 1 || h.newStart != 1 || h.newCount != 2 || h.section != " func main" {
		t.Errorf("first hunk = %+v", *h)
	}
	if h.lines[0] != " one\r" {
		t.Errorf("CR was not kept: %q", h.lines[0])
	}
	if got := fp.hunks[1].lines; len(got) != 4 || got[3] != "\\ No newline at end of file" {
		t.Errorf("second hunk lines = %q", got)
	}
}

func TestParseFilePatchWithoutHunks(t *testing.T) {
	for _, diff := range []string{
		"",
		"diff --git a/img b/img\nindex 1..2 100644\nBinary files a/img and b/img differ\n",
		"diff --git a/s b/s\nold mode 100644\nnew mode 100755\n",
	} {
		if _, err := parseFilePatch(diff); !errors.Is(err, errNoHunks) {
			t.Errorf("parseFilePatch(%q) error = %v, want errNoHunks", diff, err)
		}
	}
}

func TestBuildPatchShiftsKeptHunksAndDropsModeChange(t *testing.T) {
	fp := &filePatch{
		header: []string{"diff --git a/f b/f", "old mode 100644", "new mode 100755", "index 1..2", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{
			testHunk(1, 5, 1, 7, " 1", " 2", "+A", "+B", " 3", " 4", " 5"),
			testHunk(22, 7, 24, 7, " 22", " 23", " 24", "-25", "+X", " 26", " 27", " 28"),
		},
	}
	fp.hunks[1].pickAll(true)
	want := "diff --git a/f b/f\nindex 1..2\n--- a/f\n+++ b/f\n@@ -22,7 +22,7 @@\n 22\n 23\n 24\n-25\n+X\n 26\n 27\n 28\n"
	if got := mustBuildPatch(t, fp); got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}

	fp.hunks[1].pickAll(false)
	if got := mustBuildPatch(t, fp); got != "" {
		t.Errorf("buildPatch with nothing selected = %q, want empty", got)
	}
}

// TestHunkViewStagesOnlyThePickedHunk walks the real UI on a real
// repository: F4's view lists both hunks, the cursor goes down to the
// second one, Insert picks it, Enter stages it -- and the index then has
// exactly that change while the first hunk stays unstaged. Staging the
// second hunk alone is the case that needs buildPatch's line shift, since
// the first hunk (left out) inserts two lines above it.
func TestHunkViewStagesOnlyThePickedHunk(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)

	if n := len(v.patch.hunks); n != 2 {
		t.Fatalf("hunks = %d, want 2", n)
	}
	for i := 0; i <= len(v.patch.hunks[0].lines); i++ {
		v.ProcessKey(key(vtinput.VK_DOWN))
	}
	row, ok := v.cursorRow()
	if !ok || !row.header() || row.hunk != v.patch.hunks[1] {
		t.Fatalf("cursor is not on the second hunk's @@ line: %+v", row)
	}
	if !v.ProcessKey(key(vtinput.VK_INSERT)) {
		t.Fatal("Insert was not claimed")
	}
	if v.patch.hunks[0].anyPicked() || !v.patch.hunks[1].allPicked() {
		t.Fatalf("selection = %v/%v, want false/true", v.patch.hunks[0].anyPicked(), v.patch.hunks[1].allPicked())
	}
	if !v.ProcessKey(key(vtinput.VK_RETURN)) {
		t.Fatal("Enter was not claimed")
	}
	if !v.IsDone() {
		t.Error("the view did not close after staging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+X") || strings.Contains(staged, "+A") {
		t.Errorf("index diff should hold only the second hunk:\n%s", staged)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "+A") || strings.Contains(unstaged, "+X") {
		t.Errorf("worktree diff should hold only the first hunk:\n%s", unstaged)
	}
	entry, ok := p.selectedEntry()
	if !ok || entry.Path != "f.txt" || entry.XY != "MM" {
		t.Errorf("status panel after staging: %+v, want f.txt MM", entry)
	}
}

// TestHunkViewInSubdirectory: the panel shows paths relative to its own
// directory, `git diff` prints them relative to the repository root, and
// `git apply` run from a subdirectory would silently skip them -- this is
// the case applyFilePatch runs at the top level for.
func TestHunkViewInSubdirectory(t *testing.T) {
	repo := twoHunkRepo(t, "sub/f.txt")
	p := openStatusPanelIn(t, filepath.Join(repo, "sub"))
	v := openHunksForOnlyEntry(t, p)

	// Insert on the first hunk picks it and jumps to the second.
	v.ProcessKey(key(vtinput.VK_INSERT))
	if row, _ := v.cursorRow(); row.hunk != v.patch.hunks[1] || !row.header() {
		t.Errorf("Insert did not move the cursor to the next hunk")
	}
	v.ProcessKey(key(vtinput.VK_F2))

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+A") || strings.Contains(staged, "+X") {
		t.Errorf("index diff should hold only the first hunk:\n%s", staged)
	}
}

func TestHunkViewTogglesBackAndRefusesEmptySelection(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)

	v.ProcessKey(key(vtinput.VK_SPACE)) // picks hunk 1, moves to hunk 2
	v.ProcessKey(key(vtinput.VK_UP))
	v.ProcessKey(key(vtinput.VK_SPACE)) // cursor inside hunk 1: drops it again
	if v.patch.selectedCount() != 0 {
		t.Fatalf("selected = %d, want 0", v.patch.selectedCount())
	}
	v.ProcessKey(key(vtinput.VK_RETURN))
	if v.IsDone() {
		t.Error("Enter with nothing picked closed the view")
	}
	if staged := runRealGit(t, repo, "diff", "--cached"); staged != "" {
		t.Errorf("nothing should be staged:\n%s", staged)
	}
	v.ProcessKey(key(vtinput.VK_ESCAPE))
	if !v.IsDone() {
		t.Error("Esc did not close the view")
	}
}

// An untracked file has no `git diff`, but its lines can be picked from a
// diff against /dev/null (f4#659 part 24): the view opens with one hunk, and
// staging a picked line creates the index entry with just that line.
func TestHunksOfUntrackedFileAreOffered(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "new.txt"), "one\ntwo\nthree\n")
	p := openStatusPanelIn(t, repo)
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("no entry")
	}
	v, err := p.openHunkView(entry, modeStage)
	if err != nil {
		t.Fatalf("openHunkView(untracked) error = %v", err)
	}
	if len(v.patch.hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(v.patch.hunks))
	}
	pickLines(v.patch.hunks[0], 1) // the "two" line only
	if err := applyFilePatch(context.Background(), repo, v.patch); err != nil {
		t.Fatalf("applyFilePatch: %v", err)
	}
	if got := runRealGit(t, repo, "show", ":new.txt"); got != "two\n" {
		t.Errorf("index content = %q, want %q", got, "two\n")
	}
	if got := readRepoFile(t, filepath.Join(repo, "new.txt")); got != "one\ntwo\nthree\n" {
		t.Errorf("working file changed: %q", got)
	}
}

// TestHunkViewScreenDump draws the view after picking the first hunk; run
// it with -v to see the screen.
func TestHunkViewScreenDump(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)
	v.SetPosition(0, 0, 59, 21)
	v.ProcessKey(key(vtinput.VK_INSERT))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 22)
	v.Show(scr)
	var b strings.Builder
	scr.Dump(&b)
	text := b.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	t.Log("\n" + text)
	if !strings.Contains(text, "f.txt") || !strings.Contains(text, "@@ -1,5 +1,7 @@") {
		t.Errorf("screen misses the title or the first hunk:\n%s", text)
	}
}

func TestBuildPatchStagedShiftsOldSide(t *testing.T) {
	fp := &filePatch{
		mode:   modeUnstage,
		header: []string{"diff --git a/f b/f", "index 1..2 100644", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{
			testHunk(1, 5, 1, 7, " 1", " 2", "+A", "+B", " 3", " 4", " 5"),
			testHunk(22, 7, 24, 7, " 22", " 23", " 24", "-25", "+X", " 26", " 27", " 28"),
		},
	}
	fp.hunks[1].pickAll(true)
	// The first hunk stays in the index, so the kept one's "-a" moves
	// forward by its two lines; "+c" already counts lines of the index.
	want := "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n@@ -24,7 +24,7 @@\n 22\n 23\n 24\n-25\n+X\n 26\n 27\n 28\n"
	if got := mustBuildPatch(t, fp); got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}
}

// stagedTwoHunkRepo is twoHunkRepo with both hunks staged.
func stagedTwoHunkRepo(t *testing.T, rel string) string {
	t.Helper()
	repo := twoHunkRepo(t, rel)
	runRealGit(t, repo, "add", "-A")
	return repo
}

// TestHunkViewUnstagesOnlyThePickedHunk is the `git reset -p` side of
// TestHunkViewStagesOnlyThePickedHunk: both hunks staged, Shift+F4's view
// lists them from `git diff --cached`, the second one is picked, Enter
// takes it out of the index -- the first hunk (left in, two lines above)
// is the case needing the shift of the "-a" side.
func TestHunkViewUnstagesOnlyThePickedHunk(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)

	if v.patch.mode != modeUnstage || len(v.patch.hunks) != 2 {
		t.Fatalf("mode = %v, hunks = %d; want modeUnstage, 2", v.patch.mode, len(v.patch.hunks))
	}
	for i := 0; i <= len(v.patch.hunks[0].lines); i++ {
		v.ProcessKey(key(vtinput.VK_DOWN))
	}
	v.ProcessKey(key(vtinput.VK_INSERT))
	if v.patch.hunks[0].anyPicked() || !v.patch.hunks[1].allPicked() {
		t.Fatalf("selection = %v/%v, want false/true", v.patch.hunks[0].anyPicked(), v.patch.hunks[1].allPicked())
	}
	if patch := mustBuildPatch(t, v.patch); !strings.Contains(patch, "@@ -24,7 +24,7 @@") {
		t.Errorf("unstage patch does not carry the shifted hunk header:\n%s", patch)
	}
	if !v.ProcessKey(key(vtinput.VK_RETURN)) {
		t.Fatal("Enter was not claimed")
	}
	if !v.IsDone() {
		t.Error("the view did not close after unstaging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+A") || strings.Contains(staged, "+X") {
		t.Errorf("index diff should keep only the first hunk:\n%s", staged)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "+X") || strings.Contains(unstaged, "+A") {
		t.Errorf("worktree diff should hold only the second hunk:\n%s", unstaged)
	}
	if got := runRealGit(t, repo, "show", ":f.txt"); got != numbered(1, 2)+"A\nB\n"+numbered(3, 30) {
		t.Errorf("index content after unstaging:\n%s", got)
	}
	entry, ok := p.selectedEntry()
	if !ok || entry.Path != "f.txt" || entry.XY != "MM" {
		t.Errorf("status panel after unstaging: %+v, want f.txt MM", entry)
	}
}

// TestHunkViewUnstagesInSubdirectory: `git apply --cached -R` has to run
// at the repository root as well.
func TestHunkViewUnstagesInSubdirectory(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "sub/f.txt")
	p := openStatusPanelIn(t, filepath.Join(repo, "sub"))
	v := openHunksOf(t, p, modeUnstage)

	v.ProcessKey(key(vtinput.VK_INSERT)) // the first hunk
	v.ProcessKey(key(vtinput.VK_F2))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+X") || strings.Contains(staged, "+A") {
		t.Errorf("index diff should keep only the second hunk:\n%s", staged)
	}
	if got := runRealGit(t, repo, "show", ":sub/f.txt"); got != numbered(1, 24)+"X\n"+numbered(26, 30) {
		t.Errorf("index content after unstaging:\n%s", got)
	}
}

// TestUnstagingTheOnlyHunkOfANewFile: a staged new file is one hunk from
// /dev/null; unstaging it takes the file out of the index and leaves it
// untracked, the same end state Insert gives.
func TestUnstagingTheOnlyHunkOfANewFile(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "new.txt"), "one\ntwo\n")
	runRealGit(t, repo, "add", "new.txt")

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)
	if len(v.patch.hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(v.patch.hunks))
	}
	v.ProcessKey(key(vtinput.VK_INSERT))
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}
	if got := runRealGit(t, repo, "status", "--porcelain"); got != "?? new.txt\n" {
		t.Errorf("status after unstaging the new file = %q, want untracked", got)
	}
}

// TestStagedHunksNotOffered: Shift+F4 has nothing to show for a file with
// no staged changes, nor for a staged rename (see openHunkView).
func TestStagedHunksNotOffered(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt") // changed, nothing staged
	p := openStatusPanelIn(t, repo)
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("no entry")
	}
	if _, err := p.openHunkView(entry, modeUnstage); !errors.Is(err, errNoHunks) {
		t.Errorf("openHunkView(unstaged only, staged) error = %v, want errNoHunks", err)
	}
	shiftF4 := key(vtinput.VK_F4)
	shiftF4.ControlKeyState = vtinput.ShiftPressed
	if !p.ProcessKey(shiftF4) {
		t.Error("Shift+F4 was not claimed")
	}

	runRealGit(t, repo, "checkout", "--", "f.txt")
	runRealGit(t, repo, "mv", "f.txt", "g.txt")
	if err := p.reload(); err != nil {
		t.Fatal(err)
	}
	entry, ok = p.selectedEntry()
	if !ok || entry.OrigPath == "" {
		t.Fatalf("expected a rename entry, got %+v", entry)
	}
	if _, err := p.openHunkView(entry, modeUnstage); !errors.Is(err, errNoHunks) {
		t.Errorf("openHunkView(rename, staged) error = %v, want errNoHunks", err)
	}
}

// TestUnstageHunkViewScreenDump draws the Shift+F4 view after picking the
// first hunk; run it with -v to see the screen.
func TestUnstageHunkViewScreenDump(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)
	v.SetPosition(0, 0, 59, 21)
	v.ProcessKey(key(vtinput.VK_INSERT))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 22)
	v.Show(scr)
	var b strings.Builder
	scr.Dump(&b)
	text := b.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	t.Log("\n" + text)
	if !strings.Contains(text, "Unstage hunks: f.txt") || !strings.Contains(text, "@@ -22,7 +24,7 @@") {
		t.Errorf("screen misses the title or the second hunk:\n%s", text)
	}
}

// testHunk builds a diffHunk from its header numbers and body lines, with
// nothing picked.
func testHunk(oldStart, oldCount, newStart, newCount int, lines ...string) *diffHunk {
	return &diffHunk{oldStart: oldStart, oldCount: oldCount, newStart: newStart, newCount: newCount, lines: lines, picked: make([]bool, len(lines))}
}

func mustBuildPatch(t *testing.T, fp *filePatch) string {
	t.Helper()
	patch, err := buildPatch(fp)
	if err != nil {
		t.Fatalf("buildPatch: %v", err)
	}
	return patch
}

// pickLines picks the body lines of h with the given indexes.
func pickLines(h *diffHunk, idx ...int) {
	for _, i := range idx {
		h.picked[i] = true
	}
}

// TestBuildPatchPicksSingleLines checks the rebuilt hunks and their
// headers for a few picks of single lines, in both directions. The first
// hunk replaces "2" with "X" and "Y" (one line longer); the second hunk,
// picked whole, has to move by whatever the rebuilt first hunk changes.
func TestBuildPatchPicksSingleLines(t *testing.T) {
	const head = "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n"
	for _, tc := range []struct {
		name string
		mode hunkMode
		pick []int // lines of the first hunk
		want string
	}{
		{"stage +Y", modeStage, []int{3},
			"@@ -1,3 +1,4 @@\n 1\n 2\n+Y\n 3\n@@ -20,3 +21,3 @@\n 20\n-21\n+Z\n 22\n"},
		{"stage -2", modeStage, []int{1},
			"@@ -1,3 +1,2 @@\n 1\n-2\n 3\n@@ -20,3 +19,3 @@\n 20\n-21\n+Z\n 22\n"},
		{"unstage +Y", modeUnstage, []int{3},
			"@@ -1,3 +1,4 @@\n 1\n X\n+Y\n 3\n@@ -20,3 +21,3 @@\n 20\n-21\n+Z\n 22\n"},
		{"unstage -2", modeUnstage, []int{1},
			"@@ -1,5 +1,4 @@\n 1\n-2\n X\n Y\n 3\n@@ -22,3 +21,3 @@\n 20\n-21\n+Z\n 22\n"},
	} {
		fp := &filePatch{
			mode:   tc.mode,
			header: []string{"diff --git a/f b/f", "index 1..2 100644", "--- a/f", "+++ b/f"},
			hunks: []*diffHunk{
				testHunk(1, 3, 1, 4, " 1", "-2", "+X", "+Y", " 3"),
				testHunk(20, 3, 21, 3, " 20", "-21", "+Z", " 22"),
			},
		}
		pickLines(fp.hunks[0], tc.pick...)
		fp.hunks[1].pickAll(true)
		if got := mustBuildPatch(t, fp); got != head+tc.want {
			t.Errorf("%s: buildPatch =\n%s\nwant\n%s", tc.name, got, head+tc.want)
		}
	}
}

func TestBuildPatchRefusesUnrepresentablePicks(t *testing.T) {
	// A new file may be picked in part too (its lines are the added ones).
	fp := &filePatch{
		header: []string{"diff --git a/n b/n", "new file mode 100644", "index 0000000..1", "--- /dev/null", "+++ b/n"},
		hunks:  []*diffHunk{testHunk(0, 0, 1, 2, "+one", "+two")},
	}
	pickLines(fp.hunks[0], 0)
	if _, err := buildPatch(fp); err != nil {
		t.Errorf("part of a new file: %v", err)
	}

	// "b" loses its missing newline and "c" follows it. Keeping the old
	// "b" (no newline) while adding lines after it has no file behind it.
	fp = &filePatch{
		header: []string{"diff --git a/f b/f", "index 1..2 100644", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{testHunk(1, 2, 1, 3,
			" a", "-b", "\\ No newline at end of file", "+b", "+c", "\\ No newline at end of file")},
	}
	pickLines(fp.hunks[0], 3, 4)
	if _, err := buildPatch(fp); !errors.Is(err, errNoNewlineInside) {
		t.Errorf("old last line kept before new lines: error = %v, want errNoNewlineInside", err)
	}
	fp.hunks[0].pickAll(false)
	pickLines(fp.hunks[0], 1, 3) // just give "b" its newline
	want := "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n@@ -1,2 +1,2 @@\n a\n-b\n\\ No newline at end of file\n+b\n"
	if got := mustBuildPatch(t, fp); got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}
}

// moveTo puts the HunkView cursor on row pos (rows counted from the top:
// the first hunk's "@@" line is 0).
func moveTo(t *testing.T, v *HunkView, pos int) {
	t.Helper()
	for v.table.SelectPos < pos {
		before := v.table.SelectPos
		v.ProcessKey(key(vtinput.VK_DOWN))
		if v.table.SelectPos == before {
			t.Fatalf("cursor stuck at row %d, wanted row %d", before, pos)
		}
	}
	for v.table.SelectPos > pos {
		before := v.table.SelectPos
		v.ProcessKey(key(vtinput.VK_UP))
		if v.table.SelectPos == before {
			t.Fatalf("cursor stuck at row %d, wanted row %d", before, pos)
		}
	}
}

// In twoHunkRepo's diff (both directions) the rows are:
//
//	0 @@ first hunk   1 " 1"  2 " 2"  3 "+A"  4 "+B"  5 " 3"  6 " 4"  7 " 5"
//	8 @@ second hunk  9 " 22" 10 " 23" 11 " 24" 12 "-25" 13 "+X" ...
const (
	rowA     = 3
	rowB     = 4
	rowMinus = 12
	rowX     = 13
)

// halfPickedContent is the file twoHunkRepo starts from with "A" inserted
// and "X" added after "25" (which stays): what staging "+A" and "+X"
// alone, or unstaging "+B" and "-25" alone, leaves in the index.
func halfPickedContent() string {
	return numbered(1, 2) + "A\n" + numbered(3, 25) + "X\n" + numbered(26, 30)
}

// TestHunkViewStagesSingleLines picks single lines of both hunks on a
// real repository: "+A" (not "+B") of the first hunk and "+X" (not "-25")
// of the second. The second hunk's "+" start then moves by one line, not
// by the first hunk's two, and "-25" has to become context.
func TestHunkViewStagesSingleLines(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)

	moveTo(t, v, rowA)
	v.ProcessKey(key(vtinput.VK_INSERT))
	if v.table.SelectPos != rowB {
		t.Errorf("Insert on a line moved the cursor to row %d, want %d", v.table.SelectPos, rowB)
	}
	if got := v.patch.hunks[0].headerLine(); !strings.HasSuffix(got, " [1/2]") {
		t.Errorf("half-picked hunk header = %q, want a [1/2] suffix", got)
	}
	if row, _ := v.cursorRow(); row.IsSelected() {
		t.Error("the unpicked +B row is painted as picked")
	}
	moveTo(t, v, rowX)
	v.ProcessKey(key(vtinput.VK_INSERT))
	if n := v.patch.selectedCount(); n != 2 {
		t.Fatalf("hunks with picked lines = %d, want 2", n)
	}
	if patch := mustBuildPatch(t, v.patch); !strings.Contains(patch, "@@ -22,7 +23,8 @@") || !strings.Contains(patch, "\n 25\n+X\n") {
		t.Errorf("stage patch does not carry the rebuilt second hunk:\n%s", patch)
	}
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after staging")
	}

	if got := runRealGit(t, repo, "show", ":f.txt"); got != halfPickedContent() {
		t.Errorf("index content after staging lines:\n%s", got)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "\n+B\n") || !strings.Contains(unstaged, "\n-25\n") ||
		strings.Contains(unstaged, "+A") || strings.Contains(unstaged, "+X") {
		t.Errorf("worktree diff should hold just +B and -25:\n%s", unstaged)
	}
}

// TestHunkViewUnstagesSingleLines is the other direction on the same
// file: everything staged, then "+B" and "-25" taken back out -- the index
// ends up where TestHunkViewStagesSingleLines left it. Here "+A" and "+X"
// become context, "-25" puts the line back, and the second hunk's "-"
// start moves by the one line "+B" took out.
func TestHunkViewUnstagesSingleLines(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)

	moveTo(t, v, rowB)
	v.ProcessKey(key(vtinput.VK_INSERT))
	moveTo(t, v, rowMinus)
	v.ProcessKey(key(vtinput.VK_INSERT))
	if v.table.SelectPos != rowX {
		t.Errorf("Insert on a line moved the cursor to row %d, want %d", v.table.SelectPos, rowX)
	}
	if patch := mustBuildPatch(t, v.patch); !strings.Contains(patch, "@@ -23,8 +24,7 @@") || !strings.Contains(patch, "\n-25\n X\n") {
		t.Errorf("unstage patch does not carry the rebuilt second hunk:\n%s", patch)
	}
	v.ProcessKey(key(vtinput.VK_F2))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}

	if got := runRealGit(t, repo, "show", ":f.txt"); got != halfPickedContent() {
		t.Errorf("index content after unstaging lines:\n%s", got)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "\n+B\n") || !strings.Contains(unstaged, "\n-25\n") ||
		strings.Contains(unstaged, "+A") || strings.Contains(unstaged, "+X") {
		t.Errorf("worktree diff should hold just +B and -25:\n%s", unstaged)
	}
}

// TestHunkViewLineAndHunkToggleTogether: Insert on the "@@" line of a
// half-picked hunk picks the rest of it, and again drops it whole.
func TestHunkViewLineAndHunkToggleTogether(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)
	h := v.patch.hunks[0]

	moveTo(t, v, rowA)
	v.ProcessKey(key(vtinput.VK_SPACE))
	moveTo(t, v, 0)
	v.ProcessKey(key(vtinput.VK_SPACE))
	if !h.allPicked() {
		t.Fatal("Space on the @@ line of a half-picked hunk did not pick it whole")
	}
	if v.table.SelectPos != 8 {
		t.Errorf("cursor at row %d after a hunk toggle, want the next @@ line (8)", v.table.SelectPos)
	}
	moveTo(t, v, 1) // a context line toggles the hunk as well
	v.ProcessKey(key(vtinput.VK_SPACE))
	if h.anyPicked() {
		t.Error("Space on a context line of a picked hunk did not drop it")
	}
}

// TestHunkViewStagesAroundMissingNewline: the committed file ends in "b"
// with no newline, the worktree has "b\nc" (again no newline). Staging
// "-b" and "+b" alone gives "b" its newline and nothing more -- the
// marker after the unpicked "+c" has to go with it.
func TestHunkViewStagesAroundMissingNewline(t *testing.T) {
	repo := realGitRepo(t)
	path := filepath.Join(repo, "f.txt")
	writeRepoFile(t, path, "a\nb")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, path, "a\nb\nc")

	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)
	h := v.patch.hunks[0]
	want := []string{" a", "-b", "\\ No newline at end of file", "+b", "+c", "\\ No newline at end of file"}
	if strings.Join(h.lines, "|") != strings.Join(want, "|") {
		t.Fatalf("hunk lines = %q, want %q", h.lines, want)
	}
	moveTo(t, v, 2) // "-b"
	v.ProcessKey(key(vtinput.VK_INSERT))
	moveTo(t, v, 4) // "+b"
	v.ProcessKey(key(vtinput.VK_INSERT))
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after staging")
	}
	if got := runRealGit(t, repo, "show", ":f.txt"); got != "a\nb\n" {
		t.Errorf("index content = %q, want %q", got, "a\nb\n")
	}
}

// TestHunkViewLinePickScreenDump draws the view with "+A" of the first
// hunk and "+X" of the second picked; run it with -v to see the screen.
func TestHunkViewLinePickScreenDump(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)
	v.SetPosition(0, 0, 59, 21)
	moveTo(t, v, rowA)
	v.ProcessKey(key(vtinput.VK_INSERT))
	moveTo(t, v, rowX)
	v.ProcessKey(key(vtinput.VK_INSERT))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 22)
	v.Show(scr)
	var b strings.Builder
	scr.Dump(&b)
	text := b.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	t.Log("\n" + text)
	if !strings.Contains(text, "@@ -1,5 +1,7 @@ [1/2]") || !strings.Contains(text, "@@ -22,7 +24,7 @@ [1/2]") {
		t.Errorf("screen misses the [1/2] marks of the half-picked hunks:\n%s", text)
	}
}
