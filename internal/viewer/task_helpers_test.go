package viewer

import (
	vtui "github.com/unxed/vtui"
)

func drainFrameTasks() {
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			return
		}
	}
}
