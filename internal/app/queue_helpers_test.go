package app

import (
	"github.com/unxed/f4/internal/fileops"
	vtui "github.com/unxed/vtui"
	testing "testing"
)

func withSemanticQueueTestState(t *testing.T) {
	t.Helper()
	oldQueue := fileops.GlobalQueueManager
	t.Cleanup(func() {
		fileops.GlobalQueueManager = oldQueue
	})
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
}
