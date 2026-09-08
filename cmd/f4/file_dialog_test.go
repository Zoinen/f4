package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestFileDialogWidth(t *testing.T) {
	cases := []struct{ screen, want int }{
		{0, 50},
		{40, 40},
		{50, 40},
		{60, 40},
		{80, 40},
		{120, 60},
		{200, 100},
	}
	for _, c := range cases {
		if got := fileDialogWidth(c.screen); got != c.want {
			t.Errorf("fileDialogWidth(%d) = %d, want %d", c.screen, got, c.want)
		}
	}
}

// The rename and copy-in-place dialogs follow the f4 window for as long as
// they stay open: a resize gives them half of the new width, keeps them
// centered and stretches the name field to the new border, instead of
// leaving the old rectangle hanging outside the window (#891).
func TestFileInputBoxFollowsWindowResize(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 30)
	vtui.FrameManager.Init(screen)
	SetDefaultF4Palette()

	dlg := fileInputBox("Rename", "Rename 'a.txt' to:", "a.txt", nil)
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

	if _, height := dlg.size(); height != fileInputBoxHeight {
		t.Errorf("dialog height %d after resizing, want it fixed at %d", height, fileInputBoxHeight)
	}
	vtui.AssertLayout(t, dlg)
}

// The F5/F6 dialog has its own layout, so it gets its own resize check.
func TestCopyDialogFollowsWindowResize(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	SetDefaultF4Palette()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	src := pf.panels[0].(*FileSystemPanel)
	if err := src.vfs.SetPath(t.TempDir()); err != nil {
		t.Fatalf("set source path: %v", err)
	}
	src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "test.txt"}}}
	src.SetCursorIndex(0)
	pf.activeIdx = 0

	actionCopyMove(pf, false)
	dlg, ok := vtui.FrameManager.GetTopFrame().(*fileDialog)
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

	if _, height := dlg.size(); height != copyDialogHeight {
		t.Errorf("dialog height %d after resizing, want it fixed at %d", height, copyDialogHeight)
	}
}

func firstDialogEdit(dlg vtui.Container) *vtui.Edit {
	for _, item := range dlg.GetChildren() {
		if edit, ok := item.(*vtui.Edit); ok {
			return edit
		}
	}
	return nil
}

func assertFileDialogGeometry(t *testing.T, dlg *fileDialog, edit *vtui.Edit, screenWidth int) {
	t.Helper()

	want := fileDialogWidth(screenWidth)
	width, _ := dlg.size()
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
