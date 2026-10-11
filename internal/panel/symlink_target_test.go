package panel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// slowLinkVFS is a file system whose Readlink takes as long as the test says,
// the way a link on a far-away SFTP server does. Wrapping the interface hides
// the *vfs.OSVFS type, so the panel treats it as a remote file system.
type slowLinkVFS struct {
	vfs.VFS
	release chan struct{}
	calls   atomic.Int64
}

func (s *slowLinkVFS) Readlink(ctx context.Context, p string) (string, error) {
	s.calls.Add(1)
	select {
	case <-s.release:
		return "target.txt", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *slowLinkVFS) Symlink(ctx context.Context, target, linkPath string) error {
	return os.ErrPermission
}

func symlinkPanelRowText(scr *vtui.ScreenBuf, y, w int) string {
	var sb strings.Builder
	for x := 0; x < w; x++ {
		cell := scr.GetCell(x, y)
		if cell.Char != 0 {
			sb.WriteRune(vtui.CellBaseRune(cell.Char))
		}
	}
	return sb.String()
}

// newSymlinkTestPanel returns a panel of 80x24 over filesystem with the
// cursor on a symlink entry, the status line on or off as asked.
func newSymlinkTestPanel(t *testing.T, filesystem vfs.VFS, statusLine bool) (*FileSystemPanel, *vtui.ScreenBuf) {
	t.Helper()
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.ShowPanelFileInfo = statusLine

	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	fp := NewFileSystemPanel(0, 0, 80, 24, filesystem)
	waitForLoad(t, fp)
	fp.IsLoading = false
	if fp.LoadingTimer != nil {
		fp.LoadingTimer.Stop()
	}
	fp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: ".."}},
		{VFSItem: vfs.VFSItem{Name: "link", IsSymlink: true}},
		{VFSItem: vfs.VFSItem{Name: "plain.txt", Size: 5}},
	}
	fp.Refresh()
	fp.SetCursorIndex(1)
	return fp, scr
}

// showWithin draws the panel and fails the test if the draw call has not
// returned in time: a draw that waits for the network is the bug.
func showWithin(t *testing.T, fp *FileSystemPanel, scr *vtui.ScreenBuf, limit time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fp.Show(scr)
	}()
	select {
	case <-done:
	case <-time.After(limit):
		t.Fatal("drawing the panel blocked on a symlink lookup (f4#1766)")
	}
}

func drainUntilLinkResolved(t *testing.T, e *FileEntry) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for !e.linkResolved {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("the symlink target never arrived")
		}
	}
}

func TestSymlinkTargetDoesNotBlockStatusLineDraw(t *testing.T) {
	slow := &slowLinkVFS{VFS: vfs.NewOSVFS(t.TempDir()), release: make(chan struct{})}
	fp, scr := newSymlinkTestPanel(t, slow, true)

	showWithin(t, fp, scr, 2*time.Second)
	if row := symlinkPanelRowText(scr, 22, 80); strings.Contains(row, "→") {
		t.Fatalf("link target drawn before the lookup answered: %q", row)
	}
	// Repaints while the answer is on its way must not pile up more lookups.
	for i := 0; i < 5; i++ {
		showWithin(t, fp, scr, 2*time.Second)
	}
	deadline := time.Now().Add(2 * time.Second)
	for slow.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := slow.calls.Load(); got != 1 {
		t.Fatalf("Readlink called %d times while one answer was pending, want 1", got)
	}

	close(slow.release)
	drainUntilLinkResolved(t, fp.Entries[1])
	showWithin(t, fp, scr, 2*time.Second)
	if row := symlinkPanelRowText(scr, 22, 80); !strings.Contains(row, "→ target.txt") {
		t.Fatalf("status line lacks the link target after it arrived: %q", row)
	}
	showWithin(t, fp, scr, 2*time.Second)
	if got := slow.calls.Load(); got != 1 {
		t.Fatalf("an answered link was asked for again: %d calls", got)
	}
}

func TestSymlinkTargetDoesNotBlockBottomBorderDraw(t *testing.T) {
	slow := &slowLinkVFS{VFS: vfs.NewOSVFS(t.TempDir()), release: make(chan struct{})}
	fp, scr := newSymlinkTestPanel(t, slow, false)
	if fp.gridColumnCount() <= 1 {
		t.Fatalf("the default view has %d column(s), the bottom-border figure needs a grid", fp.gridColumnCount())
	}

	showWithin(t, fp, scr, 2*time.Second)
	close(slow.release)
	drainUntilLinkResolved(t, fp.Entries[1])
	showWithin(t, fp, scr, 2*time.Second)
	if row := symlinkPanelRowText(scr, 23, 80); !strings.Contains(row, "→ target.txt") {
		t.Fatalf("bottom border lacks the link target after it arrived: %q", row)
	}
}

func TestSymlinkTargetIsReadAtOnceOnLocalFileSystem(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("target.txt", filepath.Join(dir, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	fp, scr := newSymlinkTestPanel(t, vfs.NewOSVFS(dir), true)

	showWithin(t, fp, scr, 2*time.Second)
	if row := symlinkPanelRowText(scr, 22, 80); !strings.Contains(row, "→ target.txt") {
		t.Fatalf("local link target not drawn on the first repaint: %q", row)
	}
}
