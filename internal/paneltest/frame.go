package paneltest

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// WaitForLoad blocks until fp has finished reading its directory, running the
// UI tasks the read posts as it goes: the worker cannot finish without them,
// and no frame manager is pumping them in a test.
func WaitForLoad(t *testing.T, fp *panel.FileSystemPanel) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for fp.IsLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for panel to load")
		}
	}
	// Drain any final UI tasks after IsLoading becomes false
drain:
	for i := 0; i < 5; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			break drain
		}
	}

	// IsLoading is cleared by the final UI task just before the directory
	// worker returns to its queue loop. Enqueue a sentinel and join the worker
	// so it cannot keep reading globals used by the next test.
	fp.EnqueueDirectoryLoad(func() {})
	done := make(chan struct{})
	go func() {
		fp.LoadWorkerWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for directory worker to stop")
	}
}

// WaitForDirectoryLoads blocks until no directory-load worker is running
// anywhere in the process.
//
// The workers read vtui.FrameManager and config.App while they run, so a test
// that replaces either one has to know they are all finished first. Panels are
// created deep inside panel.PanelsFrame.ResizeConsole as well as directly, so
// the caller usually has no panel to wait on and this asks globally instead.
func WaitForDirectoryLoads(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		panel.DirectoryLoadWorkers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for the directory-load workers to stop")
	}
}

// SwapFrameManager is testutil.SwapFrameManager carrying the two background
// workers a panels frame leaves running. Both read the global frame manager, so
// both have to be joined before it is replaced — which is the whole reason the
// shared helper takes its drains from the caller.
func SwapFrameManager(t *testing.T) func() {
	t.Helper()
	return testutil.SwapFrameManager(t, func(*testing.T) { terminal.WaitForAsyncClipboard() }, WaitForDirectoryLoads)
}

// SetupMockPanelsFrame builds a panels frame wired to a MockPty, so a test can
// exercise the frame without a real child process. The panels read the working
// directory through a plain OSVFS, because tests create real files under
// t.TempDir().
func SetupMockPanelsFrame(t *testing.T) *panel.PanelsFrame {
	t.Helper()
	if vtui.FrameManager.TaskChan == nil {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	}
	pf := &panel.PanelsFrame{ActiveIdx: 1, ShowPanels: true, ShowKeyBar: true, ShowLeftPanel: true, ShowRightPanel: true}
	pf.Pty = &MockPty{}
	pf.TermView = terminal.NewTerminalView(80, 24)
	// The menu bar needs enough items for UpdateMenuCheckmarks, which reaches
	// index 0 and index 4.
	pf.MenuBar = vtui.NewMenuBar(nil)
	pf.MenuBar.Items = make([]vtui.MenuBarItem, 5)
	for i := 0; i < 5; i++ {
		pf.MenuBar.Items[i].SubItems = make([]vtui.MenuItem, 8)
	}
	pf.CmdLine = cmdline.NewCommandLine(">")
	pf.KeyBar = vtui.NewKeyBar()
	pf.Panels[0] = panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS("."))
	pf.Panels[1] = panel.NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS("."))
	WaitForLoad(t, pf.Panels[0].(*panel.FileSystemPanel))
	WaitForLoad(t, pf.Panels[1].(*panel.FileSystemPanel))
	pf.InitPTY()
	return pf
}

// FindDriveMenu returns the top-most drive menu on the frame stack. Unrelated
// tasks — the update check, for one — can push frames above it.
func FindDriveMenu(t *testing.T) *vtui.VMenu {
	t.Helper()
	frames := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].Frames
	for i := len(frames) - 1; i >= 0; i-- {
		if m, ok := DriveMenuFromFrame(frames[i]); ok && m.GetTitle() == i18n.Msg("Drive.Title") {
			return m
		}
	}
	t.Fatalf("drive menu not on the frame stack: %#v", frames)
	return nil
}

// DriveMenuFromFrame unwraps the menu a drive-menu frame carries.
func DriveMenuFromFrame(frame vtui.Frame) (*vtui.VMenu, bool) {
	switch f := frame.(type) {
	case *vtui.VMenu:
		return f, true
	case *panel.DriveMenuFrame:
		return f.VMenu, true
	default:
		return nil, false
	}
}
