package app

import (
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

// The wait internal/editor's own tests use, for the editor tests that stayed
// here. It is three lines and reads one exported flag, which is cheaper than
// exporting the harness.

func waitEditorSave(t *testing.T, ev *editor.EditorView) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for ev.IsSaving() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatal("timeout waiting for editor save")
		}
	}
}
