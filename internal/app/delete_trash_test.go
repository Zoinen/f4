package app

import (
	"context"
	"errors"
	"github.com/unxed/f4/internal/panel"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/theme"
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
	oldQueue := fileops.GlobalQueueManager
	queue := fileops.NewQueueManagerWithTasks()
	fileops.GlobalQueueManager = queue
	defer func() { fileops.GlobalQueueManager = oldQueue }()

	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0)}
	if err := probe.SetPath("/original"); err != nil {
		t.Fatal(err)
	}
	basePath := probe.GetPath()
	if err := probe.SetPath("/navigated"); err != nil {
		t.Fatal(err)
	}
	fileops.ExecuteDeleteOpWithDispositionAt(probe, basePath, []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	task := queue.Tasks()[0]
	if err := task.Run(context.Background(), &fileops.DummyReporter{}, nil); err != nil {
		t.Fatal(err)
	}
	task.SetState("Done")
	want := probe.Join(basePath, "item.txt")
	if len(probe.trashed) != 1 || probe.trashed[0] != want {
		t.Fatalf("trashed paths = %v, want %q", probe.trashed, want)
	}
}

func TestDeleteDoesNotRetryPartialRemoteMutation(t *testing.T) {
	oldQueue := fileops.GlobalQueueManager
	queue := fileops.NewQueueManagerWithTasks()
	fileops.GlobalQueueManager = queue
	defer func() { fileops.GlobalQueueManager = oldQueue }()

	partial := &vfs.PartialOperationError{Operation: "remote trash", Completed: []string{"child"}, Err: errors.New("later child failed")}
	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0), err: partial}
	fileops.ExecuteDeleteOpWithDispositionAt(probe, "/original", []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	task := queue.Tasks()[0]
	err := task.Run(context.Background(), &fileops.DummyReporter{}, nil)
	task.SetState("Done")
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
	if err := fileops.DeletePathWithDisposition(ctx, trashable, "item", vfs.DeleteToTrash); err != nil {
		t.Fatal(err)
	}
	if trashable.trashCalls != 1 || trashable.removeCalls != 0 {
		t.Fatalf("trash disposition called trash/remove %d/%d, want 1/0", trashable.trashCalls, trashable.removeCalls)
	}

	permanentOnly := &permanentOnlyDeleteProbe{}
	err := fileops.DeletePathWithDisposition(ctx, permanentOnly, "item", vfs.DeleteToTrash)
	if !errors.Is(err, vfs.ErrTrashUnsupported) {
		t.Fatalf("trash on incapable VFS returned %v, want ErrTrashUnsupported", err)
	}
	if permanentOnly.removeCalls != 0 {
		t.Fatal("trash failure silently fell back to permanent Remove")
	}

	if err := fileops.DeletePathWithDisposition(ctx, trashable, "item", vfs.DeletePermanently); err != nil {
		t.Fatal(err)
	}
	if trashable.removeCalls != 1 {
		t.Fatal("permanent disposition did not call Remove")
	}

	if err := fileops.DeletePathWithDisposition(ctx, trashable, "item", vfs.DeleteDisposition(255)); err == nil {
		t.Fatal("unknown disposition was accepted")
	}
	if trashable.removeCalls != 1 || trashable.trashCalls != 1 {
		t.Fatal("unknown disposition performed a destructive operation")
	}
}

func TestTrashDeleteStatsDoNotWalkTree(t *testing.T) {
	probe := &deleteDispositionProbe{}
	stats, err := fileops.CalculateDeleteStats(context.Background(), probe, "/", []string{"one", "two"}, vfs.DeleteToTrash, nil)
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
	oldQueue := fileops.GlobalQueueManager
	queue := fileops.NewQueueManagerWithTasks()
	fileops.GlobalQueueManager = queue
	defer func() { fileops.GlobalQueueManager = oldQueue }()

	probe := &queuedDeleteProbe{NullVFS: vfs.NewNullVFS(0)}
	if err := probe.SetPath("/upload"); err != nil {
		t.Fatal(err)
	}
	wantPath := probe.Join(probe.GetPath(), "item.txt")
	fileops.ExecuteDeleteOpWithDisposition(probe, []string{"item.txt"}, 0, vfs.DeleteToTrash, nil)
	if err := probe.SetPath("/"); err != nil {
		t.Fatal(err)
	}

	tasks := queue.Tasks()
	taskCount := len(tasks)
	if taskCount != 1 {
		t.Fatalf("queued tasks = %d, want 1", taskCount)
	}
	task := tasks[0]
	if err := task.Run(context.Background(), &fileops.DummyReporter{}, nil); err != nil {
		t.Fatal(err)
	}
	task.SetState("Done")
	if len(probe.trashed) != 1 || probe.trashed[0] != wantPath {
		t.Fatalf("trashed paths = %v, want original panel path %q", probe.trashed, wantPath)
	}
}

func TestDeleteActionsExposeDistinctDispositions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.ConfirmDelete = true

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ActiveIdx = 0
	fsp := pf.Panels[0].(*panel.FileSystemPanel)
	fsp.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "item.txt"}}}

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

	config.App.UseTrash = true
	ActionDelete(pf)
	findButton(t, "Move to Recycle Bin")

	config.App.UseTrash = false
	ActionDelete(pf)
	findButton(t, "Delete")

	actionDeletePermanent(pf)
	findButton(t, "Delete permanently")
}
