package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestWorkspaceSessionSerializationPreservesOrderNumbersAndActiveTab(t *testing.T) {
	states := []workspaceSessionState{
		{
			Number: 2, ActivePanel: 0, WidePanel: -1,
			ShowPanels: true, ShowLeft: true, ShowRight: true,
			Left:  panelSessionState{Path: "C:/alpha", Cursor: "a.txt", ViewMode: int(ViewModeBrief), SortMode: int(SortName)},
			Right: panelSessionState{Path: "D:/beta", Cursor: "b.txt", ViewMode: int(ViewModeDetailed), SortMode: int(SortTime), SortReverse: true},
		},
		{
			Number: 1, ActivePanel: 1, WidePanel: 1,
			ShowPanels: true, ShowLeft: false, ShowRight: true,
			Left:  panelSessionState{Path: "C:/short", ViewMode: int(ViewModeMedium), SortMode: int(SortExt)},
			Right: panelSessionState{Path: "D:/long", ViewMode: int(ViewModeBrief), SortMode: int(SortSize)},
		},
	}

	var encoded strings.Builder
	writeWorkspaceSessions(&encoded, states, 1)
	got, active := loadWorkspaceSessions(ParseIni(strings.NewReader(encoded.String())))
	if active != 1 {
		t.Fatalf("active workspace = %d, want 1", active)
	}
	if !reflect.DeepEqual(got, states) {
		t.Fatalf("workspace session round trip mismatch:\n got: %#v\nwant: %#v", got, states)
	}
}

func TestCaptureWorkspaceSessionsUsesTabOrderAndActiveIndex(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })

	newPanels := func(leftPath, rightPath string) *PanelsFrame {
		return &PanelsFrame{
			panels: [2]Panel{
				&FileSystemPanel{vfs: vfs.NewOSVFS(leftPath), viewMode: ViewModeBrief, sortMode: SortName},
				&FileSystemPanel{vfs: vfs.NewOSVFS(rightPath), viewMode: ViewModeDetailed, sortMode: SortSize},
			},
			activeIdx: 1, widePanel: -1, showPanels: true, showLeftPanel: true, showRightPanel: true,
		}
	}
	first := newPanels(t.TempDir(), t.TempDir())
	second := newPanels(t.TempDir(), t.TempDir())
	vtui.FrameManager.Screens = []*vtui.AppScreen{
		{Number: 8, Frames: []vtui.Frame{first}},
		{Number: 3, Frames: []vtui.Frame{second}},
	}
	vtui.FrameManager.ActiveIdx = 1

	states, active := captureWorkspaceSessions()
	if active != 1 {
		t.Fatalf("captured active workspace = %d, want 1", active)
	}
	if len(states) != 2 || states[0].Number != 8 || states[1].Number != 3 {
		t.Fatalf("captured workspace order/numbers = %#v", states)
	}
	if states[0].Left.Path != first.panels[0].(*FileSystemPanel).vfs.GetPath() ||
		states[1].Right.Path != second.panels[1].(*FileSystemPanel).vfs.GetPath() {
		t.Fatal("captured panel paths do not belong to their ordered workspaces")
	}
}
