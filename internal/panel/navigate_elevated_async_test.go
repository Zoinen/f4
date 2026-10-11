package panel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// navigateElevatedDirectoryAsync's plumbing — the background resolve, the
// RunOnUI hop, and the CommitPath + ReadDirectory it triggers — must behave
// exactly like the synchronous Enter-key path from the user's point of view
// (f4#1411). This does not need a real elevation route: it calls the method
// directly, the same way ProcessKey does once NeedsElevation (covered by
// vfs.TestOSVFSNeedsElevation) has already said yes.
func TestNavigateElevatedDirectoryAsyncAppliesResult(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fp)

	osfs, ok := fp.Vfs.(*vfs.OSVFS)
	if !ok {
		t.Fatalf("fp.Vfs is %T, want *vfs.OSVFS", fp.Vfs)
	}
	newPath := fp.Vfs.Join(root, "sub")

	fp.navigateElevatedDirectoryAsync(osfs, newPath, root, "sub")

	deadline := time.After(2 * time.Second)
	for fp.Vfs.GetPath() != newPath {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timeout waiting for async navigation; path is %q, want %q", fp.Vfs.GetPath(), newPath)
		}
	}
	waitForLoad(t, fp)
}

// The UI-freeze fix (f4#1411) moved the slow sudo resolve off the UI
// goroutine, but that alone left the wait silent: nothing on screen said an
// elevation attempt was in progress while a slow PAM prompt (e.g. a
// fingerprint reader) was up, reported live on the same ticket. This confirms
// navigateElevatedDirectoryAsync now drives the panel's existing loading
// indicator — the same one readDirectoryEx uses for a slow listing — for the
// span of the background resolve, and clears it once the result is applied.
func TestNavigateElevatedDirectoryAsyncShowsLoadingIndicator(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fp)

	osfs, ok := fp.Vfs.(*vfs.OSVFS)
	if !ok {
		t.Fatalf("fp.Vfs is %T, want *vfs.OSVFS", fp.Vfs)
	}
	newPath := fp.Vfs.Join(root, "sub")

	if fp.IsLoading {
		t.Fatalf("fp.IsLoading = true before the async navigation started")
	}

	fp.navigateElevatedDirectoryAsync(osfs, newPath, root, "sub")

	// The indicator is armed synchronously, before the (possibly slow)
	// background resolve even starts, so it is observable immediately —
	// no need to wait on or fake a slow dispatcher for this assertion.
	if !fp.IsLoading {
		t.Fatalf("fp.IsLoading = false right after starting the async navigation; want true so the panel shows an elevation-in-progress indicator")
	}

	deadline := time.After(2 * time.Second)
	for fp.Vfs.GetPath() != newPath {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("timeout waiting for async navigation; path is %q, want %q", fp.Vfs.GetPath(), newPath)
		}
	}
	waitForLoad(t, fp)

	if fp.IsLoading {
		t.Fatalf("fp.IsLoading = true after the navigation settled; the indicator should have been cleared")
	}
}

// If the panel navigates away while the background resolve is still in
// flight, applying its (now stale) result must be a no-op rather than
// clobbering wherever the panel actually is.
func TestNavigateElevatedDirectoryAsyncIgnoresStaleResult(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	for _, name := range []string{"sub", "other"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fp)

	osfs, ok := fp.Vfs.(*vfs.OSVFS)
	if !ok {
		t.Fatalf("fp.Vfs is %T, want *vfs.OSVFS", fp.Vfs)
	}
	stalePath := fp.Vfs.Join(root, "sub")
	fp.navigateElevatedDirectoryAsync(osfs, stalePath, root, "sub")

	// Move the panel elsewhere before the async result above is applied,
	// mirroring what ProcessKey does on the synchronous path.
	otherPath := fp.Vfs.Join(root, "other")
	if err := fp.SetKnownDirectoryPath(otherPath); err != nil {
		t.Fatalf("SetKnownDirectoryPath(%q): %v", otherPath, err)
	}
	fp.ReadDirectory()
	waitForLoad(t, fp)

	// Drain whatever the stale async navigation posts; it must not move the
	// panel back to stalePath.
	drainDeadline := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-drainDeadline:
			break drain
		}
	}

	if fp.Vfs.GetPath() != otherPath {
		t.Fatalf("panel path = %q, want it to have stayed at %q", fp.Vfs.GetPath(), otherPath)
	}
}
