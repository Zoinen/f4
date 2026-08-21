package gitplugin

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/unxed/f4/vfs"
)

func TestLogVFSRootIsBoundedAndTraversalModesStayLocal(t *testing.T) {
	root, repository, worktree := newLogVFSTestRepository(t)
	initial := commitLogVFSFiles(t, worktree, root, map[string]*string{
		"base.txt": pointerTo("base\n"),
	}, "initial")
	head, err := repository.Head()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := head.Name()

	if err := worktree.Checkout(&gogit.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("side"),
		Create: true,
		Hash:   initial,
		Force:  true,
	}); err != nil {
		t.Fatalf("checkout side: %v", err)
	}
	side := commitLogVFSFiles(t, worktree, root, map[string]*string{
		"side.txt": pointerTo("side\n"),
	}, "side work")
	if err := worktree.Checkout(&gogit.CheckoutOptions{Branch: mainBranch, Force: true}); err != nil {
		t.Fatalf("checkout main: %v", err)
	}
	main := commitLogVFSFiles(t, worktree, root, map[string]*string{
		"main.txt": pointerTo("main\n"),
	}, "main work")

	boundedView := NewLogVFS(Repository{Root: root}, LogVFSOptions{CommitLimit: 2})
	rootRows := readLogVFSItems(t, boundedView, "/")
	if got, want := len(rootRows), 2; got != want {
		t.Fatalf("lazy root page has %d rows, want %d", got, want)
	}
	for _, row := range rootRows {
		if !row.IsDir || row.Name == "" || row.ExtendedAttributes["git.commit"] != row.Name {
			t.Errorf("invalid commit row: %#v", row)
		}
	}
	if !containsLogVFSItem(rootRows, main.String()) {
		t.Errorf("HEAD DAG root lacks current main commit %s", main)
	}
	if containsLogVFSItem(rootRows, side.String()) {
		t.Errorf("HEAD DAG unexpectedly contains unmerged side commit %s", side)
	}

	view := NewLogVFS(Repository{Root: root})
	if err := view.SetLogMode(LogTraversalAllLocalRefs); err != nil {
		t.Fatal(err)
	}
	allRows := readLogVFSItems(t, view, "/")
	if !containsLogVFSItem(allRows, side.String()) {
		t.Errorf("all-local-refs listing lacks side branch commit %s", side)
	}
	if !containsLogVFSItem(allRows, main.String()) {
		t.Errorf("all-local-refs listing lacks main branch commit %s", main)
	}

	if err := view.SetLogMode(LogTraversalFirstParent); err != nil {
		t.Fatal(err)
	}
	firstParentRows := readLogVFSItems(t, view, "/")
	if !containsLogVFSItem(firstParentRows, main.String()) || !containsLogVFSItem(firstParentRows, initial.String()) {
		t.Errorf("first-parent listing = %v, want main and initial", logVFSItemNames(firstParentRows))
	}
	if containsLogVFSItem(firstParentRows, side.String()) {
		t.Errorf("first-parent listing unexpectedly contains side commit %s", side)
	}

	gotTraversal, gotTree := view.ToggleModeForPath("/")
	if gotTraversal != LogTraversalHeadDAG || gotTree != CommitTreeChangedFiles {
		t.Errorf("root F2 toggle = (%v, %v), want (%v, %v)", gotTraversal, gotTree, LogTraversalHeadDAG, CommitTreeChangedFiles)
	}

	clamped := NewLogVFS(Repository{Root: root}, LogVFSOptions{CommitLimit: GitLogPageSize + 10})
	if got, want := clamped.session.limit, GitLogPageSize; got != want {
		t.Errorf("commit limit = %d, want hard cap %d", got, want)
	}
}

func TestLogVFSCommitTreeModesOpenAfterBlobAndSessionOverlay(t *testing.T) {
	root, _, worktree := newLogVFSTestRepository(t)
	commitLogVFSFiles(t, worktree, root, map[string]*string{
		"keep.txt":       pointerTo("before\n"),
		"removed.txt":    pointerTo("gone\n"),
		"dir/nested.txt": pointerTo("nested\n"),
	}, "initial")
	commit := commitLogVFSFiles(t, worktree, root, map[string]*string{
		"keep.txt":    pointerTo("after\n"),
		"new.txt":     pointerTo("new\n"),
		"removed.txt": nil,
	}, "change files")

	commitPath := "/" + commit.String()
	keepPath := path.Join(commitPath, "keep.txt")
	removedPath := path.Join(commitPath, "removed.txt")
	view := NewLogVFS(Repository{Root: root})
	if err := view.SetPath(commitPath); err != nil {
		t.Fatal(err)
	}

	changedRows := readLogVFSItems(t, view, commitPath)
	for _, expected := range []string{"keep.txt", "new.txt", "removed.txt"} {
		if !containsLogVFSItem(changedRows, expected) {
			t.Errorf("changed-files tree lacks %q: %v", expected, logVFSItemNames(changedRows))
		}
	}
	removed, ok := findLogVFSItem(changedRows, "removed.txt")
	if !ok || removed.ExtendedAttributes["git.readOnly"] != "deleted" {
		t.Errorf("deleted item = %#v, want deleted read-only marker", removed)
	}
	if _, err := view.Open(context.Background(), removedPath); !errors.Is(err, os.ErrPermission) {
		t.Errorf("Open deleted path error = %v, want permission error", err)
	}
	if got := readLogVFSFile(t, view, keepPath); got != "after\n" {
		t.Errorf("after blob = %q, want after", got)
	}

	if err := view.SetOverlay(context.Background(), keepPath, []byte("overlay\n")); err != nil {
		t.Fatalf("set overlay: %v", err)
	}
	if got := readLogVFSFile(t, view, keepPath); got != "overlay\n" {
		t.Errorf("Open with overlay = %q, want overlay", got)
	}
	overlay, ok := view.Overlay(keepPath)
	if !ok || string(overlay) != "overlay\n" {
		t.Fatalf("Overlay = %q, %t", overlay, ok)
	}
	overlay[0] = 'X'
	if got := readLogVFSFile(t, view, keepPath); got != "overlay\n" {
		t.Errorf("caller mutation leaked into overlay: %q", got)
	}
	clone, ok := view.Clone().(*LogVFS)
	if !ok {
		t.Fatal("Clone did not preserve LogVFS type")
	}
	if got := readLogVFSFile(t, clone, keepPath); got != "overlay\n" {
		t.Errorf("clone did not share session overlay: %q", got)
	}
	if err := view.ClearOverlay(context.Background(), keepPath); err != nil {
		t.Fatalf("clear overlay: %v", err)
	}
	if got := readLogVFSFile(t, view, keepPath); got != "after\n" {
		t.Errorf("after clearing overlay = %q, want after", got)
	}
	if err := view.SetOverlay(context.Background(), removedPath, []byte("nope")); !errors.Is(err, os.ErrPermission) {
		t.Errorf("SetOverlay deleted error = %v, want permission error", err)
	}

	if got, want := view.ToggleCommitTreeMode(), CommitTreeFullSnapshot; got != want {
		t.Errorf("commit-tree toggle = %v, want %v", got, want)
	}
	snapshotRows := readLogVFSItems(t, view, commitPath)
	if containsLogVFSItem(snapshotRows, "removed.txt") {
		t.Errorf("full snapshot contains deleted path: %v", logVFSItemNames(snapshotRows))
	}
	if !containsLogVFSItem(snapshotRows, "dir") || !containsLogVFSItem(snapshotRows, "keep.txt") || !containsLogVFSItem(snapshotRows, "new.txt") {
		t.Errorf("full snapshot rows = %v, want dir, keep, new", logVFSItemNames(snapshotRows))
	}
	nestedPath := path.Join(commitPath, "dir")
	nestedRows := readLogVFSItems(t, view, nestedPath)
	if !containsLogVFSItem(nestedRows, "nested.txt") {
		t.Errorf("snapshot nested tree = %v", logVFSItemNames(nestedRows))
	}
	if got := readLogVFSFile(t, view, path.Join(nestedPath, "nested.txt")); got != "nested\n" {
		t.Errorf("snapshot nested blob = %q", got)
	}
}

func TestLogVFSCancellationAndReadOnlyMutations(t *testing.T) {
	root, _, worktree := newLogVFSTestRepository(t)
	commit := commitLogVFSFiles(t, worktree, root, map[string]*string{"file.txt": pointerTo("content\n")}, "initial")
	view := NewLogVFS(Repository{Root: root})
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := view.ReadDir(canceled, "/", func([]vfs.VFSItem) {}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ReadDir error = %v, want context.Canceled", err)
	}
	if _, err := view.Open(canceled, path.Join("/", commit.String(), "file.txt")); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled Open error = %v, want context.Canceled", err)
	}
	if err := view.MkDir(context.Background(), "/anything"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("MkDir error = %v, want permission", err)
	}
	if err := view.Remove(context.Background(), "/anything"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("Remove error = %v, want permission", err)
	}
	if err := view.Rename(context.Background(), "/from", "/to"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("Rename error = %v, want permission", err)
	}
	if _, err := view.Create(context.Background(), "/anything"); !errors.Is(err, os.ErrPermission) {
		t.Errorf("Create error = %v, want permission", err)
	}
	if err := view.SetAttributes(context.Background(), "/anything", vfs.VFSItem{}); !errors.Is(err, os.ErrPermission) {
		t.Errorf("SetAttributes error = %v, want permission", err)
	}
}

func newLogVFSTestRepository(t *testing.T) (string, *gogit.Repository, *gogit.Worktree) {
	t.Helper()
	root := t.TempDir()
	repository, err := gogit.PlainInit(root, false)
	if err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	return root, repository, worktree
}

func commitLogVFSFiles(t *testing.T, worktree *gogit.Worktree, root string, files map[string]*string, message string) plumbing.Hash {
	t.Helper()
	for name, content := range files {
		fullName := filepath.Join(root, filepath.FromSlash(name))
		if content == nil {
			if err := os.Remove(fullName); err != nil {
				t.Fatalf("remove %s: %v", name, err)
			}
			if _, err := worktree.Remove(name); err != nil {
				t.Fatalf("remove index %s: %v", name, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fullName), 0o755); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(fullName, []byte(*content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if _, err := worktree.Add(name); err != nil {
			t.Fatalf("add %s: %v", name, err)
		}
	}
	hash, err := worktree.Commit(message, &gogit.CommitOptions{Author: &object.Signature{
		Name:  "f4 log test",
		Email: "log@example.invalid",
		When:  time.Now().UTC(),
	}})
	if err != nil {
		t.Fatalf("commit %q: %v", message, err)
	}
	return hash
}

func readLogVFSItems(t *testing.T, view *LogVFS, directory string) []vfs.VFSItem {
	t.Helper()
	var items []vfs.VFSItem
	if err := view.ReadDir(context.Background(), directory, func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", directory, err)
	}
	return items
}

func readLogVFSFile(t *testing.T, view *LogVFS, file string) string {
	t.Helper()
	reader, err := view.Open(context.Background(), file)
	if err != nil {
		t.Fatalf("Open(%q): %v", file, err)
	}
	defer reader.Close()
	data := make([]byte, reader.Size())
	count, readErr := reader.ReadAt(context.Background(), data, 0)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		t.Fatalf("ReadAt(%q): %v", file, readErr)
	}
	return string(data[:count])
}

func containsLogVFSItem(items []vfs.VFSItem, name string) bool {
	_, ok := findLogVFSItem(items, name)
	return ok
}

func findLogVFSItem(items []vfs.VFSItem, name string) (vfs.VFSItem, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return vfs.VFSItem{}, false
}

func logVFSItemNames(items []vfs.VFSItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	return names
}

func pointerTo(value string) *string { return &value }
