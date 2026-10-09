package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// pickDriveRow opens the drive menu of side and chooses the row named name.
func pickDriveRow(t *testing.T, pf *PanelsFrame, side int, name string) {
	t.Helper()
	pf.ShowDriveMenu(side)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	for i, item := range menu.Items {
		if !item.Separator && strings.Contains(item.Text, name) {
			menu.OnAction(i)
			return
		}
	}
	t.Fatalf("no %q row in the drive menu", name)
}

func newRevealTestFrame(t *testing.T) (*PanelsFrame, string) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	dir := t.TempDir()
	t.Cleanup(sysinfo.SnapshotDrives())
	sysinfo.SetDrives([]sysinfo.DriveEntry{
		{Name: "Alpha", Factory: func() vfs.VFS { return vfs.NewOSVFS(dir) }},
	})
	return pf, dir
}

// f4#1847: choosing a location for a hidden panel shows the panel.
func TestDriveMenuChoiceShowsTheHiddenSide(t *testing.T) {
	pf, _ := newRevealTestFrame(t)
	pf.ShowLeftPanel = false

	pf.ShowDriveMenu(0)
	if pf.ShowLeftPanel {
		t.Fatal("opening the menu alone showed the panel; Esc must leave it hidden")
	}
	vtui.FrameManager.GetTopFrame().(interface{ Close() }).Close()

	pickDriveRow(t, pf, 0, "Alpha")
	if !pf.ShowLeftPanel || !pf.ShowPanels {
		t.Fatalf("left panel still hidden after choosing a location: left=%v panels=%v", pf.ShowLeftPanel, pf.ShowPanels)
	}
}

func TestDriveMenuChoiceBringsPanelsBackAfterCtrlO(t *testing.T) {
	pf, _ := newRevealTestFrame(t)
	pf.TogglePanelsVisibility() // Ctrl+O
	if pf.ShowPanels {
		t.Fatal("Ctrl+O did not hide the panels")
	}
	pickDriveRow(t, pf, 1, "Alpha")
	if !pf.ShowPanels || !pf.ShowRightPanel {
		t.Fatalf("panels still hidden after choosing a location: panels=%v right=%v", pf.ShowPanels, pf.ShowRightPanel)
	}
}
