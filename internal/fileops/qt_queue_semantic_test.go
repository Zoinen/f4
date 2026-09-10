package fileops

import (
	"errors"
	"github.com/unxed/vtui"
	"testing"
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
	qf.Table.SelectPos = 1

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

func TestQueueDropdownPublishesAndControlsBackgroundQueue(t *testing.T) {
	withSemanticQueueTestState(t)
	panels := vtui.NewDesktop()
	vtui.FrameManager.Push(panels)
	GlobalQueueManager = &OpQueueManager{activeKeys: make(map[string]bool)}
	active := vtui.FrameManager.GetTopFrame()
	if !HandleQueueDropdownAction(map[string]any{"action": "queue.ensure"}) {
		t.Fatal("queue not created")
	}
	GlobalQueueManager.tasks = []*QueueTask{{ID: 1, State: "Done", Type: "Copy"}}
	GlobalQueueManager.RefreshUI()
	model := BackgroundOperationsQueue()
	if model == nil || len(model.Items) != 1 {
		t.Fatalf("missing background queue: %+v", model)
	}
	if !HandleQueueDropdownAction(map[string]any{"action": "queue.clearCompleted", "target": model.ID}) {
		t.Fatal("background clear not routed")
	}
	if len(GlobalQueueManager.tasks) != 0 {
		t.Fatal("completed task not cleared")
	}
	if vtui.FrameManager.GetTopFrame() != active {
		t.Fatal("dropdown switched workspace")
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
	queued.Mu.Lock()
	state := queued.State
	queued.Mu.Unlock()
	if state != "Queued" {
		t.Fatalf("cancel action bypassed confirmation, state = %q", state)
	}

	if !qf.HandleSemanticAction(map[string]any{"target": target, "action": "queue.clearCompleted"}) {
		t.Fatal("clear completed action was rejected")
	}
	GlobalQueueManager.Mu.Lock()
	remaining := append([]*QueueTask(nil), GlobalQueueManager.tasks...)
	GlobalQueueManager.Mu.Unlock()
	if len(remaining) != 1 || remaining[0] != queued {
		t.Fatalf("clear completed retained/removed the wrong tasks: %#v", remaining)
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

func TestQueueSemanticPauseTargetsOneOperation(t *testing.T) {
	withSemanticQueueTestState(t)
	qf := NewQueueFrame()
	tasks := []*QueueTask{{ID: 1, State: "Queued"}, {ID: 2, State: "Queued"}}
	GlobalQueueManager = &OpQueueManager{tasks: tasks, frame: qf}
	qf.UpdateTasks(tasks)
	action := map[string]any{"target": vtui.SemanticID(qf), "action": "queue.pause", "taskId": 2}
	if !qf.HandleSemanticAction(action) {
		t.Fatal("pause rejected")
	}
	model := qf.semanticModel()
	if tasks[0].State != "Queued" || model.Items[1].State != "Paused" || !model.Items[1].Resumable || model.Items[1].Pausable || !model.Items[1].Cancellable || model.CanClose {
		t.Fatalf("wrong paused model: %+v", model)
	}
	action["action"] = "queue.resume"
	if !qf.HandleSemanticAction(action) || tasks[1].State != "Queued" {
		t.Fatal("resume rejected")
	}
	action["taskId"] = 99
	if qf.HandleSemanticAction(action) {
		t.Fatal("stale task accepted")
	}
}
