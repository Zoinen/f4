package plughost

import (
	"errors"
	"runtime"
	"strconv"
	"sync/atomic"

	"github.com/unxed/vtui"
)

// The UI-thread deadlock (docs of f4#1686, "Known issues"): Host.InputBox and
// Host.Menu ask the UI goroutine to show a dialog and wait for the answer. When
// the UI goroutine is itself inside a call into the plugin - a VFS request, a
// key handler - and the plugin asks for a dialog while serving it, the UI
// goroutine waits for the plugin and the plugin waits for the UI goroutine.
// Nothing can answer, so f4 froze.
//
// uiGuard wraps a plugin's transport and counts the calls that were made from
// the UI goroutine and have not returned yet; a dialog request that arrives
// while there is one is refused with errUIBlocked instead of being queued
// behind a goroutine that will never get to it.

var errUIBlocked = errors.New("f4 is waiting for this plugin on its interface thread, so a dialog cannot be shown now: " +
	"ask from a command or a hotkey handler, not while serving a file-system request")

// uiGoroutine is the id of the goroutine that runs FrameManager's tasks, 0
// until it has been learned.
var (
	uiGoroutine      atomic.Int64
	uiGoroutineAsked atomic.Bool
)

// learnUIGoroutine has the UI goroutine report its own id, once: the tasks
// FrameManager runs are the one thing known to run on it.
func learnUIGoroutine() {
	if vtui.FrameManager == nil || !uiGoroutineAsked.CompareAndSwap(false, true) {
		return
	}
	vtui.FrameManager.PostTask(func() { uiGoroutine.Store(currentGoroutineID()) })
}

func currentGoroutineID() int64 {
	var buf [64]byte
	line := buf[:runtime.Stack(buf[:], false)]
	const prefix = "goroutine "
	if len(line) < len(prefix) {
		return 0
	}
	line = line[len(prefix):]
	end := 0
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	id, err := strconv.ParseInt(string(line[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func onUIGoroutine() bool {
	id := uiGoroutine.Load()
	return id != 0 && id == currentGoroutineID()
}

// uiGuard is a PluginTransport that remembers whether the UI goroutine is
// waiting for the plugin.
type uiGuard struct {
	inner   PluginTransport
	waiting atomic.Int32
}

func newUIGuard(inner PluginTransport) *uiGuard {
	learnUIGoroutine()
	return &uiGuard{inner: inner}
}

func (g *uiGuard) Call(method string, params any, result any) error {
	if onUIGoroutine() {
		g.waiting.Add(1)
		defer g.waiting.Add(-1)
	}
	return g.inner.Call(method, params, result)
}

// uiBlocked reports whether the UI goroutine is inside a call into the plugin.
func (g *uiGuard) uiBlocked() bool { return g.waiting.Load() > 0 }

// uiBlockedFor is uiBlocked for whatever transport a host method was given.
func uiBlockedFor(back PluginTransport) bool {
	g, ok := back.(*uiGuard)
	return ok && g.uiBlocked()
}
