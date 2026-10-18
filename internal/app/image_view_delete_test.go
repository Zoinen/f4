package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// F8 in the picture viewer asks the panel's question and removes the file
// only after "Delete" (f4#1991).
func TestImageViewDeleteAsksThenRemoves(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	saved := config.App
	t.Cleanup(func() { config.App = saved })
	config.App.ConfirmDelete = true
	config.App.UseTrash = false
	config.App.DeleteCancelFocused = true

	dir := t.TempDir()
	target := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)

	question := func() *vtui.Window {
		t.Helper()
		dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if !ok {
			t.Fatalf("no question on top: %T", vtui.FrameManager.GetTopFrame())
		}
		return dlg
	}

	removed := false
	imageViewDelete(nil, v, target, func() { removed = true })
	dlg := question()
	children := dlg.GetChildren()
	if dlg.GetFocusedItem() != children[len(children)-1] {
		t.Error("with DeleteCancelFocused the question must start on Cancel")
	}
	dlg.SetExitCode(1)
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(target); err != nil || removed {
		t.Fatalf("Cancel deleted the file: %v, removed=%v", err, removed)
	}

	imageViewDelete(nil, v, target, func() { removed = true })
	question().SetExitCode(0)
	deadline := time.After(2 * time.Second)
	for !removed {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the viewer was not told about the delete")
		}
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("the file is still there: %v", err)
	}
}
