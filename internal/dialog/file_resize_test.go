package dialog

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// The rename and copy-in-place dialogs follow the f4 window for as long as
// they stay open: a resize gives them half of the new width, keeps them
// centered and stretches the name field to the new border, instead of
// leaving the old rectangle hanging outside the window (#891).
func TestFileInputBoxFollowsWindowResize(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 30)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()

	dlg := FileInputBox("Rename", "Rename 'a.txt' to:", "a.txt", nil)
	if dlg == nil {
		t.Fatal("rename dialog was not created")
	}
	edit := firstDialogEdit(dlg)
	if edit == nil {
		t.Fatal("rename dialog has no text field")
	}

	assertFileDialogGeometry(t, dlg, edit, 120)
	dlg.ResizeConsole(200, 40)
	assertFileDialogGeometry(t, dlg, edit, 200)
	dlg.ResizeConsole(80, 24)
	assertFileDialogGeometry(t, dlg, edit, 80)

	if _, height := dlg.Size(); height != fileInputBoxHeight {
		t.Errorf("dialog height %d after resizing, want it fixed at %d", height, fileInputBoxHeight)
	}
	vtui.AssertLayout(t, dlg)
}

// The F5/F6 dialog has its own layout, so it gets its own resize check.
