package git

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// untrackedConfirm presses F8 on the status panel's selected entry and
// returns the delete confirmation it must have pushed -- the same shape
// discardConfirm (hunk_discard_test.go) checks for HunkView's own Enter.
func untrackedConfirm(t *testing.T, p *statusPanel) *vtui.Window {
	t.Helper()
	before := vtui.FrameManager.GetTopFrame()
	if !p.ProcessKey(key(vtinput.VK_F8)) {
		t.Fatal("F8 was not claimed")
	}
	confirm, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || confirm == before || confirm.OnResult == nil {
		t.Fatalf("top frame after F8 is %T, want the *vtui.Window delete confirmation", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(func() { vtui.FrameManager.RemoveFrame(confirm) })
	return confirm
}

// TestF8OnUntrackedFileOffersToDeleteIt: F8 on the sole entry of a fresh
// repo's status (an untracked file) asks first and removes it from disk
// only once the delete button is confirmed.
func TestF8OnUntrackedFileOffersToDeleteIt(t *testing.T) {
	repo := realGitRepo(t)
	path := filepath.Join(repo, "new.txt")
	writeRepoFile(t, path, "hello\n")
	p := openStatusPanelIn(t, repo)

	confirm := untrackedConfirm(t, p)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the file was removed before the delete was confirmed: %v", err)
	}
	confirm.OnResult(0) // "Delete"

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("stat after delete: err = %v, want a not-exist error", err)
	}
	if p.table.ItemCount != 0 {
		t.Errorf("panel rows after delete = %d, want 0", p.table.ItemCount)
	}
}

// TestF8OnUntrackedDirectoryDeletesItRecursively: git status reports an
// untracked directory as one first-level "sub/" entry, not descended into
// (parseStatus, status.go); F8 on it has to remove the whole directory,
// not just try (and fail) to os.Remove a non-empty one.
func TestF8OnUntrackedDirectoryDeletesItRecursively(t *testing.T) {
	repo := realGitRepo(t)
	dir := filepath.Join(repo, "sub")
	writeRepoFile(t, filepath.Join(dir, "a.txt"), "a\n")
	writeRepoFile(t, filepath.Join(dir, "b.txt"), "b\n")
	p := openStatusPanelIn(t, repo)

	entry, ok := p.selectedEntry()
	if !ok || entry.Path != "sub/" {
		t.Fatalf("selected entry = %+v, want the untracked directory sub/", entry)
	}

	untrackedConfirm(t, p).OnResult(0)

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("stat after delete: err = %v, want a not-exist error", err)
	}
	if p.table.ItemCount != 0 {
		t.Errorf("panel rows after delete = %d, want 0", p.table.ItemCount)
	}
}

// TestF8OnUntrackedFileCancelledChangesNothing: Cancel (or Esc, any code
// but 0) leaves the file and the panel as they were, the same guarantee
// TestHunkViewDiscardCancelledChangesNothing (hunk_discard_test.go) checks
// for a tracked change's own F8.
func TestF8OnUntrackedFileCancelledChangesNothing(t *testing.T) {
	repo := realGitRepo(t)
	path := filepath.Join(repo, "new.txt")
	writeRepoFile(t, path, "hello\n")
	p := openStatusPanelIn(t, repo)

	for _, code := range []int{1, -1} {
		untrackedConfirm(t, p).OnResult(code)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("code %d: the file was removed: %v", code, err)
		}
		if p.table.ItemCount != 1 {
			t.Fatalf("code %d: panel rows = %d, want 1 still", code, p.table.ItemCount)
		}
	}
}

// TestF8NextToATrackedChangeOnlyDeletesTheUntrackedOne checks the two F8
// paths do not bleed into each other in the same status list: an
// untracked file offers to delete it, while a tracked modification right
// next to it still opens HunkView as before.
func TestF8NextToATrackedChangeOnlyDeletesTheUntrackedOne(t *testing.T) {
	repo := realGitRepo(t)
	trackedPath := filepath.Join(repo, "tracked.txt")
	writeRepoFile(t, trackedPath, "one\ntwo\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, trackedPath, "one\nCHANGED\n")
	writeRepoFile(t, filepath.Join(repo, "new.txt"), "hello\n")

	p := openStatusPanelIn(t, repo)
	if p.table.ItemCount != 2 {
		t.Fatalf("rows = %d, want 2", p.table.ItemCount)
	}
	p.table.SelectPos = 0 // sorted by path: new.txt before tracked.txt
	entry, ok := p.selectedEntry()
	if !ok || entry.XY != "??" || entry.Path != "new.txt" {
		t.Fatalf("first row = %+v, want the untracked new.txt", entry)
	}

	untrackedConfirm(t, p).OnResult(0)
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new.txt still exists after delete: err = %v", err)
	}
	if _, err := os.Stat(trackedPath); err != nil {
		t.Fatalf("the tracked file was touched: %v", err)
	}

	entry, ok = p.selectedEntry()
	if !ok || entry.Path != "tracked.txt" {
		t.Fatalf("selected entry after delete = %+v, want tracked.txt", entry)
	}
	before := vtui.FrameManager.GetTopFrame()
	if !p.ProcessKey(key(vtinput.VK_F8)) {
		t.Fatal("F8 was not claimed")
	}
	v, ok := vtui.FrameManager.GetTopFrame().(*HunkView)
	if !ok || vtui.FrameManager.GetTopFrame() == before {
		t.Fatalf("F8 on a tracked change opened %T, want a HunkView", vtui.FrameManager.GetTopFrame())
	}
	t.Cleanup(func() { vtui.FrameManager.RemoveFrame(v) })
	if v.patch.mode != modeDiscard {
		t.Errorf("mode = %v, want modeDiscard", v.patch.mode)
	}
}

// TestDeleteUntrackedScreenDump draws the confirmation F8 opens over an
// untracked file, in English and Russian; run it with -v to see the
// screens.
func TestDeleteUntrackedScreenDump(t *testing.T) {
	t.Cleanup(func() { i18n.InitLang("", "", "") })
	for _, lang := range []string{"en", "ru"} {
		i18n.InitLang(lang, "en", "")
		repo := realGitRepo(t)
		writeRepoFile(t, filepath.Join(repo, "new.txt"), "hello\n")
		p := openStatusPanelIn(t, repo)

		scr := vtui.NewSilentScreenBuf()
		scr.AllocBuf(80, 25)
		vtui.FrameManager.Init(scr)
		confirm := untrackedConfirm(t, p)

		confirm.Show(scr)
		var b strings.Builder
		scr.Dump(&b)
		text, _, _ := strings.Cut(b.String(), "--- CELL METADATA")
		t.Logf("%s:\n%s", lang, text)
		for _, want := range []string{"new.txt", strings.ReplaceAll(i18n.Msg("GitStatus.DeleteUntrackedButton"), "&", "")} {
			if !strings.Contains(text, want) {
				t.Errorf("%s screen lacks %q:\n%s", lang, want, text)
			}
		}
	}
}
