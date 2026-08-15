package vtui

import (
	"fmt"
	"os"
	"runtime/debug"
)

// LogAndRepanic records the stack of a panic that is about to cross a
// goroutine boundary, and then lets it continue.
//
// It exists because of how gogpu calls us back. Draw and update callbacks run
// on a dedicated render thread; gogpu's internal/thread.(*Thread).CallVoid
// recovers whatever they panic with and re-panics that value on the calling
// goroutine. The value survives, the stack does not, so the crash report shows
// the re-panic site inside gogpu and says nothing at all about the fault. A
// nil dereference three frames deep in our own renderer looks exactly like a
// nil dereference in gogpu's threading code.
//
// Deferring this at the top of every callback that a foreign thread invokes
// costs nothing and puts the real stack in the debug log before the value is
// handed over.
//
// Use it as the deferred call itself — recover only works one frame deep:
//
//	defer LogAndRepanic("gogpu OnDraw")
func LogAndRepanic(where string) {
	r := recover()
	if r == nil {
		return
	}
	stack := debug.Stack()
	DebugLog("PANIC on a foreign thread in %s: %v\n%s", where, r, stack)
	fmt.Fprintf(os.Stderr, "PANIC on a foreign thread in %s: %v\n%s\n", where, r, stack)
	panic(r)
}
