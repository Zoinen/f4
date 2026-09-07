package main

import (
	"testing"
	"time"
)

func TestDialogProgressSchedulerCoalescesLatestState(t *testing.T) {
	var posted []func()
	var timers []func()
	var applied []dialogProgressUpdate

	scheduler := newDialogProgressScheduler(
		func(task func()) { posted = append(posted, task) },
		func(_ time.Duration, task func()) { timers = append(timers, task) },
		100*time.Millisecond,
		func(update dialogProgressUpdate) { applied = append(applied, update) },
	)

	scheduler.request(dialogProgressUpdate{kind: dialogProgressTransfer, filename: "first", totalPct: 1})
	scheduler.request(dialogProgressUpdate{kind: dialogProgressTransfer, filename: "latest", totalPct: 99})
	if len(posted) != 1 {
		t.Fatalf("got %d posted tasks before the first frame, want 1", len(posted))
	}

	posted[0]()
	if len(applied) != 1 || applied[0].filename != "latest" || applied[0].totalPct != 99 {
		t.Fatalf("first frame = %#v, want latest progress state", applied)
	}
	if len(timers) != 1 {
		t.Fatalf("got %d timers after the first frame, want 1", len(timers))
	}

	scheduler.request(dialogProgressUpdate{kind: dialogProgressTransfer, filename: "after interval", totalPct: 100})
	timers[0]()
	if len(posted) != 2 {
		t.Fatalf("got %d posted tasks after the interval, want 2", len(posted))
	}
	posted[1]()
	if len(applied) != 2 || applied[1].filename != "after interval" {
		t.Fatalf("second frame = %#v, want post-interval progress state", applied)
	}

	scheduler.stop()
	scheduler.request(dialogProgressUpdate{kind: dialogProgressTransfer, filename: "ignored"})
	if len(posted) != 2 {
		t.Fatalf("stop allowed another posted task: %d", len(posted))
	}
}
