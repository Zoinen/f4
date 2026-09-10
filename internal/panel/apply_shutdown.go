package panel

import (
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/terminal"
)

func CancelOperationsForShutdown() {
	CancelAllForegroundApplyCommands()
	if fileops.GlobalQueueManager != nil {
		fileops.GlobalQueueManager.CancelAll()
	}
	if terminal.GlobalBackgroundJobs != nil {
		terminal.GlobalBackgroundJobs.CancelAll()
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		queued, background := 0, 0
		if fileops.GlobalQueueManager != nil {
			queued = fileops.GlobalQueueManager.ActiveTasksCount()
		}
		if terminal.GlobalBackgroundJobs != nil {
			background = terminal.GlobalBackgroundJobs.ActiveCount()
		}
		if queued == 0 && background == 0 && activeForegroundApplyCommandCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cmdline.CleanupAllApplyCommandResources()
	// FUSE mounts created from the panels belong to this process, and the
	// kernel connection dies with it: leaving them up would strand a mount
	// point that hangs every program walking into it. Mounts started with
	// --daemon live in a process of their own and are not in this list.
	fusefs.UnmountAll()
}
