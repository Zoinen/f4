package main

import (
	"time"

	"github.com/unxed/vtui"
)

// IndexPhase is where the line index stands. The editor can work with an index
// that is still filling — it just cannot answer questions about lines it has
// not reached yet — so the phase is what tells the difference between "there is
// no more to find" and "there is more, we are looking".
type IndexPhase int

const (
	// IndexIdle: nothing has been scanned and nothing is scanning.
	IndexIdle IndexPhase = iota
	// IndexScanning: a scan is running and the index is still growing.
	IndexScanning
	// IndexComplete: the index describes every line of the buffer.
	IndexComplete
	// IndexFailed: the scan stopped on an error and the index is short.
	IndexFailed
)

func (p IndexPhase) String() string {
	switch p {
	case IndexScanning:
		return "scanning"
	case IndexComplete:
		return "complete"
	case IndexFailed:
		return "failed"
	default:
		return "idle"
	}
}

// IndexStatus is what the editor knows about its line index right now. Lines is
// how many are indexed, which equals the file's line count only once the phase
// is IndexComplete — anything reporting a total before then is guessing.
type IndexStatus struct {
	Phase   IndexPhase
	Scanned int64
	Total   int64
	Lines   int
	Err     error
}

// Percent is how far the scan has read, for anything that wants to show it.
func (s IndexStatus) Percent() int {
	if s.Total <= 0 {
		return 100
	}
	if s.Scanned >= s.Total {
		return 100
	}
	return int(s.Scanned * 100 / s.Total)
}

// indexNotifyInterval throttles progress updates. A scan reports every batch it
// applies, which on a fast local file is hundreds a second; subscribers exist
// to draw a percentage, and a percentage does not need drawing that often.
const indexNotifyInterval = 100 * time.Millisecond

// IndexState returns the current status. It reads UI-thread state and is meant
// for the paint path, which is where a status line asks.
func (ev *EditorView) IndexState() IndexStatus {
	return ev.indexStatus
}

// SubscribeIndex registers a callback for index progress. It fires on the UI
// thread, throttled while scanning but never for a phase change, so a listener
// waiting for completion hears about it immediately. The returned function
// unsubscribes.
func (ev *EditorView) SubscribeIndex(fn func(IndexStatus)) func() {
	if fn == nil {
		return func() {}
	}
	ev.indexSubID++
	id := ev.indexSubID
	if ev.indexSubs == nil {
		ev.indexSubs = make(map[int]func(IndexStatus))
	}
	ev.indexSubs[id] = fn
	return func() { delete(ev.indexSubs, id) }
}

// setIndexStatus records a new status and tells the subscribers, on the UI
// thread. Progress within a phase is throttled; a change of phase is not.
func (ev *EditorView) setIndexStatus(s IndexStatus) {
	phaseChanged := s.Phase != ev.indexStatus.Phase
	ev.indexStatus = s

	if !phaseChanged && time.Since(ev.indexNotifiedAt) < indexNotifyInterval {
		return
	}
	ev.indexNotifiedAt = time.Now()
	for _, fn := range ev.indexSubs {
		fn(s)
	}
	if phaseChanged {
		vtui.FrameManager.Redraw()
	}
}

// noteIndexProgress is what the scan reports through: how far it has read and
// how many lines that came to. It keeps the phase as it is.
//
// Removed once as dead code, correctly at the time: the scan updated its
// status only at start and finish, so the status line showed 0% for the whole
// of a scan — which on a file big enough to watch reads as a scan that is
// stuck rather than one that is working. The batches call it now.
func (ev *EditorView) noteIndexProgress(scanned int64, lines int) {
	s := ev.indexStatus
	s.Scanned = scanned
	s.Lines = lines
	ev.setIndexStatus(s)
}

// indexIsComplete reports whether the index describes the whole buffer, which
// is the question every caller that needs a line number for a far-away offset
// actually wants answered.
func (ev *EditorView) indexIsComplete() bool {
	return ev.indexStatus.Phase == IndexComplete
}
