//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestPanelsFrame_DriveMenu_CursorOnCurrentDrive verifies that opening the
// drive menu (Alt+F1/F2) for a panel sitting on a real filesystem drive lands
// the cursor on that drive, not on the "Other panel" entry.
func TestPanelsFrame_DriveMenu_CursorOnCurrentDrive(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Put the left panel on the current working directory's drive.
	cur, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	vol := filepath.VolumeName(cur)
	if vol == "" {
		t.Skip("current directory has no drive letter")
	}
	left := pf.panels[0].(*FileSystemPanel)
	left.vfs = vfs.NewOSVFS(cur)
	if err := left.vfs.SetPath(cur); err != nil {
		t.Fatal(err)
	}

	// Open Alt+F1 (left panel drive menu).
	pf.showDriveMenu(0)

	top := vtui.FrameManager.GetTopFrame()
	menu, ok := top.(*vtui.VMenu)
	if !ok {
		t.Fatal("Drive menu not opened")
	}

	if menu.SelectPos == 0 {
		t.Fatalf("cursor landed on \"Other panel\" (row 0) instead of drive %s", vol)
	}

	row := menu.Items[menu.SelectPos].Text
	if !strings.Contains(strings.ToUpper(row), strings.ToUpper(vol)) {
		t.Errorf("cursor on row %q, expected it to match drive %s", row, vol)
	}
}
