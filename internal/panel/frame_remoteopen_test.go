package panel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type blockedOpenVFS struct {
	vfs.VFS
	started chan struct{}
	stopped chan struct{}
}

func (*blockedOpenVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	return vfs.VFSItem{Name: "clip.mp4", Size: 100, SizeKnown: true}, nil
}
func (v *blockedOpenVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return &blockedOpenFile{started: v.started, stopped: v.stopped}, nil
}

type blockedOpenFile struct {
	vfs.ReadAtCloser
	started, stopped chan struct{}
}

func (f *blockedOpenFile) Read(ctx context.Context, data []byte) (int, error) {
	close(f.started)
	<-ctx.Done()
	close(f.stopped)
	return 0, ctx.Err()
}
func (*blockedOpenFile) Close() error { return nil }

func TestRemoteOpenCtrlCCancelsDownload(t *testing.T) {
	if _, _, supported := AssociatedFileCommand("clip.mp4"); !supported {
		t.Skip("no desktop associations")
	}
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	fs := &blockedOpenVFS{VFS: vfs.NewNullVFS(0), started: make(chan struct{}), stopped: make(chan struct{})}
	launches := 0
	pf.ExternalUIRunner = func(string, []string, string) error { launches++; return nil }
	OpenRemoteAssociatedFile(pf, fs, "clip.mp4")
	deadline := time.After(2 * time.Second)
	var dialog vtui.Frame
	for dialog == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("download progress was not shown")
		}
		top := vtui.FrameManager.GetTopFrame()
		if top != nil && top.GetTitle() == " Opening remote file " {
			dialog = top
		}
	}
	select {
	case <-fs.started:
	case <-time.After(time.Second):
		t.Fatal("download did not start")
	}
	ctrlC := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_C, ControlKeyState: vtinput.LeftCtrlPressed}
	dialog.ProcessKey(ctrlC)
	select {
	case <-fs.stopped:
	case <-time.After(time.Second):
		// Always release the worker before reporting the failed regression.
		dialog.Close()
		<-fs.stopped
		t.Fatal("Ctrl+C did not cancel the remote video download")
	}
	if launches != 0 {
		t.Fatal("cancelled download opened a local player")
	}
}

func TestDownloadAssociatedFileCancellationRemovesPartialCopy(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)
	name := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(name, make([]byte, 256*1024), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, _, err := downloadAssociatedFile(ctx, vfs.NewOSVFS(filepath.Dir(name)), name, func(string, int) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("download error = %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial downloads remain: %v, %v", entries, err)
	}
}
