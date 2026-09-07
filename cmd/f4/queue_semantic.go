package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func queueSemanticStateClass(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "queued":
		return "queued"
	case "starting":
		return "starting"
	case "scanning":
		return "scanning"
	case "running":
		return "running"
	case "cancelling":
		return "cancelling"
	case "done":
		return "completed"
	case "error":
		return "error"
	case "cancelled":
		return "cancelled"
	case "paused":
		// No built-in queue action creates this state, but retaining its
		// semantic meaning lets a task supplied by a plugin render honestly.
		return "paused"
	default:
		return "unknown"
	}
}

func (qf *QueueFrame) semanticModel() extui.OperationsQueueModel {
	model := extui.OperationsQueueModel{
		ID:          vtui.SemanticID(qf),
		Title:       strings.TrimSpace(qf.GetTitle()),
		Selected:    qf.table.SelectPos,
		Top:         qf.table.TopPos,
		CancelText:  Msg("Queue.BtnCancel"),
		ClearText:   Msg("Queue.BtnClear"),
		EmptyText:   Msg("Jobs.Empty"),
		DetailsText: Msg("Jobs.BtnShow"),
	}

	columnIDs := []string{"id", "state", "type", "description", "progress", "speed"}
	for index, column := range qf.table.Columns {
		id := fmt.Sprintf("column-%d", index)
		if index < len(columnIDs) {
			id = columnIDs[index]
		}
		alignment := "left"
		if column.Alignment == vtui.AlignRight {
			alignment = "right"
		} else if column.Alignment == vtui.AlignCenter {
			alignment = "center"
		}
		model.Columns = append(model.Columns, extui.OperationsQueueColumnModel{
			ID:        id,
			Title:     column.Title,
			Width:     column.Width,
			Alignment: alignment,
		})
	}

	tasks := qf.authoritativeTasksSnapshot()
	model.Items = make([]extui.OperationsQueueItemModel, 0, len(tasks))
	for index, task := range tasks {
		task.mu.Lock()
		item := extui.OperationsQueueItemModel{
			ID:              fmt.Sprintf("queue-task-%d", task.ID),
			TaskID:          task.ID,
			Index:           index,
			Type:            task.Type,
			Description:     task.Desc,
			State:           task.State,
			StateClass:      queueSemanticStateClass(task.State),
			Action:          task.Action,
			CurrentFile:     task.CurrentFile,
			CurrentProgress: task.CurrentProgress,
			Progress:        task.Progress,
			TotalText:       task.TotalText,
			Elapsed:         task.Elapsed,
			ETA:             task.ETA,
			Speed:           task.Speed,
			Cancellable:     queueTaskCancellable(task.State),
			Terminal:        queueTaskTerminal(task.State),
			Active:          queueTaskActive(task.State),
			CancelPrompt:    fmt.Sprintf("Cancel task ID %d?", task.ID),
		}
		if task.ErrorMsg != nil {
			item.Error = task.ErrorMsg.Error()
		}
		item.HasDetails = task.OpenDetails != nil || (task.State == "Error" && task.ErrorMsg != nil)
		if task.State == "Running" || task.State == "Scanning" || task.State == "Cancelling" {
			item.DisplayText = task.CurrentFile
		} else {
			item.DisplayText = task.Desc
		}
		task.mu.Unlock()

		if item.Active {
			model.ActiveCount++
		}
		switch item.State {
		case "Queued":
			model.QueuedCount++
		case "Starting", "Scanning", "Running", "Cancelling":
			model.RunningCount++
		case "Done":
			model.CompletedCount++
		case "Error":
			model.ErrorCount++
		case "Cancelled":
			model.CancelledCount++
		}
		model.Items = append(model.Items, item)
	}

	if model.Selected >= 0 && model.Selected < len(model.Items) {
		model.SelectedTaskID = model.Items[model.Selected].TaskID
	} else if len(model.Items) == 0 {
		model.Selected = -1
	}
	model.HasActive = model.ActiveCount > 0
	model.CanClear = model.CompletedCount+model.ErrorCount+model.CancelledCount > 0
	model.CanClose = !model.HasActive
	return model
}

func (qf *QueueFrame) SemanticNode(_ *vtui.SemanticContext) map[string]any {
	return qf.semanticModel().ToMap()
}

func (qf *QueueFrame) semanticTaskIndex(action map[string]any) (int, bool) {
	index := -1
	if rawID, present := action["taskId"]; present {
		taskID := semanticInt(rawID)
		for candidate, task := range qf.tasks {
			task.mu.Lock()
			matches := task.ID == taskID
			task.mu.Unlock()
			if matches {
				index = candidate
				break
			}
		}
		if index < 0 {
			return 0, false
		}
	}
	if rawIndex, present := action["index"]; present {
		candidate := semanticInt(rawIndex)
		if candidate < 0 || candidate >= len(qf.tasks) || (index >= 0 && candidate != index) {
			return 0, false
		}
		index = candidate
	}
	return index, index >= 0
}

func (qf *QueueFrame) selectSemanticTask(index int) bool {
	if index < 0 || index >= len(qf.tasks) {
		return false
	}
	qf.table.SelectPos = index
	qf.table.EnsureVisible()
	return true
}

func (qf *QueueFrame) hasActiveTasks() bool {
	for _, task := range qf.authoritativeTasksSnapshot() {
		task.mu.Lock()
		active := queueTaskActive(task.State)
		task.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

// authoritativeTasksSnapshot closes the small Enqueue -> UI refresh window:
// the manager already owns a new Queued task while QueueFrame.tasks can still
// contain the previous table rows.  TUI close validation reads the manager;
// semantic state and close validation must use the same source of truth.
func (qf *QueueFrame) authoritativeTasksSnapshot() []*QueueTask {
	if GlobalQueueManager != nil {
		GlobalQueueManager.mu.Lock()
		if GlobalQueueManager.frame == qf {
			tasks := append([]*QueueTask(nil), GlobalQueueManager.tasks...)
			GlobalQueueManager.mu.Unlock()
			return tasks
		}
		GlobalQueueManager.mu.Unlock()
	}
	return append([]*QueueTask(nil), qf.tasks...)
}

// handleOperationsQueueWorkspaceClose intercepts the generic workspace close
// path because vtui's semantic workspace.close removes a screen directly and
// therefore cannot call QueueFrame.ProcessKey.  claimed is true whenever the
// request identifies a queue workspace, including a mismatched/stale request
// that must be rejected rather than delegated to the generic closer.
func handleOperationsQueueWorkspaceClose(action map[string]any) (handled, claimed bool) {
	if vtui.FrameManager == nil || action == nil {
		return false, false
	}
	actionName := semanticString(action["action"])
	target := semanticString(action["target"])
	if actionName != "queue.close" && actionName != "workspace.close" && actionName != "tab.close" &&
		!(actionName == "close" && strings.HasPrefix(target, "workspace-tab-")) {
		return false, false
	}

	queueAt := make(map[int]*QueueFrame)
	targetIndex := -1
	for screenIndex, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if queue, ok := frame.(*QueueFrame); ok {
				queueAt[screenIndex] = queue
				if target == vtui.SemanticID(queue) {
					targetIndex = screenIndex
				}
			}
		}
		if target == fmt.Sprintf("workspace-tab-%d", screen.Number) {
			targetIndex = screenIndex
		}
	}

	requestedIndex := -1
	indexPresent := false
	if raw, present := action["index"]; present {
		requestedIndex = semanticInt(raw)
		indexPresent = true
	}
	if targetIndex >= 0 && indexPresent && requestedIndex != targetIndex {
		if queueAt[targetIndex] != nil || queueAt[requestedIndex] != nil {
			return false, true
		}
		return false, false
	}
	index := targetIndex
	if index < 0 && indexPresent {
		index = requestedIndex
	}
	queue := queueAt[index]
	if queue == nil {
		return false, false
	}
	if queue.hasActiveTasks() {
		vtui.ShowToast("Cannot close queue while operations are active. Use Ctrl+Tab to switch.", 3*time.Second)
		return true, true
	}

	if GlobalQueueManager != nil {
		GlobalQueueManager.mu.Lock()
		if GlobalQueueManager.frame == queue {
			GlobalQueueManager.frame = nil
		}
		GlobalQueueManager.mu.Unlock()
	}
	if len(vtui.FrameManager.Screens) > 1 {
		vtui.FrameManager.CloseScreen(index)
	} else {
		queue.Close()
	}
	return true, true
}

func (qf *QueueFrame) HandleSemanticAction(action map[string]any) bool {
	if action == nil || semanticString(action["target"]) != vtui.SemanticID(qf) {
		return false
	}
	switch semanticString(action["action"]) {
	case "queue.select":
		index, ok := qf.semanticTaskIndex(action)
		return ok && qf.selectSemanticTask(index)
	case "queue.activate":
		index, ok := qf.semanticTaskIndex(action)
		if !ok || !qf.selectSemanticTask(index) {
			return false
		}
		qf.openTaskDetails(index)
		return true
	case "queue.cancel":
		index, ok := qf.semanticTaskIndex(action)
		if !ok || !qf.selectSemanticTask(index) {
			return false
		}
		return qf.requestCancelTask(index)
	case "queue.clearCompleted":
		if _, hasTaskID := action["taskId"]; hasTaskID {
			return false
		}
		if GlobalQueueManager == nil {
			return false
		}
		GlobalQueueManager.ClearCompleted()
		return true
	case "queue.close":
		if qf.hasActiveTasks() {
			vtui.ShowToast("Cannot close queue while operations are active. Use Ctrl+Tab to switch.", 3*time.Second)
			return true
		}
		qf.Close()
		return true
	}
	return false
}
