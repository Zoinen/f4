package gitplugin

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestLogVFSControllerF2TogglesModeAndRefreshesTargetVFS(t *testing.T) {
	root, _, worktree := newLogVFSTestRepository(t)
	commitLogVFSFiles(t, worktree, root, map[string]*string{"file.txt": pointerTo("content\n")}, "initial")
	view := NewLogVFS(Repository{Root: root})
	app := &logControllerTestApp{}

	if handled := view.ProcessPanelKey(app, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F2,
	}); !handled {
		t.Fatal("F2 was not handled")
	}
	if got, want := view.LogMode(), LogTraversalAllLocalRefs; got != want {
		t.Fatalf("F2 root traversal mode = %v, want %v", got, want)
	}
	if got, want := app.refreshed, 1; got != want || app.refreshedVFS != view {
		t.Fatalf("refresh = %d/%T, want 1/target LogVFS", got, app.refreshedVFS)
	}
	if view.ProcessPanelKey(app, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2, ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("modified F2 was unexpectedly handled")
	}
}

func TestLogVFSControllerF4CreatesOverlayAndReadOnlyActionsAreConsumed(t *testing.T) {
	root, _, worktree := newLogVFSTestRepository(t)
	commit := commitLogVFSFiles(t, worktree, root, map[string]*string{"file.txt": pointerTo("historical\n")}, "initial")
	view := NewLogVFS(Repository{Root: root})
	commitPath := "/" + commit.String()
	if err := view.SetPath(commitPath); err != nil {
		t.Fatal(err)
	}
	app := &logControllerTestApp{selected: []string{"file.txt"}}

	if handled := view.ProcessPanelKey(app, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F4,
	}); !handled {
		t.Fatal("F4 was not handled")
	}
	if got := len(app.editorRequests); got != 1 {
		t.Fatalf("editor requests = %d, want 1", got)
	}
	request := app.editorRequests[0]
	if !request.Temporary || string(request.Content) != "historical\n" || request.OnSave == nil {
		t.Fatalf("overlay request = %#v, want temporary historical editor with save callback", request)
	}
	if err := request.OnSave(context.Background(), []byte("overlay\n")); err != nil {
		t.Fatalf("save overlay: %v", err)
	}
	if got := readLogVFSFile(t, view, view.Join(commitPath, "file.txt")); got != "overlay\n" {
		t.Fatalf("overlay contents = %q, want saved content", got)
	}
	if got, want := app.refreshed, 1; got != want || app.refreshedVFS != view {
		t.Fatalf("save refresh = %d/%T, want 1/target LogVFS", got, app.refreshedVFS)
	}

	for _, action := range []vfs.PanelAction{vfs.PanelActionCreate, vfs.PanelActionDelete} {
		if !view.HandlePanelAction(app, action, nil) {
			t.Fatalf("read-only action %v was not consumed", action)
		}
	}
	if got, want := len(app.messages), 2; got != want {
		t.Fatalf("read-only messages = %d, want %d", got, want)
	}
	if view.HandlePanelAction(app, vfs.PanelActionActivate, nil) {
		t.Fatal("activate was unexpectedly consumed")
	}
}

type logControllerTestApp struct {
	selected       []string
	refreshed      int
	refreshedVFS   vfs.VFS
	openedVFS      vfs.VFS
	openErr        error
	messages       []string
	editorRequests []vfs.TextEditorRequest
}

func (*logControllerTestApp) GetActivePanelVFS() vfs.VFS  { return nil }
func (*logControllerTestApp) GetPassivePanelVFS() vfs.VFS { return nil }
func (app *logControllerTestApp) GetSelectedNames() []string {
	return append([]string(nil), app.selected...)
}
func (app *logControllerTestApp) GetSelectedName() string {
	if len(app.selected) == 0 {
		return ""
	}
	return app.selected[0]
}
func (*logControllerTestApp) RefreshAll()                {}
func (*logControllerTestApp) SetPendingSelection(string) {}
func (app *logControllerTestApp) RunProgressTask(_ string, _ string, _ bool, worker func(context.Context, func(string, int)) error, done func(error)) {
	done(worker(context.Background(), func(string, int) {}))
}
func (*logControllerTestApp) RunAdvancedProgressTask(_ string, _ bool, _ func(context.Context, vfs.TaskReporter) error, _ func(error)) {
}
func (app *logControllerTestApp) Message(_ string, message string, _ []string) int {
	app.messages = append(app.messages, message)
	return 0
}
func (*logControllerTestApp) InputBox(string, string, string, func(string)) {}
func (*logControllerTestApp) Menu(string, []string, func(int))              {}

func (*logControllerTestApp) PanelSnapshot(vfs.PanelSide) vfs.PanelSnapshot {
	return vfs.PanelSnapshot{}
}
func (*logControllerTestApp) ObservePanelChanges(vfs.PanelObserver) vfs.Registration {
	return nil
}
func (app *logControllerTestApp) OpenPassiveVFS(view vfs.VFS) error {
	app.openedVFS = view
	return app.openErr
}
func (app *logControllerTestApp) RefreshVFS(view vfs.VFS) {
	app.refreshed++
	app.refreshedVFS = view
}
func (app *logControllerTestApp) OpenTextEditor(request vfs.TextEditorRequest) error {
	app.editorRequests = append(app.editorRequests, request)
	return nil
}

var _ vfs.App = (*logControllerTestApp)(nil)
var _ vfs.PanelHost = (*logControllerTestApp)(nil)
var _ vfs.TextEditorHost = (*logControllerTestApp)(nil)
