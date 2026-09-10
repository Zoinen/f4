package app

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/vtui"
	"testing"
)

func TestQueueWorkspaceSemanticCloseGuardAndTabIdentity(t *testing.T) {
	withSemanticQueueTestState(t)
	vtui.FrameManager.Push(vtui.NewDesktop())
	qf := fileops.NewQueueFrame()
	active := &fileops.QueueTask{ID: 31, Type: "Copy", Desc: "large copy", State: "Running"}
	// Deliberately leave QueueFrame's UI snapshot stale. Enqueue mutates the
	// authoritative manager before its posted UpdateTasks callback runs; a
	// semantic close in that interval must still see and protect this task.
	qf.UpdateTasks(nil)
	fileops.GlobalQueueManager = fileops.NewQueueManagerWithTasks(active)
	vtui.FrameManager.AddScreenBackground(qf)
	fileops.GlobalQueueManager.EnsureQueueWorkspace()
	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatalf("workspace count = %d, want 2", len(vtui.FrameManager.Screens))
	}

	inactiveScene := vtui.FrameManager.ExportSemanticScene()
	tabs := inactiveScene["workspaceTabs"].(map[string]any)["tabs"].([]map[string]any)
	if tabs[1]["closable"] != false {
		t.Fatalf("active queue tab remained closable: %#v", tabs[1])
	}
	tabID := tabs[1]["id"].(string)
	if !HandleSemanticAction(map[string]any{
		"target": tabID, "action": "workspace.close", "index": 1,
	}) {
		t.Fatal("active queue close guard did not claim the request")
	}
	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatal("active queue workspace was closed")
	}

	vtui.FrameManager.SwitchScreen(1)
	activeScene := vtui.FrameManager.ExportSemanticScene()
	queue := activeScene["operationsQueue"].(map[string]any)
	if queue["tabId"] != tabID || queue["workspaceIndex"] != 1 || queue["workspaceNumber"] != tabs[1]["number"] {
		t.Fatalf("queue workspace identity = %#v, tab = %#v", queue, tabs[1])
	}

	active.Mu.Lock()
	active.State = "Done"
	active.Progress = 100
	active.Mu.Unlock()
	closableScene := vtui.FrameManager.ExportSemanticScene()
	closableTabs := closableScene["workspaceTabs"].(map[string]any)["tabs"].([]map[string]any)
	if closableTabs[1]["closable"] != true {
		t.Fatalf("terminal queue tab did not become closable: %#v", closableTabs[1])
	}
	if !HandleSemanticAction(map[string]any{
		"target": tabID, "action": "workspace.close", "index": 1,
	}) {
		t.Fatal("terminal queue close was rejected")
	}
	if len(vtui.FrameManager.Screens) != 1 {
		t.Fatalf("terminal queue workspace was not closed: %d screens", len(vtui.FrameManager.Screens))
	}
}
