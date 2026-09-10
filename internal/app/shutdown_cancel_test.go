package app

import (
	"context"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
	"testing"
)

// The subject is panel.CancelOperationsForShutdown, which lives here: the test walks
// a queued operation and a background job through one shutdown and asks that
// both were cancelled. It reaches two packages to do that, which is why it
// cannot sit inside either.

func TestCancelOperationsForShutdownCancelsQueueAndBackgroundJobs(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	originalQueue := fileops.GlobalQueueManager
	originalJobs := terminal.GlobalBackgroundJobs
	defer func() {
		fileops.GlobalQueueManager = originalQueue
		terminal.GlobalBackgroundJobs = originalJobs
	}()

	ctx, cancel := context.WithCancel(context.Background())
	queued := fileops.NewQueueTaskWithCancel(1, "Queued", ctx, cancel)
	fileops.GlobalQueueManager = fileops.NewQueueManagerWithTasks(queued)
	terminal.GlobalBackgroundJobs = terminal.NewBackgroundJobRegistry()
	backgroundCancelled := false
	var background *terminal.BackgroundJob
	background = terminal.GlobalBackgroundJobs.Start("test job", func() {
		backgroundCancelled = true
		background.Finish()
	})

	panel.CancelOperationsForShutdown()

	state, _, _ := queued.Status()
	if state != "Cancelled" || ctx.Err() != context.Canceled {
		t.Fatalf("queued task after shutdown cancellation: state=%q context=%v", state, ctx.Err())
	}
	if !backgroundCancelled {
		t.Fatal("background-job registry was not cancelled during shutdown")
	}
}
