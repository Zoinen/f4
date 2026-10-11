package mediainfo

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
)

// canOpenCurrentTestApp is a minimal vfs.App double for canOpenCurrent
// (f4#1356). It deliberately does not implement vfs.SelectedIsDirHost, so
// it also pins the fallback for a host that cannot answer "is the
// selection a directory?" at all.
type canOpenCurrentTestApp struct {
	activeVFS vfs.VFS
	selected  string
}

func (a *canOpenCurrentTestApp) GetActivePanelVFS() vfs.VFS { return a.activeVFS }
func (*canOpenCurrentTestApp) GetPassivePanelVFS() vfs.VFS  { return nil }
func (*canOpenCurrentTestApp) GetSelectedNames() []string   { return nil }
func (a *canOpenCurrentTestApp) GetSelectedName() string    { return a.selected }
func (*canOpenCurrentTestApp) RefreshAll()                  {}
func (*canOpenCurrentTestApp) SetPendingSelection(string)   {}
func (*canOpenCurrentTestApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*canOpenCurrentTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (*canOpenCurrentTestApp) Message(string, string, []string) int          { return 0 }
func (*canOpenCurrentTestApp) InputBox(string, string, string, func(string)) {}
func (*canOpenCurrentTestApp) Menu(string, []string, func(int))              {}

// dirAwareTestApp additionally implements vfs.SelectedIsDirHost, the way
// PanelsFrame answers it from the cursor entry's already-cached
// vfs.VFSItem.IsDir (f4#1356), so canOpenCurrent can be exercised against a
// host that does know.
type dirAwareTestApp struct {
	canOpenCurrentTestApp
	isDir    bool
	dirKnown bool
}

func (a *dirAwareTestApp) GetSelectedIsDir() (bool, bool) { return a.isDir, a.dirKnown }

// TestCanOpenCurrent checks the Enabled predicate wired into the panel
// command (f4#1356) against the cases openCurrent itself would otherwise
// have to reject with an error dialog, plus the directory case that dialog
// used to be the only way to discover.
func TestCanOpenCurrent(t *testing.T) {
	local := vfs.NewOSVFS(t.TempDir())

	if canOpenCurrent(nil) {
		t.Error("nil app: want disabled")
	}
	if canOpenCurrent(&canOpenCurrentTestApp{activeVFS: nil, selected: "song.mp3"}) {
		t.Error("no active panel VFS: want disabled")
	}
	if canOpenCurrent(&canOpenCurrentTestApp{activeVFS: local, selected: ""}) {
		t.Error("no selection: want disabled")
	}
	if !canOpenCurrent(&canOpenCurrentTestApp{activeVFS: local, selected: "song.mp3"}) {
		t.Error("host without SelectedIsDirHost, otherwise-valid selection: want enabled (unknown falls back to enabled)")
	}

	dirKnownTrue := &dirAwareTestApp{
		canOpenCurrentTestApp: canOpenCurrentTestApp{activeVFS: local, selected: "folder"},
		isDir:                 true,
		dirKnown:              true,
	}
	if canOpenCurrent(dirKnownTrue) {
		t.Error("selection known to be a directory: want disabled")
	}

	fileKnown := &dirAwareTestApp{
		canOpenCurrentTestApp: canOpenCurrentTestApp{activeVFS: local, selected: "song.mp3"},
		isDir:                 false,
		dirKnown:              true,
	}
	if !canOpenCurrent(fileKnown) {
		t.Error("selection known not to be a directory: want enabled")
	}

	dirUnknown := &dirAwareTestApp{
		canOpenCurrentTestApp: canOpenCurrentTestApp{activeVFS: local, selected: "song.mp3"},
		isDir:                 true,
		dirKnown:              false,
	}
	if !canOpenCurrent(dirUnknown) {
		t.Error("SelectedIsDirHost present but known=false: want enabled (fall back to openCurrent's own error dialog)")
	}
}
