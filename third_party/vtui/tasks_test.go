package vtui

import (
	"testing"
	"time"
)

func TestRunAsync_TaskExecutionAndCancellation(t *testing.T) {
	// Setup isolated FrameManager for the test
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// 1. Test Execution via RunOnUI
	done := make(chan bool, 1)
	ctx := RunAsync(func(c *TaskContext) {
		c.RunOnUI(func() {
			done <- true
		})
	})

	// Simulate main loop extracting the task
	select {
	case task := <-fm.TaskChan:
		task() // Execute the safe UI closure
	case <-time.After(1 * time.Second):
		t.Fatal("Task was not pushed to TaskChan")
	}

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("RunOnUI task was not fully executed")
	}

	// 2. Test Cancellation
	ctx.Cancel()
	if ctx.Err() == nil {
		t.Error("TaskContext should report an error after Cancel() is called")
	}
}

func TestRunAsync_RedrawDecisionUsesCapturedManager(t *testing.T) {
	target := NewFrameManager()
	target.Init(NewSilentScreenBuf())
	defer target.Shutdown()
	other := NewFrameManager()
	other.Init(NewSilentScreenBuf())
	defer other.Shutdown()

	oldFrameManager := FrameManager
	FrameManager = target
	defer func() { FrameManager = oldFrameManager }()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	RunAsync(func(ctx *TaskContext) {
		close(started)
		<-release
		ctx.RunOnUIWithRedrawDecision(func() bool {
			close(done)
			return false
		})
	})
	<-started
	FrameManager = other
	close(release)

	select {
	case task := <-target.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("redraw-decision task was not posted to its captured manager")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("redraw-decision task did not execute")
	}
	select {
	case <-other.TaskChan:
		t.Fatal("redraw-decision task was posted to the replacement manager")
	default:
	}
}
