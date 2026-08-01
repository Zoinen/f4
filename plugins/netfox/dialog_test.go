package netfox

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type mockApp struct{}

func (m *mockApp) GetActivePanelVFS() vfs.VFS      { return nil }
func (m *mockApp) GetPassivePanelVFS() vfs.VFS     { return nil }
func (m *mockApp) GetSelectedNames() []string      { return nil }
func (m *mockApp) GetSelectedName() string         { return "" }
func (m *mockApp) RefreshAll()                     {}
func (m *mockApp) SetPendingSelection(name string) {}
func (m *mockApp) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
}
func (m *mockApp) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
}
func (m *mockApp) Message(title, msg string, buttons []string) int               { return 0 }
func (m *mockApp) InputBox(title, prompt, history string, callback func(string)) {}
func (m *mockApp) Menu(title string, items []string, callback func(int))         {}

func TestConnectionDialogLayout(t *testing.T) {
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	// Create a temporary VFS
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_netfox.json")
	nf := NewNetFoxVFS(dbPath)

	app := &mockApp{}

	// Show dialog pushes it to the FrameManager
	showConnectionDialog(app, nf, "")

	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(vtui.Container)
	if !ok {
		t.Fatalf("Top frame is not a vtui.Container")
	}

	// Validate the layout
	vtui.AssertLayout(t, dlg)
}
