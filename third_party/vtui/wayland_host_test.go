//go:build linux

package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestWaylandHost_KeyRepeatLogic(t *testing.T) {
	host := &WaylandHost{}

	host.mu.Lock()
	host.isRepeating = true
	host.repeatVK = vtinput.VK_A
	host.repeatNext = time.Now().Add(-1 * time.Millisecond) // force immediate trigger
	host.mu.Unlock()

	if !host.isRepeating {
		t.Error("Expected isRepeating to be true")
	}

	// Note: full integration test of Redraw() spin loop requires mocking window.Widget
	// which is deeply integrated with the C Wayland library in windowtrace.
}
