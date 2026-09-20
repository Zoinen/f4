package panel

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// makeUnlistableDirectory creates root/name, denies listing it and checks
// that the host now produces the state #814 describes: Stat succeeds and
// opening the directory for listing is refused. A host that does not (root
// on Unix, an ACL the host ignores, an elevation route) cannot exercise the
// refusal, so the test is skipped with what was observed.
func makeUnlistableDirectory(t *testing.T, root, name string) string {
	t.Helper()
	if vfs.GetSudoClient().IsAvailable() {
		t.Skip("an elevation route is configured; the directory would be listed through it")
	}
	dir := filepath.Join(root, name)
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	denyPanelDirectoryListing(t, dir)
	if st, err := hostfs.Stat(dir); err != nil || !st.IsDir() {
		t.Skipf("precondition not met: Stat(%q) = %v, %v; want a directory", dir, st, err)
	}
	if f, err := hostfs.Open(dir); err == nil {
		_ = f.Close()
		t.Skipf("precondition not met: the host lets %q be opened for listing", dir)
	} else if !errors.Is(err, os.ErrPermission) {
		t.Skipf("precondition not met: opening %q failed with %v, not a permission error", dir, err)
	}
	return dir
}

// TestEnterOnUnlistableDirectoryKeepsPanelInPlace: Enter on a folder the user
// may not list must not show the folder being entered and left (#814). The
// panel keeps its path and cursor, and no directory load starts.
func TestEnterOnUnlistableDirectoryKeepsPanelInPlace(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	makeUnlistableDirectory(t, root, "denied")

	fp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fp)
	fp.SelectName("denied")
	if got := fp.GetSelectedName(); got != "denied" {
		t.Fatalf("cursor is on %q, want the denied folder", got)
	}

	fp.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if got := fp.Vfs.GetPath(); got != root {
		t.Fatalf("panel path after Enter = %q, want it to stay %q", got, root)
	}
	if fp.IsLoading {
		t.Error("Enter on the denied folder started a directory load")
	}
	if got := fp.GetSelectedName(); got != "denied" {
		t.Errorf("cursor moved to %q, want it to stay on the denied folder", got)
	}
}

// TestNavigateToPathRefusesUnlistableDirectoryInPlace covers the routes that
// go through NavigateToPath (cd, bookmarks, Ctrl+PgUp to a parent): the panel
// stays, the refusal is reported, and the change counts as handled so a typed
// cd is not passed on to the shell.
func TestNavigateToPathRefusesUnlistableDirectoryInPlace(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	denied := makeUnlistableDirectory(t, root, "denied")

	fsp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fsp)
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}

	if !pf.NavigateToPath(fsp, denied) {
		t.Fatal("NavigateToPath reported the refused change as not handled")
	}
	if got := fsp.Vfs.GetPath(); got != root {
		t.Fatalf("panel path = %q, want it to stay %q", got, root)
	}
	if fsp.IsLoading {
		t.Error("NavigateToPath started a directory load for the denied folder")
	}
	if fsp.directoryErrorDialog == nil {
		t.Error("the refusal was not reported")
	}
}

// TestFolderHistorySkipsUnlistableDirectory: the history walk tries entries
// until one can be opened, so a folder the user may not list is passed over
// silently, like any other unavailable entry.
func TestFolderHistorySkipsUnlistableDirectory(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	denied := makeUnlistableDirectory(t, root, "denied")
	readable := filepath.Join(root, "readable")
	if err := os.Mkdir(readable, 0o700); err != nil {
		t.Fatal(err)
	}

	fsp := NewFileSystemPanel(0, 0, 80, 24, vfs.NewOSVFS(root))
	waitForLoad(t, fsp)
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0, FolderHistoryPos: [2]int{-1, -1}}

	if !pf.NavigateAvailableFolderHistory(fsp, []string{denied, readable}, 0, 1) {
		t.Fatal("history walk found no entry to open")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != readable {
		t.Fatalf("history walk landed on %q, want %q", got, readable)
	}
	if fsp.directoryErrorDialog != nil {
		t.Error("the skipped entry was reported")
	}
}
