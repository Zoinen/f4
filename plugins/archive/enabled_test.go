package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// enabledTestApp is a minimal vfs.App double for canOperateOnArchive and
// canAddArchive (f4#1356): just enough of the interface for the predicates
// to read, with none of the UI-bridge methods ever expected to be called.
type enabledTestApp struct {
	activeVFS vfs.VFS
	selected  string
	names     []string
}

func (a *enabledTestApp) GetActivePanelVFS() vfs.VFS { return a.activeVFS }
func (*enabledTestApp) GetPassivePanelVFS() vfs.VFS  { return nil }
func (a *enabledTestApp) GetSelectedNames() []string { return a.names }
func (a *enabledTestApp) GetSelectedName() string    { return a.selected }
func (*enabledTestApp) RefreshAll()                  {}
func (*enabledTestApp) SetPendingSelection(string)   {}
func (*enabledTestApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*enabledTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (*enabledTestApp) Message(string, string, []string) int          { return 0 }
func (*enabledTestApp) InputBox(string, string, string, func(string)) {}
func (*enabledTestApp) Menu(string, []string, func(int))              {}

// TestCanOperateOnArchive covers the eligibility rule that backs both
// archive.extract's PluginCommand.Enabled (dims the Files menu row/palette
// entry) and the direct guard actionExtractArchive/actionTestArchive now
// apply against their own Shift+F2/Shift+F3 hotkeys, which bypass
// PluginCommand.Enabled entirely since they are wired through
// RegisterGlobalHotkey rather than through the plugin-command dispatch
// (f4#1356).
func TestCanOperateOnArchive(t *testing.T) {
	tmpDir := t.TempDir()
	local := vfs.NewOSVFS(tmpDir)

	if canOperateOnArchive(&enabledTestApp{activeVFS: nil, selected: "a.zip"}) {
		t.Error("no active panel VFS: want disabled")
	}

	if canOperateOnArchive(&enabledTestApp{activeVFS: local, selected: ""}) {
		t.Error("no selection: want disabled")
	}
	if canOperateOnArchive(&enabledTestApp{activeVFS: local, selected: ".."}) {
		t.Error("cursor on '..': want disabled")
	}

	textPath := filepath.Join(tmpDir, "notes.txt")
	if err := os.WriteFile(textPath, []byte("plain text, not an archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if canOperateOnArchive(&enabledTestApp{activeVFS: local, selected: "notes.txt"}) {
		t.Error("selection is a plain file, not a recognized archive: want disabled")
	}

	// DetectFormat recognizes ".zip" from the name alone, so the extension
	// is what matters here -- the content never has to look like a real
	// archive for this case.
	zipPath := filepath.Join(tmpDir, "data.zip")
	if err := os.WriteFile(zipPath, []byte("not a real zip payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if !canOperateOnArchive(&enabledTestApp{activeVFS: local, selected: "data.zip"}) {
		t.Error("selection has a recognized archive extension: want enabled")
	}

	if !canOperateOnArchive(&enabledTestApp{activeVFS: &ArchiveVFS{}, selected: ""}) {
		t.Error("active panel already is an ArchiveVFS: want enabled regardless of selection")
	}
}

// TestCanAddArchive covers the eligibility rule that backs both
// archive.add's PluginCommand.Enabled and the direct guard the Shift+F1
// hotkey needs for the same reason canOperateOnArchive documents (f4#1356).
// actionAddArchive already silently declines an empty/".."-only selection,
// so this only pins that canAddArchive agrees with it.
func TestCanAddArchive(t *testing.T) {
	local := vfs.NewOSVFS(t.TempDir())

	if canAddArchive(&enabledTestApp{activeVFS: nil, names: []string{"file.txt"}}) {
		t.Error("no active panel VFS: want disabled")
	}
	if canAddArchive(&enabledTestApp{activeVFS: local, names: nil}) {
		t.Error("nothing selected: want disabled")
	}
	if canAddArchive(&enabledTestApp{activeVFS: local, names: []string{".."}}) {
		t.Error("only the '..' row selected: want disabled")
	}
	if !canAddArchive(&enabledTestApp{activeVFS: local, names: []string{"file.txt"}}) {
		t.Error("one real file selected: want enabled")
	}
	if !canAddArchive(&enabledTestApp{activeVFS: local, names: []string{"..", "file.txt"}}) {
		t.Error("'..' alongside a real selection: want enabled")
	}
}
