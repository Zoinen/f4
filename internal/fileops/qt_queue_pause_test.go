package fileops

import (
	"context"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

func TestQueuePauseCheckpointResumeAndCancel(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := &QueueTask{ID: 1, State: "Running", ctx: ctx, cancel: cancel}
	qm := &OpQueueManager{tasks: []*QueueTask{task}}
	old := GlobalQueueManager
	GlobalQueueManager = qm
	defer func() { GlobalQueueManager = old }()
	for _, cancelPaused := range []bool{false, true} {
		if !qm.SetPaused(1, true) {
			t.Fatal("pause rejected")
		}
		done := make(chan bool, 1)
		go func() { done <- task.IsCancelled() }()
		deadline := time.Now().Add(time.Second)
		for {
			task.Mu.Lock()
			state := task.State
			task.Mu.Unlock()
			if state == "Paused" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("no pause acknowledgement")
			}
			time.Sleep(time.Millisecond)
		}
		other := &QueueTask{State: "Running"}
		if other.IsCancelled() {
			t.Fatal("another operation was affected")
		}
		select {
		case <-done:
			t.Fatal("worker advanced while paused")
		case <-time.After(20 * time.Millisecond):
		}
		if qm.ClearCompleted() != 0 || qm.ActiveTasksCount() != 1 {
			t.Fatal("paused work lost its active reservation")
		}
		if cancelPaused {
			if !qm.Cancel(1) {
				t.Fatal("cancel rejected")
			}
		} else if !qm.SetPaused(1, false) {
			t.Fatal("resume rejected")
		}
		select {
		case cancelled := <-done:
			if cancelled != cancelPaused {
				t.Fatal("wrong cancellation result")
			}
		case <-time.After(time.Second):
			t.Fatal("worker remained blocked")
		}
	}
}

func TestQueuePausedBeforeDispatchResumesQueued(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	task := &QueueTask{ID: 7, State: "Queued"}
	qm := &OpQueueManager{tasks: []*QueueTask{task}}
	if !qm.SetPaused(7, true) || task.State != "Paused" {
		t.Fatal("queued pause failed")
	}
	if !qm.SetPaused(7, false) || task.State != "Queued" {
		t.Fatal("queued resume failed")
	}
	task.State = "Done"
	if qm.SetPaused(7, true) {
		t.Fatal("terminal task paused")
	}
}
