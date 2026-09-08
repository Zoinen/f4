package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type refreshReadResult struct {
	items []vfs.VFSItem
	err   error
}

type controlledRefreshVFS struct {
	*vfs.NullVFS
	currentPath string
	reads       chan chan refreshReadResult
}

func (v *controlledRefreshVFS) GetPath() string           { return v.currentPath }
func (v *controlledRefreshVFS) SetPath(path string) error { v.currentPath = path; return nil }
func (v *controlledRefreshVFS) IsAtRoot() bool            { return true }
func (v *controlledRefreshVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	return vfs.VFSItem{Name: "/", IsDir: true}, nil
}
func (v *controlledRefreshVFS) ReadDir(ctx context.Context, _ string, chunk func([]vfs.VFSItem)) error {
	reply := make(chan refreshReadResult, 1)
	select {
	case v.reads <- reply:
	case <-ctx.Done():
		return ctx.Err()
	}
	// Deliberately deliver a late result even when the consumer canceled it.
	result := <-reply
	chunk(result.items)
	return result.err
}

func TestSameDirectoryRefreshRejectsFailedAndSupersededResults(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fs := &controlledRefreshVFS{NullVFS: vfs.NewNullVFS(0), currentPath: "/same", reads: make(chan chan refreshReadResult, 2)}
	nextRead := func() chan refreshReadResult {
		select {
		case reply := <-fs.reads:
			return reply
		case <-time.After(2 * time.Second):
			t.Fatal("directory read did not start")
			return nil
		}
	}
	result := func(name string, err error) refreshReadResult {
		return refreshReadResult{[]vfs.VFSItem{{Name: name, Revision: "v1"}}, err}
	}
	fp := NewFileSystemPanel(0, 0, 40, 20, fs)
	defer func() {
		if fp.cancelLoad != nil {
			fp.cancelLoad()
		}
	}()
	nextRead() <- result("keep.txt", nil)
	waitForLoad(t, fp)
	old := fp.entries[0]
	fp.ReadDirectory()
	nextRead() <- result("partial.txt", errors.New("read failed"))
	waitForLoad(t, fp)
	if len(fp.entries) != 1 || fp.entries[0] != old {
		t.Fatal("failed refresh replaced the last good listing")
	}
	fp.ReadDirectory()
	stale := nextRead()
	if err := fs.SetPath("/next"); err != nil {
		t.Fatal(err)
	}
	fp.ReadDirectory()
	stale <- result("stale.txt", nil)
	nextRead() <- result("current.txt", nil)
	waitForLoad(t, fp)
	if len(fp.entries) != 1 || fp.entries[0].Name != "current.txt" {
		t.Fatalf("superseded result replaced navigation: %+v", fp.entries)
	}
}

func TestDirectoryReconciliationRetainsUnchangedRows(t *testing.T) {
	a := &fileEntry{VFSItem: vfs.VFSItem{Name: "a", Size: 12, MTime: time.Unix(123, 0)}, Selected: true}
	b := &fileEntry{VFSItem: vfs.VFSItem{Name: "b", Size: 13, MTime: time.Unix(123, 0)}}
	fp := &FileSystemPanel{entries: []*fileEntry{a, b}, catalogRevision: 9}
	if fp.reconcileDirectoryEntries([]*fileEntry{{VFSItem: a.VFSItem}, {VFSItem: b.VFSItem}}) {
		t.Fatal("unchanged listing published a mutation")
	}
	if fp.entries[0] != a || !fp.entries[0].Selected {
		t.Fatal("lost entry/selection")
	}
	next := []*fileEntry{{VFSItem: vfs.VFSItem{Name: "new", Revision: "v1"}}, {VFSItem: b.VFSItem}}
	if !fp.reconcileDirectoryEntries(next) {
		t.Fatal("changed listing ignored")
	}
	if fp.entries[1] != b {
		t.Fatal("survivor recreated")
	}
	ranges := fp.catalogRefreshDelta["ranges"].([]extui.M)
	if len(ranges) != 1 || ranges[0]["oldIndex"] != 1 || ranges[0]["index"] != 1 || ranges[0]["count"] != 1 {
		t.Fatalf("bad retained ranges: %#v", ranges)
	}
}

func TestDirectoryReconciliationDetectsOverwriteAndIgnoresAccessTime(t *testing.T) {
	item := vfs.VFSItem{Name: "a.png", Size: 12, MTime: time.Unix(123, 0)}
	accessed := item
	accessed.ATime = time.Now()
	if !sameDirectoryItem(item, accessed) {
		t.Fatal("preview access invalidates listing")
	}
	changed := item
	changed.Size++
	if sameDirectoryItem(item, changed) {
		t.Fatal("size change hidden")
	}
	changed = item
	changed.Revision = "new"
	if sameDirectoryItem(item, changed) {
		t.Fatal("provider content revision hidden")
	}
}

func TestDirectoryReconciliationUnknownVersionsAndCalculatedDirectories(t *testing.T) {
	unknown := &fileEntry{VFSItem: vfs.VFSItem{Name: "unknown.png"}}
	fp := &FileSystemPanel{entries: []*fileEntry{unknown}}
	if !fp.reconcileDirectoryEntries([]*fileEntry{{VFSItem: unknown.VFSItem}}) {
		t.Fatal("unknown content version was reused across observations")
	}
	if ranges := fp.catalogRefreshDelta["ranges"].([]extui.M); len(ranges) != 0 {
		t.Fatal("unknown content was declared reusable")
	}
	dir := &fileEntry{VFSItem: vfs.VFSItem{Name: "dir", IsDir: true, Size: 1234, MTime: time.Unix(123, 0)}, SizeCalculated: true}
	fp.entries = []*fileEntry{dir}
	next := dir.VFSItem
	next.Size = 0
	if fp.reconcileDirectoryEntries([]*fileEntry{{VFSItem: next}}) || fp.entries[0] != dir {
		t.Fatal("unchanged directory lost its calculated size")
	}
}

func TestDirectoryReconciliationPreservesCursorAndClampsDeletedTail(t *testing.T) {
	item := func(name string) *fileEntry {
		return &fileEntry{VFSItem: vfs.VFSItem{Name: name, Revision: "v1"}}
	}
	fp := &FileSystemPanel{entries: []*fileEntry{item("a"), item("b"), item("c")}, cursorIdx: 1}
	fp.reconcileDirectoryEntries([]*fileEntry{item("b"), item("c")})
	if fp.cursorIdx != 0 {
		t.Fatal("cursor did not follow surviving identity")
	}
	fp.cursorIdx = 1
	fp.reconcileDirectoryEntries([]*fileEntry{item("b")})
	if fp.cursorIdx != 0 {
		t.Fatal("deleted tail did not fall back to previous row")
	}
	fp.reconcileDirectoryEntries(nil)
	if fp.cursorIdx != 0 {
		t.Fatal("empty directory has an invalid cursor")
	}
}

func TestSameDirectoryRefreshKeepsCatalogDuringReadAndNoOpRevision(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	fp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(dir))
	defer func() {
		if fp.cancelLoad != nil {
			fp.cancelLoad()
		}
	}()
	waitForLoad(t, fp)
	fp.updateSemanticRevisions()
	previous := append([]*fileEntry(nil), fp.entries...)
	revision := fp.catalogRevision
	fp.ReadDirectory()
	if len(fp.entries) != len(previous) || fp.catalogProvisional {
		t.Fatal("same-folder refresh exposed a placeholder")
	}
	for i := range previous {
		if fp.entries[i] != previous[i] {
			t.Fatal("refresh cleared displayed entries before completion")
		}
	}
	waitForLoad(t, fp)
	fp.updateSemanticRevisions()
	if fp.catalogRevision != revision {
		t.Fatalf("no-op refresh advanced catalog: %d -> %d", revision, fp.catalogRevision)
	}
	for i := range previous {
		if fp.entries[i] != previous[i] {
			t.Fatalf("no-op refresh recreated entry %d: old=%+v new=%+v", i, previous[i], fp.entries[i])
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	fp.ReadDirectory()
	waitForLoad(t, fp)
	found := false
	for _, entry := range fp.entries {
		if entry.Name == "new.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("refresh missed inserted file")
	}
}

func TestOperationRefreshTargetsAndDeduplicatesPanels(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir, unrelated := t.TempDir(), t.TempDir()
	child := filepath.Join(dir, "test")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	pf := &PanelsFrame{}
	left := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(dir))
	right := NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS(unrelated))
	pf.panels[0], pf.panels[1] = left, right
	defer func() { left.cancelLoad(); right.cancelLoad() }()
	waitForLoad(t, left)
	waitForLoad(t, right)
	l, r := left.loadGeneration, right.loadGeneration
	refreshOperationViews(
		panelOperationLocation{pf, vfs.NewOSVFS(child), child},
		panelOperationLocation{pf, vfs.NewOSVFS(dir), dir},
	)
	if left.loadGeneration != l+1 || right.loadGeneration != r {
		t.Fatalf("expected one affected read and no unrelated read: left=%d right=%d", left.loadGeneration-l, right.loadGeneration-r)
	}
	waitForLoad(t, left)
}
