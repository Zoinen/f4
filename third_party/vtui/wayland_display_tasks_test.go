//go:build linux && !android && (amd64 || arm64)

package vtui

import (
	"testing"

	"github.com/neurlang/wayland/wl"
)

// Toolkit calls queued from FrameManager's goroutine run in the present
// callback on the display goroutine, once each, in order.
func TestWaylandDisplayTasksRunInPresentCallback(t *testing.T) {
	h := &WaylandHost{}
	var ran []int
	h.mu.Lock()
	h.displayTasks = append(h.displayTasks, func() { ran = append(ran, 1) }, func() { ran = append(ran, 2) })
	h.mu.Unlock()
	h.present.request(func() error { return nil })

	h.HandleCallbackDone(wl.CallbackDoneEvent{})
	if len(ran) != 2 || ran[0] != 1 || ran[1] != 2 {
		t.Fatalf("tasks ran %v, want [1 2]", ran)
	}
	h.present.request(func() error { return nil })
	h.HandleCallbackDone(wl.CallbackDoneEvent{})
	if len(ran) != 2 {
		t.Fatalf("tasks ran again: %v", ran)
	}
}

// Once the host has closed, a queued task is dropped rather than run
// against a window that is going away.
func TestWaylandDisplayTasksDroppedAfterClose(t *testing.T) {
	h := &WaylandHost{}
	ran := false
	h.mu.Lock()
	h.displayTasks = append(h.displayTasks, func() { ran = true })
	h.mu.Unlock()
	h.present.request(func() error { return nil })
	h.Close()

	h.HandleCallbackDone(wl.CallbackDoneEvent{})
	if ran {
		t.Fatal("task ran after Close")
	}
	var _ windowMaximizer = (*WaylandRenderer)(nil)
}
