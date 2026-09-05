package main

import (
	"errors"
	"testing"

	"github.com/unxed/vtui"
)

func withSemanticQueueTestState(t *testing.T) {
	t.Helper()
	oldQueue := GlobalQueueManager
	t.Cleanup(func() {
		GlobalQueueManager = oldQueue
	})
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
}

func TestQueueSemanticModelExportsNativeQueueState(t *testing.T) {
	withSemanticQueueTestState(t)
	detailsOpened := false
	qf := NewQueueFrame()
	qf.UpdateTasks([]*QueueTask{
		{ID: 11, Type: "Copy", Desc: "Copy files", State: "Queued"},
		{
			ID: 12, Type: "Move", Desc: "Move files", State: "Running",
			Action: "Moving", CurrentFile: "video.mov", CurrentProgress: 42,
			Progress: 25, TotalText: "250 MiB / 1 GiB", Elapsed: "Elapsed: 00:00:04",
			ETA: "Remaining: 00:00:12", Speed: "64 MiB/s",
			OpenDetails: func(vtui.Frame) { detailsOpened = true },
		},
		{ID: 13, Type: "Delete", Desc: "Delete old", State: "Error", ErrorMsg: errors.New("permission denied")},
		{ID: 14, Type: "Apply", Desc: "Apply command", State: "Done", Progress: 100},
		{ID: 15, Type: "Archive", Desc: "Archive files", State: "Cancelled"},
	})
	qf.table.SelectPos = 1

	node := qf.SemanticNode(nil)
	if node["kind"] != "operationsQueue" || node["selectedTaskId"] != 12 {
		t.Fatalf("queue identity/selection = %#v", node)
	}
	if node["activeCount"] != 2 || node["queuedCount"] != 1 || node["runningCount"] != 1 ||
		node["completedCount"] != 1 || node["errorCount"] != 1 || node["cancelledCount"] != 1 {
		t.Fatalf("queue counts = %#v", node)
	}
	if node["hasActive"] != true || node["canClear"] != true || node["canClose"] != false {
		t.Fatalf("queue capabilities = %#v", node)
	}
	items := node["items"].([]map[string]any)
	running := items[1]
	if running["id"] != "queue-task-12" || running["stateClass"] != "running" ||
		running["displayText"] != "video.mov" || running["currentProgress"] != 42 ||
		running["progress"] != 25 || running["elapsed"] != "Elapsed: 00:00:04" ||
		running["eta"] != "Remaining: 00:00:12" || running["speed"] != "64 MiB/s" ||
		running["hasDetails"] != true || running["cancellable"] != true {
		t.Fatalf("running item = %#v", running)
	}
	errorItem := items[2]
	if errorItem["error"] != "permission denied" || errorItem["stateClass"] != "error" ||
		errorItem["hasDetails"] != true || errorItem["terminal"] != true {
		t.Fatalf("error item = %#v", errorItem)
	}

	target := vtui.SemanticID(qf)
	if qf.HandleSemanticAction(map[string]any{
		"target": "stale-queue", "action": "queue.select", "taskId": 11, "index": 0,
	}) {
		t.Fatal("action for another queue target was accepted")
	}
	if qf.HandleSemanticAction(map[string]any{
		"target": target, "action": "queue.select", "taskId": 11, "index": 1,
	}) {
		t.Fatal("mismatched stable task ID and row index were accepted")
	}
	if !qf.HandleSemanticAction(map[string]any{
		"target": target, "action": "queue.activate", "taskId": 12, "index": 1,
	}) || !detailsOpened {
		t.Fatal("valid queue activation did not open task details")
	}
}

func TestQueueSemanticCancelConfirmsAndClearKeepsActiveTasks(t *testing.T) {
	withSemanticQueueTestState(t)
	qf := NewQueueFrame()
	queued := &QueueTask{ID: 21, Type: "Copy", Desc: "queued", State: "Queued"}
	done := &QueueTask{ID: 22, Type: "Copy", Desc: "done", State: "Done"}
	failed := &QueueTask{ID: 23, Type: "Copy", Desc: "failed", State: "Error"}
	qf.UpdateTasks([]*QueueTask{queued, done, failed})
	GlobalQueueManager = &OpQueueManager{
		tasks:      []*QueueTask{queued, done, failed},
		activeKeys: make(map[string]bool),
		frame:      qf,
	}
	vtui.FrameManager.Push(qf)

	target := vtui.SemanticID(qf)
	if !qf.HandleSemanticAction(map[string]any{
		"target": target, "action": "queue.cancel", "taskId": 21, "index": 0,
	}) {
		t.Fatal("cancellable task action was rejected")
	}
	if top := vtui.FrameManager.GetTopFrame(); top == nil || top == qf || top.GetTitle() != " Confirm " {
		t.Fatalf("cancel did not open the existing confirmation dialog: %T %#v", top, top)
	}
	queued.mu.Lock()
	state := queued.State
	queued.mu.Unlock()
	if state != "Queued" {
		t.Fatalf("cancel action bypassed confirmation, state = %q", state)
	}

	if !qf.HandleSemanticAction(map[string]any{"target": target, "action": "queue.clearCompleted"}) {
		t.Fatal("clear completed action was rejected")
	}
	GlobalQueueManager.mu.Lock()
	remaining := append([]*QueueTask(nil), GlobalQueueManager.tasks...)
	GlobalQueueManager.mu.Unlock()
	if len(remaining) != 1 || remaining[0] != queued {
		t.Fatalf("clear completed retained/removed the wrong tasks: %#v", remaining)
	}
}

func TestQueueWorkspaceSemanticCloseGuardAndTabIdentity(t *testing.T) {
	withSemanticQueueTestState(t)
	vtui.FrameManager.Push(vtui.NewDesktop())
	qf := NewQueueFrame()
	active := &QueueTask{ID: 31, Type: "Copy", Desc: "large copy", State: "Running"}
	// Deliberately leave QueueFrame's UI snapshot stale. Enqueue mutates the
	// authoritative manager before its posted UpdateTasks callback runs; a
	// semantic close in that interval must still see and protect this task.
	qf.UpdateTasks(nil)
	GlobalQueueManager = &OpQueueManager{
		tasks:      []*QueueTask{active},
		activeKeys: make(map[string]bool),
		frame:      qf,
	}
	vtui.FrameManager.AddScreenBackground(qf)
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

	active.mu.Lock()
	active.State = "Done"
	active.Progress = 100
	active.mu.Unlock()
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

func TestSplitQueueTimeSpeedTextRetainsETA(t *testing.T) {
	elapsed, eta, speed := splitQueueTimeSpeedText(
		"Elapsed: 00:01  Remaining: 00:02:03       12 MiB/s")
	if elapsed != "Elapsed: 00:01" || eta != "Remaining: 00:02:03" || speed != "12 MiB/s" {
		t.Fatalf("composite progress = (%q, %q, %q)", elapsed, eta, speed)
	}
	elapsed, eta, speed = splitQueueTimeSpeedText("3 files/s")
	if elapsed != "" || eta != "" || speed != "3 files/s" {
		t.Fatalf("plain speed = (%q, %q, %q)", elapsed, eta, speed)
	}
}
