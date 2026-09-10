package panel

import (
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"testing"
)

func TestSemanticDropAcrossWorkspaces(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	sourceOwner, destinationOwner := setupMockPanelsFrame(t), setupMockPanelsFrame(t)
	defer sourceOwner.Close()
	defer destinationOwner.Close()
	for _, owner := range []*PanelsFrame{sourceOwner, destinationOwner} {
		owner.ShowPanels = true
		owner.ShowLeftPanel = true
		owner.ShowRightPanel = true
	}
	source, destination := sourceOwner.Panels[0].(*FileSystemPanel), destinationOwner.Panels[0].(*FileSystemPanel)
	source.Vfs = vfs.NewOSVFS(t.TempDir())
	destination.Vfs = vfs.NewOSVFS(t.TempDir())
	source.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "cross-tab.txt"}}}
	destination.Entries = nil
	vtui.FrameManager.Screens = []*vtui.AppScreen{{Number: 7, Frames: []vtui.Frame{sourceOwner}}, {Number: 19, Frames: []vtui.Frame{destinationOwner}}}
	vtui.FrameManager.ActiveIdx = 1
	sourceModel := source.SemanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
	destinationModel := destination.SemanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
	sourceEndpoint := map[string]any{"side": 0, "panelId": sourceModel.ID, "path": sourceModel.Path, "catalogRevision": sourceModel.CatalogRevision, "entryIds": []string{sourceModel.Entries[0].EntryID}}
	request := map[string]any{"side": 0, "panelId": destinationModel.ID, "path": destinationModel.Path, "catalogRevision": destinationModel.CatalogRevision, "source": sourceEndpoint, "operation": "copy"}
	for _, operation := range []string{"copy", "move"} {
		request["operation"] = operation
		plan, err := destinationOwner.planSemanticDrop(request)
		if err != nil {
			t.Fatal(err)
		}
		if plan.sourceOwner != sourceOwner || plan.source != source.Vfs || plan.sourceDir != source.Vfs.GetPath() || plan.target.fs != destination.Vfs || plan.move != (operation == "move") {
			t.Fatalf("wrong cross-workspace plan: %+v", plan)
		}
	}
	if got := semanticWorkspaceDragTarget("workspace-tab-19"); got != destinationOwner {
		t.Fatal("stable tab identity did not resolve")
	}
	if semanticWorkspaceDragTarget("workspace-tab-1") != nil {
		t.Fatal("tab index was treated as identity")
	}
	vtui.FrameManager.ActiveIdx = 0
	if !HandleSemanticWorkspaceDrag(map[string]any{"action": "workspace.dragActivate", "target": "workspace-tab-19"}) || vtui.FrameManager.ActiveIdx != 1 {
		t.Fatal("hover did not immediately activate the destination workspace")
	}
	destinationOwner.ShowPanels = false
	if semanticWorkspaceDragTarget("workspace-tab-19") != nil {
		t.Fatal("terminal tab accepted drop")
	}
	destinationOwner.ShowPanels = true
	sourceEndpoint["path"] = "stale"
	if _, err := destinationOwner.planSemanticDrop(request); err == nil {
		t.Fatal("stale source accepted")
	}
	sourceEndpoint["path"] = sourceModel.Path
	sourceOwner.Closed = true
	if _, err := destinationOwner.planSemanticDrop(request); err == nil {
		t.Fatal("closed source accepted")
	}
	sourceOwner.Closed = false
	vtui.FrameManager.Screens = vtui.FrameManager.Screens[1:]
	vtui.FrameManager.ActiveIdx = 0
	if _, err := destinationOwner.planSemanticDrop(request); err == nil {
		t.Fatal("removed source workspace accepted")
	}
}

func TestSemanticDropOnWorkspaceTabUsesActivePanel(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	owner := setupMockPanelsFrame(t)
	defer owner.Close()
	owner.ShowPanels, owner.ShowLeftPanel, owner.ShowRightPanel = true, true, true
	owner.ActiveIdx = 1
	store := &TempPanelStore{}
	temp := NewTempPanelVFS(nil, store, 0)
	owner.Panels[1].(*FileSystemPanel).Vfs = temp
	vtui.FrameManager.Screens = []*vtui.AppScreen{{Number: 23, Frames: []vtui.Frame{owner}}}
	vtui.FrameManager.ActiveIdx = 0
	source := vfs.NewOSVFS(t.TempDir())
	path := source.Join(source.GetPath(), "external.txt")
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	if !HandleSemanticWorkspaceDrag(map[string]any{"action": "workspace.dropFiles", "target": "workspace-tab-23", "operation": "copy", "paths": []string{path}}) {
		t.Fatal("unhandled tab drop")
	}
	entries := readTempPanelItems(t, temp)
	if len(entries) != 1 {
		t.Fatalf("tab did not use active panel: %v", entries)
	}
}
