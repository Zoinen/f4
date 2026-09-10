package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

func TestPanelVisibilityActionsDoNotReloadCatalogs(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := paneltest.SetupMockPanelsFrame(t)
	t.Cleanup(func() {
		pf.Close()
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	})
	pf.LastAutoRefresh = time.Now().Add(time.Hour)

	type snapshot struct {
		entries []*panel.FileEntry
		cursor  int
		top     int
	}
	want := [2]snapshot{}
	for side, pane := range pf.Panels {
		fsp := pane.(*panel.FileSystemPanel)
		if fsp.CancelLoad != nil {
			fsp.CancelLoad()
			fsp.CancelLoad = nil
		}
		fsp.IsLoading = false
		fsp.PendingSelection = ""
		fsp.Vfs = vfs.NewOSVFS(t.TempDir())
		fsp.Entries = []*panel.FileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "one"}},
			{VFSItem: vfs.VFSItem{Name: "two"}},
			{VFSItem: vfs.VFSItem{Name: "three"}},
		}
		fsp.CursorIdx = 2
		fsp.Table.TopPos = 1
		want[side] = snapshot{fsp.Entries, fsp.CursorIdx, fsp.Table.TopPos}
	}

	vtui.FrameManager.Push(pf)
	assertIntact := func(stage string) {
		t.Helper()
		for side, pane := range pf.Panels {
			fsp := pane.(*panel.FileSystemPanel)
			if fsp.IsLoading {
				t.Fatalf("%s: side %d started a directory reload", stage, side)
			}
			if len(fsp.Entries) != len(want[side].entries) {
				t.Fatalf("%s: side %d catalog length = %d, want %d",
					stage, side, len(fsp.Entries), len(want[side].entries))
			}
			for i := range fsp.Entries {
				if fsp.Entries[i] != want[side].entries[i] {
					t.Fatalf("%s: side %d entry %d was replaced", stage, side, i)
				}
			}
			if fsp.CursorIdx != want[side].cursor || fsp.Table.TopPos != want[side].top {
				t.Fatalf("%s: side %d cursor/top = %d/%d, want %d/%d",
					stage, side, fsp.CursorIdx, fsp.Table.TopPos,
					want[side].cursor, want[side].top)
			}
		}
	}

	for _, tc := range []struct {
		name   string
		stages int
	}{
		{name: "Panel.Toggle", stages: 2},
		{name: "Panel.ToggleLeftPanel", stages: 2},
		{name: "Panel.ToggleRightPanel", stages: 2},
		{name: "Panel.TogglePassivePanel", stages: 2},
	} {
		for stage := 0; stage < tc.stages; stage++ {
			if !RunAction(tc.name) {
				t.Fatalf("%s stage %d did not run", tc.name, stage)
			}
			assertIntact(tc.name)
		}
	}
}
