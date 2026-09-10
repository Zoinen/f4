package panel

import (
	vtui "github.com/unxed/vtui"
	testing "testing"
	time "time"
)

func waitForTransferDialog(t *testing.T, title string) vtui.Container {
	t.Helper()
	// Check if it's already on top and not closed
	top := vtui.FrameManager.GetTopFrame()
	if top != nil && top.GetTitle() == title && !top.IsDone() {
		return top.(vtui.Container)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			// Only return active dialogs, ignore stale closed ones
			if top != nil && top.GetTitle() == title && !top.IsDone() {
				return top.(vtui.Container)
			}
		case <-timeout:
			t.Fatalf("Timeout waiting for dialog %q", title)
		}
	}
}
