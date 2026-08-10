package main

import (
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Visibility is presentation state. Hiding or revealing a panel must not
// start a directory reload: native frontends keep their renderer and scroll
// position alive while the panel is covered by the terminal surface.
func TestPanelVisibilityActionsDoNotReloadCatalogs(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	t.Cleanup(func() {
		pf.Close()
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	})
	pf.lastAutoRefresh = time.Now().Add(time.Hour)

	type snapshot struct {
		entries []*fileEntry
		cursor  int
		top     int
	}
	want := [2]snapshot{}
	for side, panel := range pf.panels {
		fsp := panel.(*FileSystemPanel)
		if fsp.cancelLoad != nil {
			fsp.cancelLoad()
			fsp.cancelLoad = nil
		}
		fsp.isLoading = false
		fsp.pendingSelection = ""
		fsp.vfs = vfs.NewOSVFS(t.TempDir())
		fsp.entries = []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
			{VFSItem: vfs.VFSItem{Name: "one"}},
			{VFSItem: vfs.VFSItem{Name: "two"}},
			{VFSItem: vfs.VFSItem{Name: "three"}},
		}
		fsp.cursorIdx = 2
		fsp.table.TopPos = 1
		want[side] = snapshot{fsp.entries, fsp.cursorIdx, fsp.table.TopPos}
	}

	vtui.FrameManager.Push(pf)
	assertIntact := func(stage string) {
		t.Helper()
		for side, panel := range pf.panels {
			fsp := panel.(*FileSystemPanel)
			if fsp.isLoading {
				t.Fatalf("%s: side %d started a directory reload", stage, side)
			}
			if len(fsp.entries) != len(want[side].entries) {
				t.Fatalf("%s: side %d catalog length = %d, want %d",
					stage, side, len(fsp.entries), len(want[side].entries))
			}
			for i := range fsp.entries {
				if fsp.entries[i] != want[side].entries[i] {
					t.Fatalf("%s: side %d entry %d was replaced", stage, side, i)
				}
			}
			if fsp.cursorIdx != want[side].cursor || fsp.table.TopPos != want[side].top {
				t.Fatalf("%s: side %d cursor/top = %d/%d, want %d/%d",
					stage, side, fsp.cursorIdx, fsp.table.TopPos,
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
