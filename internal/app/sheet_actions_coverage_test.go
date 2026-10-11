package app

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestSelectedSpreadsheetPathFiltersPanelSelection(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ActiveIdx = 0
	left := pf.Panels[0].(*panel.FileSystemPanel)
	left.Vfs = vfs.NewOSVFS(root)
	vtui.FrameManager.Push(pf)

	for _, tc := range []struct {
		name string
		want string
	}{
		{"book.xlsx", filepath.Join(root, "book.xlsx")},
		{"export.CSV", filepath.Join(root, "export.CSV")},
		{"notes.txt", ""},
		{"..", ""},
	} {
		left.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: tc.name}}}
		left.SetCursorIndex(0)
		if got := selectedSpreadsheetPath(); got != tc.want {
			t.Errorf("selectedSpreadsheetPath(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSpreadsheetActionsWithoutFrameManagerAreInert(t *testing.T) {
	old := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = old })
	if activePanelsFrame() != nil {
		t.Fatal("active panel found without a frame manager")
	}
	if findSheetWorkspace() {
		t.Fatal("sheet workspace found without a frame manager")
	}
	if actionSpreadsheet() {
		t.Fatal("spreadsheet action handled without a frame manager")
	}
}
