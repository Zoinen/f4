package fileops

import (
	"os"
	"testing"
	"time"
)

// Enqueue reports "operation went to the background" from a goroutine that
// sleeps half a second first, and the toast it shows reads vtui.FrameManager.
// A test that enqueues, finishes inside that half second and then swaps the
// frame manager in its cleanup races that read — which is what QueueShowToast
// is a seam for. The application supplies the real toast; here it says nothing.
func TestMain(m *testing.M) {
	QueueShowToast = func(string, time.Duration) {}
	os.Exit(m.Run())
}
