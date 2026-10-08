package editor

import (
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestSearchProgressFastCompletionNeverShowsPopup(t *testing.T) {
	ev := newFindAllEditor(t, "mp4")
	vtui.FrameManager.Push(ev)
	finished := false
	dlg, _ := RunSearchWithProgressOn(ev, "mp4", func(ctx *vtui.TaskContext, dlg *vtui.Window) {
		ctx.RunOnUI(func() { dlg.Close(); finished = true })
	})
	if vtui.FrameManager.GetTopFrame() != ev {
		t.Fatal("quick search immediately opened a modal progress popup")
	}
	pumpFindAll(t, func() bool { return finished })
	pumpFindAllFor(t, 200*time.Millisecond, func() (string, bool) {
		return "completed search showed a late progress popup", vtui.FrameManager.GetTopFrame() != ev
	})
	if !dlg.IsDone() {
		t.Fatal("search completion did not close progress")
	}
}

func TestSearchProgressSlowSearchStaysInOwningTabAndCancels(t *testing.T) {
	ev := newFindAllEditor(t, "mp4")
	vtui.FrameManager.Push(ev)
	owner := vtui.FrameManager.ActiveIdx
	stopped := make(chan struct{})
	dlg, ctx := RunSearchWithProgressOn(ev, "mp4", func(ctx *vtui.TaskContext, _ *vtui.Window) {
		<-ctx.Done()
		close(stopped)
	})
	vtui.FrameManager.AddScreen(vtui.NewDesktop())
	active := vtui.FrameManager.ActiveIdx
	pumpFindAll(t, func() bool {
		frames := vtui.FrameManager.GetActiveFrames(owner)
		return frames[len(frames)-1] == dlg
	})
	if vtui.FrameManager.ActiveIdx != active {
		t.Fatal("delayed progress stole the active workspace")
	}
	dlg.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("closing delayed progress did not stop the worker")
	}
	if ctx.Err() == nil {
		t.Fatal("closing delayed progress did not cancel the task")
	}
}

func TestSearchProgressQueuedShowDoesNotResurfaceAfterClose(t *testing.T) {
	ev := newFindAllEditor(t, "mp4")
	vtui.FrameManager.Push(ev)
	stopped := make(chan struct{})
	dlg, _ := RunSearchWithProgressOn(ev, "mp4", func(ctx *vtui.TaskContext, _ *vtui.Window) {
		<-ctx.Done()
		close(stopped)
	})
	// Capture the deadline callback without executing it, then close first.
	var show func()
	select {
	case show = <-vtui.FrameManager.TaskChan:
	case <-time.After(time.Second):
		t.Fatal("slow search did not queue progress")
	}
	dlg.Close()
	show()
	if vtui.FrameManager.GetTopFrame() != ev {
		t.Fatal("queued progress reopened a completed search")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}
