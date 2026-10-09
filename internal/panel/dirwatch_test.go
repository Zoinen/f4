package panel

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dirwatch"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var fastWatch = dirwatch.Options{Quiet: 10 * time.Millisecond, PollInterval: 20 * time.Millisecond}

func setWatchDirectories(t *testing.T, on bool) {
	t.Helper()
	old := config.App.WatchDirectories
	config.App.WatchDirectories = on
	t.Cleanup(func() { config.App.WatchDirectories = old })
}

func waitTrue(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestPanelDirWatchFollowsThePanel: the watch sits on the directory the panel
// shows, moves when the panel does, and is dropped for a VFS that is not the
// plain OS one and when the setting is off.
func TestPanelDirWatchFollowsThePanel(t *testing.T) {
	setWatchDirectories(t, true)
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir1, dir2 := t.TempDir(), t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(dir1))
	waitForLoad(t, fsp)

	var calls atomic.Int32
	pd := &panelDirWatch{opts: fastWatch}
	t.Cleanup(pd.stop)
	pd.sync(fsp, func() { calls.Add(1) })
	if got := pd.currentPath(); got != dir1 {
		t.Fatalf("watched path = %q, want %q", got, dir1)
	}
	pd.sync(fsp, func() { calls.Add(1) }) // same directory: nothing restarts
	if err := os.WriteFile(filepath.Join(dir1, "a"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitTrue(t, "a change in the first directory", func() bool { return calls.Load() >= 1 })

	fsp.Vfs = vfs.NewOSVFS(dir2)
	pd.sync(fsp, func() { calls.Add(1) })
	if got := pd.currentPath(); got != dir2 {
		t.Fatalf("watched path after moving = %q, want %q", got, dir2)
	}
	before := calls.Load()
	if err := os.WriteFile(filepath.Join(dir2, "b"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitTrue(t, "a change in the second directory", func() bool { return calls.Load() > before })

	fsp.Vfs = vfs.NewNullVFS(0)
	pd.sync(fsp, func() { calls.Add(1) })
	if got := pd.currentPath(); got != "" {
		t.Errorf("a non-OS VFS is watched at %q", got)
	}

	fsp.Vfs = vfs.NewOSVFS(dir1)
	config.App.WatchDirectories = false
	pd.sync(fsp, func() { calls.Add(1) })
	if got := pd.currentPath(); got != "" {
		t.Errorf("watching is off in the config, yet %q is watched", got)
	}
	if watchedPath(nil) != "" {
		t.Error("a nil panel has a watched path")
	}
}

// TestWatchedPanelShowsANewFile: a file created behind the panel's back
// reaches its listing through the watcher, the posted task and
// ReadDirectory, with the cursor kept where it was.
func TestWatchedPanelShowsANewFile(t *testing.T) {
	setWatchDirectories(t, true)
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fsp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(dir))
	waitForLoad(t, fsp)
	fsp.SelectName("keep.txt")

	pf := &PanelsFrame{}
	pf.Panels[0] = fsp
	pf.dirWatch[0].opts = fastWatch
	t.Cleanup(pf.dirWatch[0].stop)
	pf.syncDirWatches()
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	has := func() bool {
		for _, e := range fsp.AllEntries() {
			if e.Name == "new.txt" {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if has() && !fsp.IsLoading {
			break
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !has() {
		t.Fatal("the new file never reached the panel's listing")
	}
	if got := fsp.GetSelectedName(); got != "keep.txt" {
		t.Errorf("cursor moved to %q, want it to stay on keep.txt", got)
	}
}

// TestReloadWatchedIgnoresAStalePanel: a report for a panel that has since
// moved elsewhere, or a closed frame, reloads nothing.
func TestReloadWatchedIgnoresAStalePanel(t *testing.T) {
	setWatchDirectories(t, true)
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fsp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fsp)
	pf := &PanelsFrame{}
	pf.Panels[0] = fsp
	pf.dirWatch[0].path = "/somewhere/else"
	pf.reloadWatched(0)
	if fsp.IsLoading {
		t.Error("a report for another directory started a load")
	}
	pf.dirWatch[0].path = fsp.Vfs.GetPath()
	pf.Closed = true
	pf.reloadWatched(0)
	if fsp.IsLoading {
		t.Error("a closed frame started a load")
	}
	pf.syncDirWatches() // closed: starts nothing
	if pf.dirWatch[0].watcher != nil {
		t.Error("a closed frame started a watcher")
	}
}
