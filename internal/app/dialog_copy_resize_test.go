package app

import (
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"testing"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestCopyDialogFollowsWindowResize(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	src := pf.Panels[0].(*panel.FileSystemPanel)
	if err := src.Vfs.SetPath(t.TempDir()); err != nil {
		t.Fatalf("set source path: %v", err)
	}
	src.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "test.txt"}}}
	src.SetCursorIndex(0)
	pf.ActiveIdx = 0

	ActionCopyMove(pf, false)
	dlg, ok := vtui.FrameManager.GetTopFrame().(*dialog.FileDialog)
	if !ok {
		t.Fatal("copy dialog not found on top")
	}
	defer vtui.FrameManager.Pop()

	edit := firstDialogEdit(dlg)
	if edit == nil {
		t.Fatal("copy dialog has no destination field")
	}

	assertFileDialogGeometry(t, dlg, edit, 80)
	vtui.AssertLayout(t, dlg)

	dlg.ResizeConsole(160, 40)
	assertFileDialogGeometry(t, dlg, edit, 160)

	dlg.ResizeConsole(100, 30)
	assertFileDialogGeometry(t, dlg, edit, 100)

	if _, height := dlg.Size(); height != dialog.CopyBoxHeight {
		t.Errorf("dialog height %d after resizing, want it fixed at %d", height, dialog.CopyBoxHeight)
	}
}

// Test helpers do not cross a package boundary, so the two the dialog package
// keeps for its own resize test are spelled again here. They are eight lines
// and they assert the same contract from the other side of the move.
func firstDialogEdit(dlg vtui.Container) *vtui.Edit {
	for _, item := range dlg.GetChildren() {
		if edit, ok := item.(*vtui.Edit); ok {
			return edit
		}
	}
	return nil
}

func assertFileDialogGeometry(t *testing.T, dlg *dialog.FileDialog, edit *vtui.Edit, screenWidth int) {
	t.Helper()

	want := dialog.FileDialogWidth(screenWidth)
	width, _ := dlg.Size()
	if width != want {
		t.Fatalf("dialog width on a %d column screen = %d, want %d", screenWidth, width, want)
	}
	if dlg.X1 != (screenWidth-want)/2 {
		t.Fatalf("dialog starts at column %d on a %d column screen, want %d", dlg.X1, screenWidth, (screenWidth-want)/2)
	}
	if dlg.X1 < 0 || dlg.X2 >= screenWidth {
		t.Fatalf("dialog columns %d..%d fall outside a %d column screen", dlg.X1, dlg.X2, screenWidth)
	}
	if edit.X1 != dlg.X1+2 || edit.X2 != dlg.X2-2 {
		t.Fatalf("text field columns %d..%d, want it stretched over %d..%d", edit.X1, edit.X2, dlg.X1+2, dlg.X2-2)
	}
}
