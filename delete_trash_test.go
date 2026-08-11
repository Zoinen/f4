package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type deleteDispositionProbe struct {
	vfs.VFS
	removeCalls int
	trashCalls  int
	readCalls   int
}

func (p *deleteDispositionProbe) Remove(context.Context, string) error {
	p.removeCalls++
	return nil
}

func (p *deleteDispositionProbe) MoveToTrash(context.Context, string) error {
	p.trashCalls++
	return nil
}

func (p *deleteDispositionProbe) ReadDir(context.Context, string, func([]vfs.VFSItem)) error {
	p.readCalls++
	return nil
}

type permanentOnlyDeleteProbe struct {
	vfs.VFS
	removeCalls int
}

func (p *permanentOnlyDeleteProbe) Remove(context.Context, string) error {
	p.removeCalls++
	return nil
}

type queuedDeleteProbe struct {
	*vfs.NullVFS
	trashed []string
	err     error
}

func (p *queuedDeleteProbe) MoveToTrash(_ context.Context, path string) error {
	p.trashed = append(p.trashed, path)
	return p.err
}

func TestQueuedTrashUsesActionBoundaryPathSnapshot(t *testing.T) {
	oldQueue := GlobalQueueManager
	queue := &OpQueueManager{activeKeys: make(map[string]bool)}
	GlobalQueueManager = queue
	defer func() { GlobalQueueManager = oldQueue }()

	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0)}
	if err := probe.SetPath("/original"); err != nil {
		t.Fatal(err)
	}
	basePath := probe.GetPath()
	if err := probe.SetPath("/navigated"); err != nil {
		t.Fatal(err)
	}
	ExecuteDeleteOpWithDispositionAt(nil, probe, basePath, []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	queue.mu.Lock()
	task := queue.tasks[0]
	queue.mu.Unlock()
	if err := task.Run(context.Background(), &DummyReporter{}, nil); err != nil {
		t.Fatal(err)
	}
	want := probe.Join(basePath, "item.txt")
	if len(probe.trashed) != 1 || probe.trashed[0] != want {
		t.Fatalf("trashed paths = %v, want %q", probe.trashed, want)
	}
}

func TestDeleteDoesNotRetryPartialRemoteMutation(t *testing.T) {
	oldQueue := GlobalQueueManager
	queue := &OpQueueManager{activeKeys: make(map[string]bool)}
	GlobalQueueManager = queue
	defer func() { GlobalQueueManager = oldQueue }()

	partial := &vfs.PartialOperationError{Operation: "remote trash", Completed: []string{"child"}, Err: errors.New("later child failed")}
	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0), err: partial}
	ExecuteDeleteOpWithDispositionAt(nil, probe, "/original", []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	queue.mu.Lock()
	task := queue.tasks[0]
	queue.mu.Unlock()
	err := task.Run(context.Background(), &DummyReporter{}, nil)
	if !errors.Is(err, vfs.ErrOperationPartial) {
		t.Fatalf("Run error = %v, want partial operation", err)
	}
	if len(probe.trashed) != 1 {
		t.Fatalf("partial operation was retried %d times", len(probe.trashed))
	}
}

func TestDeletePathDispositionDoesNotFallback(t *testing.T) {
	ctx := context.Background()
	trashable := &deleteDispositionProbe{}
	if err := deletePathWithDisposition(ctx, trashable, "item", vfs.DeleteToTrash); err != nil {
		t.Fatal(err)
	}
	if trashable.trashCalls != 1 || trashable.removeCalls != 0 {
		t.Fatalf("trash disposition called trash/remove %d/%d, want 1/0", trashable.trashCalls, trashable.removeCalls)
	}

	permanentOnly := &permanentOnlyDeleteProbe{}
	err := deletePathWithDisposition(ctx, permanentOnly, "item", vfs.DeleteToTrash)
	if !errors.Is(err, vfs.ErrTrashUnsupported) {
		t.Fatalf("trash on incapable VFS returned %v, want ErrTrashUnsupported", err)
	}
	if permanentOnly.removeCalls != 0 {
		t.Fatal("trash failure silently fell back to permanent Remove")
	}

	if err := deletePathWithDisposition(ctx, trashable, "item", vfs.DeletePermanently); err != nil {
		t.Fatal(err)
	}
	if trashable.removeCalls != 1 {
		t.Fatal("permanent disposition did not call Remove")
	}

	if err := deletePathWithDisposition(ctx, trashable, "item", vfs.DeleteDisposition(255)); err == nil {
		t.Fatal("unknown disposition was accepted")
	}
	if trashable.removeCalls != 1 || trashable.trashCalls != 1 {
		t.Fatal("unknown disposition performed a destructive operation")
	}
}

func TestTrashDeleteStatsDoNotWalkTree(t *testing.T) {
	probe := &deleteDispositionProbe{}
	stats, err := calculateDeleteStats(context.Background(), probe, "/", []string{"one", "two"}, vfs.DeleteToTrash, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 2 || stats.Dirs != 0 {
		t.Fatalf("trash stats = %+v, want two selected roots", stats)
	}
	if probe.readCalls != 0 {
		t.Fatalf("trash progress recursively enumerated the VFS %d time(s)", probe.readCalls)
	}
}

func TestQueuedTrashCapturesOriginalDirectory(t *testing.T) {
	oldQueue := GlobalQueueManager
	queue := &OpQueueManager{activeKeys: make(map[string]bool)}
	GlobalQueueManager = queue
	defer func() { GlobalQueueManager = oldQueue }()

	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0)}
	if err := probe.SetPath("/upload"); err != nil {
		t.Fatal(err)
	}
	wantPath := probe.Join(probe.GetPath(), "item.txt")
	ExecuteDeleteOpWithDisposition(nil, probe, []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	if err := probe.SetPath("/"); err != nil {
		t.Fatal(err)
	}

	queue.mu.Lock()
	taskCount := len(queue.tasks)
	if taskCount != 1 {
		queue.mu.Unlock()
		t.Fatalf("queued tasks = %d, want 1", taskCount)
	}
	task := queue.tasks[0]
	queue.mu.Unlock()
	if err := task.Run(context.Background(), &DummyReporter{}, nil); err != nil {
		t.Fatal(err)
	}
	task.mu.Lock()
	task.State = "Done"
	task.mu.Unlock()
	if len(probe.trashed) != 1 || probe.trashed[0] != wantPath {
		t.Fatalf("trashed paths = %v, want original panel path %q", probe.trashed, wantPath)
	}
}

func TestDeleteActionsExposeDistinctDispositions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.ConfirmDelete = true

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.activeIdx = 0
	fsp := pf.panels[0].(*FileSystemPanel)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "item.txt"}}}

	findButton := func(t *testing.T, want string) {
		t.Helper()
		dlg, ok := vtui.FrameManager.GetTopFrame().(vtui.Container)
		if !ok {
			t.Fatal("delete confirmation was not shown")
		}
		for _, child := range dlg.GetChildren() {
			if button, ok := child.(*vtui.Button); ok {
				label, _, _ := vtui.ParseAmpersandString(button.GetText())
				if strings.TrimSpace(strings.Trim(label, "[]")) == want {
					vtui.FrameManager.Pop()
					return
				}
			}
		}
		t.Fatalf("button %q not found", want)
	}

	AppConfig.UseTrash = true
	actionDelete(pf)
	findButton(t, "Move to Trash")

	AppConfig.UseTrash = false
	actionDelete(pf)
	findButton(t, "Delete")

	actionDeletePermanent(pf)
	findButton(t, "Delete permanently")
}
