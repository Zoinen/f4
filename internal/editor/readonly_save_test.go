package editor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func readOnlySaveEditor(t *testing.T, mode os.FileMode) (*EditorView, string) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	ev := NewEditorView(piecetable.New([]byte("abc")), v, path)
	t.Cleanup(ev.Close)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	ev.File = f
	ev.CursorPos = 0
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '!'})
	return ev, path
}

func waitSaved(t *testing.T, ev *EditorView) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for ev.Saving {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("the save did not finish")
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A read-only file is not rewritten without a question (f4#1981): the save
// renames a new file over it, which its permissions do not stop. Save writes
// it and keeps its permissions; the question is not asked again.
func TestEditorSave_ReadOnlyFileAsksFirst(t *testing.T) {
	ev, path := readOnlySaveEditor(t, 0o400)

	ev.SaveToFile(nil)
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || dlg.OnResult == nil {
		t.Fatalf("no question before saving a read-only file (top frame %T)", vtui.FrameManager.GetTopFrame())
	}
	if ev.Saving || readFile(t, path) != "abc" {
		t.Fatal("the read-only file was written before the user answered")
	}

	dlg.OnResult(1) // Cancel
	if ev.Saving || readFile(t, path) != "abc" || !ev.Modified {
		t.Fatal("Cancel still saved the read-only file")
	}
	if runtime.GOOS == "windows" {
		// What Save does over the read-only attribute is the save's own
		// business there; the question is what this test is about.
		return
	}

	ev.SaveToFile(nil)
	dlg = vtui.FrameManager.GetTopFrame().(*vtui.Window)
	dlg.OnResult(0) // Save
	waitSaved(t, ev)
	if got := readFile(t, path); got != "!abc" {
		t.Fatalf("Save wrote %q, want %q", got, "!abc")
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o222 != 0 {
		t.Fatalf("the file lost its permissions: %v %v", info.Mode(), err)
	}

	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '?'})
	top := vtui.FrameManager.GetTopFrame()
	ev.SaveToFile(nil)
	if vtui.FrameManager.GetTopFrame() != top {
		t.Fatal("the question came again after the user had answered Save")
	}
	waitSaved(t, ev)
}

func TestEditorSave_WritableFileSavesAtOnce(t *testing.T) {
	ev, path := readOnlySaveEditor(t, 0o600)
	top := vtui.FrameManager.GetTopFrame()
	ev.SaveToFile(nil)
	if vtui.FrameManager.GetTopFrame() != top {
		t.Fatal("a writable file got the read-only question")
	}
	waitSaved(t, ev)
	if got := readFile(t, path); got != "!abc" {
		t.Fatalf("saved %q, want %q", got, "!abc")
	}
}
