package fileops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// The synchronize planner, checked against the rules Total Commander's help
// states: which side a pair is copied to, what "ignore date" is allowed to
// conclude, and what the asymmetric mirror does with a file the master
// folder does not have.

var (
	syncTestEarly = time.Date(2024, 5, 4, 12, 0, 0, 0, time.UTC)
	syncTestLate  = time.Date(2024, 6, 4, 12, 0, 0, 0, time.UTC)
)

func syncWriteFile(t *testing.T, dir, name, content string, stamp time.Time) string {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return full
}

func syncTestSides(t *testing.T, left, right string) SyncSides {
	t.Helper()
	return SyncSides{
		LeftFS:    compareTestVFS(t, left),
		LeftRoot:  left,
		RightFS:   compareTestVFS(t, right),
		RightRoot: right,
	}
}

func syncTestOptions() config.SyncOptions {
	return config.DefaultSyncOptions()
}

// syncPairsByRel builds the plan and indexes it, because the assertions are
// about named files rather than about the order they come back in.
func syncPairsByRel(t *testing.T, sides SyncSides, opts config.SyncOptions, keep syncNameFilter) map[string]SyncPair {
	t.Helper()
	pairs, err := BuildSyncPairs(context.Background(), sides, opts, keep, nil, nil)
	if err != nil {
		t.Fatalf("build sync pairs: %v", err)
	}
	out := make(map[string]SyncPair, len(pairs))
	for _, p := range pairs {
		out[p.Rel] = p
	}
	return out
}

func syncAssert(t *testing.T, pairs map[string]SyncPair, rel string, state SyncState, action SyncAction) {
	t.Helper()
	p, ok := pairs[rel]
	if !ok {
		t.Fatalf("%q is missing from the plan", rel)
	}
	if p.State != state {
		t.Errorf("%q: state = %d, want %d", rel, p.State, state)
	}
	if p.Action != action {
		t.Errorf("%q: action = %d, want %d", rel, p.Action, action)
	}
}

func TestBuildSyncPairsDirections(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "only-left.txt", "l", syncTestEarly)
	syncWriteFile(t, right, "only-right.txt", "r", syncTestEarly)
	syncWriteFile(t, left, "newer-left.txt", "new", syncTestLate)
	syncWriteFile(t, right, "newer-left.txt", "old", syncTestEarly)
	syncWriteFile(t, left, "newer-right.txt", "old", syncTestEarly)
	syncWriteFile(t, right, "newer-right.txt", "new", syncTestLate)
	syncWriteFile(t, left, "same.txt", "same", syncTestEarly)
	syncWriteFile(t, right, "same.txt", "same", syncTestEarly)
	syncWriteFile(t, left, "sub/deep.txt", "deep", syncTestLate)

	pairs := syncPairsByRel(t, syncTestSides(t, left, right), syncTestOptions(), nil)

	syncAssert(t, pairs, "only-left.txt", SyncLeftOnly, SyncCopyToRight)
	syncAssert(t, pairs, "only-right.txt", SyncRightOnly, SyncCopyToLeft)
	syncAssert(t, pairs, "newer-left.txt", SyncLeftNewer, SyncCopyToRight)
	syncAssert(t, pairs, "newer-right.txt", SyncRightNewer, SyncCopyToLeft)
	syncAssert(t, pairs, "same.txt", SyncEqual, SyncSkip)
	syncAssert(t, pairs, "sub/deep.txt", SyncLeftOnly, SyncCopyToRight)

	if _, ok := pairs["sub"]; ok {
		t.Error("a folder became a row of its own; only files are rows")
	}
}

// A pair stamped at the same second with different sizes is the "!=" Total
// Commander shows: something changed, and nothing says which copy to keep.
func TestBuildSyncPairsSameTimeDifferentSizeHasNoDirection(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "f.txt", "short", syncTestEarly)
	syncWriteFile(t, right, "f.txt", "much longer content", syncTestEarly)

	pairs := syncPairsByRel(t, syncTestSides(t, left, right), syncTestOptions(), nil)
	syncAssert(t, pairs, "f.txt", SyncDiffers, SyncSkip)
}

// "ignore date": name and size are the whole truth, so the answer can only
// be equal or not equal.
func TestBuildSyncPairsIgnoreDate(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "same-size.txt", "abcd", syncTestLate)
	syncWriteFile(t, right, "same-size.txt", "wxyz", syncTestEarly)
	syncWriteFile(t, left, "other-size.txt", "abcd", syncTestEarly)
	syncWriteFile(t, right, "other-size.txt", "abcdefgh", syncTestEarly)

	opts := syncTestOptions()
	opts.IgnoreDate = true
	pairs := syncPairsByRel(t, syncTestSides(t, left, right), opts, nil)

	syncAssert(t, pairs, "same-size.txt", SyncEqual, SyncSkip)
	syncAssert(t, pairs, "other-size.txt", SyncDiffers, SyncSkip)
}

// Ignoring dates while reading contents is the combination the help calls
// out: same size is no longer enough to pass as equal.
func TestBuildSyncPairsIgnoreDateByContent(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "f.txt", "abcd", syncTestLate)
	syncWriteFile(t, right, "f.txt", "wxyz", syncTestEarly)

	opts := syncTestOptions()
	opts.IgnoreDate, opts.ByContent = true, true
	pairs := syncPairsByRel(t, syncTestSides(t, left, right), opts, nil)

	syncAssert(t, pairs, "f.txt", SyncDiffers, SyncSkip)
}

// Asymmetric makes the right folder a mirror of the left one: everything
// different goes right, and what only the right side has is deleted there.
func TestBuildSyncPairsAsymmetricMirrorsToTheRight(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "only-left.txt", "l", syncTestEarly)
	syncWriteFile(t, right, "only-right.txt", "r", syncTestEarly)
	syncWriteFile(t, left, "newer-right.txt", "old", syncTestEarly)
	syncWriteFile(t, right, "newer-right.txt", "new", syncTestLate)
	syncWriteFile(t, left, "same.txt", "same", syncTestEarly)
	syncWriteFile(t, right, "same.txt", "same", syncTestEarly)

	opts := syncTestOptions()
	opts.Asymmetric = true
	pairs := syncPairsByRel(t, syncTestSides(t, left, right), opts, nil)

	syncAssert(t, pairs, "only-left.txt", SyncLeftOnly, SyncCopyToRight)
	syncAssert(t, pairs, "only-right.txt", SyncRightOnly, SyncDeleteRight)
	syncAssert(t, pairs, "newer-right.txt", SyncRightNewer, SyncCopyToRight)
	syncAssert(t, pairs, "same.txt", SyncEqual, SyncSkip)
}

func TestBuildSyncPairsWithoutSubdirsStaysOnTop(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "top.txt", "t", syncTestEarly)
	syncWriteFile(t, left, "sub/deep.txt", "d", syncTestEarly)

	opts := syncTestOptions()
	opts.Subdirs = false
	pairs := syncPairsByRel(t, syncTestSides(t, left, right), opts, nil)

	if _, ok := pairs["top.txt"]; !ok {
		t.Error("the folder's own file is missing from the plan")
	}
	if _, ok := pairs["sub/deep.txt"]; ok {
		t.Error("a file below a subfolder was compared without the subfolder option")
	}
}

func TestBuildSyncPairsHonoursTheNameFilter(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "keep.txt", "k", syncTestEarly)
	syncWriteFile(t, left, "drop.bin", "d", syncTestEarly)

	keep := func(name string) bool { return filepath.Ext(name) == ".txt" }
	pairs := syncPairsByRel(t, syncTestSides(t, left, right), syncTestOptions(), keep)

	if _, ok := pairs["keep.txt"]; !ok {
		t.Error("a file the filter accepts is missing from the plan")
	}
	if _, ok := pairs["drop.bin"]; ok {
		t.Error("a file the filter rejects reached the plan")
	}
}

// A file facing a folder of the same name is reported and locked: copying
// either way would have to remove the other side's tree first.
func TestBuildSyncPairsLocksFileAgainstFolder(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "clash", "file", syncTestEarly)
	syncWriteFile(t, right, "clash/inside.txt", "dir", syncTestEarly)

	pairs := syncPairsByRel(t, syncTestSides(t, left, right), syncTestOptions(), nil)
	p, ok := pairs["clash"]
	if !ok {
		t.Fatal("the clashing pair is missing from the plan")
	}
	if !p.Locked {
		t.Error("a file facing a folder is not locked")
	}
	if p.Action != SyncSkip {
		t.Errorf("a locked pair got action %d", p.Action)
	}
	if got := p.AllowedActions(); len(got) != 1 || got[0] != SyncSkip {
		t.Errorf("a locked pair offers %v", got)
	}
}

func TestSyncPlanTotalsCountsEachKindOfAction(t *testing.T) {
	pairs := []SyncPair{
		{Rel: "a", HasLeft: true, Left: vfs.VFSItem{Size: 10}, Action: SyncCopyToRight},
		{Rel: "b", HasLeft: true, Left: vfs.VFSItem{Size: 5}, Action: SyncCopyToRight},
		{Rel: "c", HasRight: true, Right: vfs.VFSItem{Size: 7}, Action: SyncCopyToLeft},
		{Rel: "d", HasRight: true, Action: SyncDeleteRight},
		{Rel: "e", HasLeft: true, Action: SyncDeleteLeft},
		{Rel: "f", Action: SyncSkip},
	}
	got := SyncPlanTotals(pairs)
	want := SyncTotals{ToRight: 2, ToLeft: 1, DeleteRight: 1, DeleteLeft: 1, BytesToRight: 15, BytesToLeft: 7}
	if got != want {
		t.Errorf("totals = %+v, want %+v", got, want)
	}
	if got.Empty() {
		t.Error("a plan with actions reports itself empty")
	}
	if !SyncPlanTotals(pairs[len(pairs)-1:]).Empty() {
		t.Error("a plan of nothing but skips does not report itself empty")
	}
}

func TestSyncPairAllowedActions(t *testing.T) {
	leftOnly := SyncPair{HasLeft: true}
	if got := leftOnly.AllowedActions(); len(got) != 3 {
		t.Errorf("a left-only pair offers %v", got)
	}
	both := SyncPair{HasLeft: true, HasRight: true}
	if got := both.AllowedActions(); len(got) != 5 {
		t.Errorf("a two-sided pair offers %v", got)
	}
}

// The executor, at the level a plan reaches it: a copy into a folder that
// does not exist yet, an overwrite, and a deletion.
func TestRunSyncPairCopiesAndDeletes(t *testing.T) {
	left, right := t.TempDir(), t.TempDir()
	syncWriteFile(t, left, "sub/new.txt", "fresh", syncTestLate)
	syncWriteFile(t, left, "over.txt", "new content", syncTestLate)
	syncWriteFile(t, right, "over.txt", "old", syncTestEarly)
	syncWriteFile(t, right, "gone.txt", "bye", syncTestEarly)

	sides := syncTestSides(t, left, right)
	pairs := syncPairsByRel(t, sides, syncTestOptions(), nil)
	// The deletion is not a default of a symmetric run; the window is
	// where a user sets one, so the test sets it the same way.
	gone := pairs["gone.txt"]
	gone.Action = SyncDeleteRight

	state := &FileOpState{Buffer: make([]byte, 32*1024), OverwriteAll: true}
	for _, p := range []SyncPair{pairs["sub/new.txt"], pairs["over.txt"], gone} {
		if err := runSyncPair(context.Background(), sides, p, vfs.DeletePermanently, state, nil); err != nil {
			t.Fatalf("run %q: %v", p.Rel, err)
		}
	}

	if got := syncReadFile(t, filepath.Join(right, "sub", "new.txt")); got != "fresh" {
		t.Errorf("copied file holds %q", got)
	}
	if got := syncReadFile(t, filepath.Join(right, "over.txt")); got != "new content" {
		t.Errorf("overwritten file holds %q", got)
	}
	if _, err := os.Stat(filepath.Join(right, "gone.txt")); !os.IsNotExist(err) {
		t.Errorf("the deleted file is still there: %v", err)
	}
}

func syncReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(data)
}

func TestSyncEnsureDirCreatesEveryMissingLevel(t *testing.T) {
	root := t.TempDir()
	v := compareTestVFS(t, root)
	if err := syncEnsureDir(context.Background(), v, root, "a/b/c"); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}
	st, err := os.Stat(filepath.Join(root, "a", "b", "c"))
	if err != nil || !st.IsDir() {
		t.Fatalf("the folder chain was not created: %v", err)
	}
	// Running it again on an existing chain is not an error.
	if err := syncEnsureDir(context.Background(), v, root, "a/b/c"); err != nil {
		t.Fatalf("ensure existing dir: %v", err)
	}
}

func TestSyncEnsureDirRefusesToDigThroughAFile(t *testing.T) {
	root := t.TempDir()
	syncWriteFile(t, root, "a", "file", syncTestEarly)
	v := compareTestVFS(t, root)
	if err := syncEnsureDir(context.Background(), v, root, "a/b"); err == nil {
		t.Error("a file in the middle of the path was accepted as a folder")
	}
}

func TestSyncRelDirAndBaseName(t *testing.T) {
	cases := []struct{ rel, dir, base string }{
		{"file.txt", "", "file.txt"},
		{"a/file.txt", "a", "file.txt"},
		{"a/b/file.txt", "a/b", "file.txt"},
	}
	for _, c := range cases {
		if got := syncRelDir(c.rel); got != c.dir {
			t.Errorf("syncRelDir(%q) = %q, want %q", c.rel, got, c.dir)
		}
		if got := syncBaseName(c.rel); got != c.base {
			t.Errorf("syncBaseName(%q) = %q, want %q", c.rel, got, c.base)
		}
	}
}

func TestDefaultSyncActionFollowsTheMode(t *testing.T) {
	cases := []struct {
		state      SyncState
		asymmetric bool
		want       SyncAction
	}{
		{SyncEqual, false, SyncSkip},
		{SyncEqual, true, SyncSkip},
		{SyncDiffers, false, SyncSkip},
		{SyncDiffers, true, SyncCopyToRight},
		{SyncLeftOnly, false, SyncCopyToRight},
		{SyncRightOnly, false, SyncCopyToLeft},
		{SyncRightOnly, true, SyncDeleteRight},
		{SyncRightNewer, false, SyncCopyToLeft},
		{SyncRightNewer, true, SyncCopyToRight},
	}
	for _, c := range cases {
		if got := DefaultSyncAction(c.state, c.asymmetric); got != c.want {
			t.Errorf("DefaultSyncAction(%d, %v) = %d, want %d", c.state, c.asymmetric, got, c.want)
		}
	}
}
