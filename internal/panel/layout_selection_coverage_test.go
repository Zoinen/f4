package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestPanelsFrameWideAndViewModeGuards(t *testing.T) {
	left := &FileSystemPanel{Table: vtui.NewTable(0, 0, 20, 10, nil), Frame: vtui.NewBorderedFrame(0, 0, 20, 10, vtui.SingleBox, ""), Vfs: vfs.NewNullVFS(0)}
	right := &FileSystemPanel{Table: vtui.NewTable(0, 0, 20, 10, nil), Frame: vtui.NewBorderedFrame(0, 0, 20, 10, vtui.SingleBox, ""), Vfs: vfs.NewNullVFS(0)}
	left.Table.Columns = []vtui.TableColumn{{Title: "Name", Width: 20}}
	right.Table.Columns = []vtui.TableColumn{{Title: "Name", Width: 20}}
	pf := &PanelsFrame{Panels: [2]Panel{left, right}, ActiveIdx: 0}

	pf.SetWidePanel(1)
	if !pf.Wide || pf.WidePanel != 1 || pf.ActiveIdx != 1 || !pf.ShowPanels {
		t.Fatalf("SetWidePanel(1) = wide=%v panel=%d active=%d show=%v", pf.Wide, pf.WidePanel, pf.ActiveIdx, pf.ShowPanels)
	}
	pf.ExitWide()
	if pf.Wide || pf.WidePanel != -1 {
		t.Fatalf("ExitWide left wide state: wide=%v panel=%d", pf.Wide, pf.WidePanel)
	}
	pf.SetWidePanel(99)
	if pf.Wide || pf.WidePanel != -1 {
		t.Fatalf("invalid SetWidePanel changed state: wide=%v panel=%d", pf.Wide, pf.WidePanel)
	}

	pf.SetPanelViewMode(0, ViewModeDetailed)
	if left.ViewMode != ViewModeDetailed || left.Wide {
		t.Fatalf("SetPanelViewMode did not update ordinary mode: mode=%v wide=%v", left.ViewMode, left.Wide)
	}
	pf.SetPanelViewMode(-1, ViewModeBrief)
	pf.SetPanelViewMode(2, ViewModeBrief)
	pf.Panels[1] = &mouseCaptureTestPanel{}
	pf.SetPanelViewMode(1, ViewModeBrief)
}
