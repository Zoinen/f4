package extui

import "testing"

func TestOperationsQueueModelToMap(t *testing.T) {
	scene := Scene{
		OperationsQueue: &OperationsQueueModel{
			ID: "queue", Title: "Operations Queue", Selected: 1, SelectedTaskID: 8,
			WorkspaceIndex: 2, WorkspaceNumber: 4, TabID: "workspace-tab-4",
			ActiveCount: 1, QueuedCount: 1, HasActive: true, CanClear: true,
			CancelText: "Cancel Task", ClearText: "Clear Done", EmptyText: "Nothing is running",
			DetailsText: "Show",
			Columns:     []OperationsQueueColumnModel{{ID: "state", Title: "State", Width: 10}},
			Items: []OperationsQueueItemModel{{
				ID: "queue-task-8", TaskID: 8, Index: 1, State: "Queued", StateClass: "queued",
				Description: "Copy files", DisplayText: "Copy files", Progress: 17,
				ETA: "Remaining: 00:00:03", Cancellable: true, Active: true,
			}},
		},
	}
	out := scene.ToMap()
	queue, ok := out["operationsQueue"].(M)
	if !ok || queue["kind"] != "operationsQueue" || queue["tabId"] != "workspace-tab-4" ||
		queue["selectedTaskId"] != 8 || queue["canClear"] != true {
		t.Fatalf("serialized queue = %#v", out["operationsQueue"])
	}
	columns := queue["columns"].([]M)
	if len(columns) != 1 || columns[0]["id"] != "state" || columns[0]["width"] != 10 {
		t.Fatalf("serialized columns = %#v", columns)
	}
	items := queue["items"].([]M)
	if len(items) != 1 || items[0]["id"] != "queue-task-8" || items[0]["stateClass"] != "queued" ||
		items[0]["eta"] != "Remaining: 00:00:03" || items[0]["cancellable"] != true {
		t.Fatalf("serialized items = %#v", items)
	}
}
