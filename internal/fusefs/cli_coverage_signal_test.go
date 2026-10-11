//go:build !windows && !plan9 && !js

package fusefs

import (
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Ctrl+C (or a plain SIGTERM, e.g. from a supervisor) on a foreground mount
// has to unmount it: otherwise the mount point outlives the process that
// owned it, and every program that walks into it after that hangs.
//
// This sends itself a real SIGTERM via syscall.Kill, which only exists on
// POSIX-ish platforms; that is why this test lives in a file carrying the
// same build tag as platform_unix.go rather than in cli_coverage_test.go.
func TestWaitOrSignalUnmountsOnSIGTERM(t *testing.T) {
	oldUnmount := unmountOwn
	release := make(chan struct{})
	var unmountCalled atomic.Bool
	unmountOwn = func(string) error {
		unmountCalled.Store(true)
		close(release) // let the fake wait() return, so waitOrSignal need not sit through its 5s grace period
		return nil
	}
	t.Cleanup(func() { unmountOwn = oldUnmount })

	wait := func() { <-release }

	finished := make(chan struct{})
	go func() {
		waitOrSignal(wait, "/mnt/does-not-matter")
		close(finished)
	}()

	// signal.Notify inside waitOrSignal runs synchronously right after the
	// goroutine starts; there is nothing external to poll for, so a short,
	// generous sleep is the pragmatic wait (the same tradeoff pty_test.go
	// makes for "give the OS a moment").
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("waitOrSignal did not return after SIGTERM")
	}
	if !unmountCalled.Load() {
		t.Fatal("SIGTERM must unmount a foreground mount")
	}
}
