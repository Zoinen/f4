package main

import "time"

func cancelOperationsForShutdown() {
	cancelAllForegroundApplyCommands()
	if GlobalQueueManager != nil {
		GlobalQueueManager.CancelAll()
	}
	if GlobalBackgroundJobs != nil {
		GlobalBackgroundJobs.CancelAll()
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		queued, background := 0, 0
		if GlobalQueueManager != nil {
			queued = GlobalQueueManager.ActiveTasksCount()
		}
		if GlobalBackgroundJobs != nil {
			background = GlobalBackgroundJobs.ActiveCount()
		}
		if queued == 0 && background == 0 && activeForegroundApplyCommandCount() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cleanupAllApplyCommandResources()
}
