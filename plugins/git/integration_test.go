package gitplugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestSnapshotStillAtDirectoryRejectsStalePanelLocation(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem := vfs.NewOSVFS(root)

	current := vfs.PanelSnapshot{VFS: filesystem, Path: root}
	if !snapshotStillAtDirectory(current, root) {
		t.Fatal("current local panel did not match its observed directory")
	}

	if err := filesystem.SetPath(child); err != nil {
		t.Fatal(err)
	}
	current.Path = child
	if snapshotStillAtDirectory(current, root) {
		t.Fatal("stale worker directory matched a panel that had entered a child directory")
	}
	if !snapshotStillAtDirectory(current, child) {
		t.Fatal("current child directory did not match its observation")
	}

	if snapshotStillAtDirectory(vfs.PanelSnapshot{}, child) {
		t.Fatal("non-local snapshot matched an observed directory")
	}
}

func TestAutomaticStatusRefreshSkipsFreshSnapshot(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	plugin := &Plugin{
		initialized: true,
		statuses: map[string]*repositoryStatus{
			root: {root: root, updatedAt: time.Now()},
		},
		statusTasks: make(map[string]*statusRefreshTask),
	}

	plugin.scheduleAutomaticStatusRefresh(Repository{Root: root}, "panel", func() {})
	if got := len(plugin.statusTasks); got != 0 {
		t.Fatalf("fresh status scheduled a background scan: %d tasks", got)
	}
}

func TestAutomaticStatusRefreshKeepsLatestObserverAndCancelsUnusedWork(t *testing.T) {
	root := filepath.Clean(t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	task := &statusRefreshTask{
		cancel: cancel,
		callbacks: map[string]func(){
			"first":  func() {},
			"second": func() {},
		},
	}
	plugin := &Plugin{
		initialized: true,
		statuses:    make(map[string]*repositoryStatus),
		statusTasks: map[string]*statusRefreshTask{root: task},
	}

	plugin.cancelAutomaticStatusObserver("first")
	if got := len(task.callbacks); got != 1 {
		t.Fatalf("callbacks after first observer left = %d, want 1", got)
	}
	select {
	case <-ctx.Done():
		t.Fatal("shared automatic scan was cancelled while another panel still needed it")
	default:
	}

	plugin.cancelAutomaticStatusObserver("second")
	if _, ok := plugin.statusTasks[root]; ok {
		t.Fatal("unused automatic scan remained registered")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("unused automatic scan was not cancelled")
	}
}
