//go:build windows

package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsPTYIsBusyRefreshesAsynchronously(t *testing.T) {
	oldProbe := windowsPTYBusyProbe
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseProbe := func() { releaseOnce.Do(func() { close(release) }) }
	defer func() {
		releaseProbe()
		windowsPTYBusyProbe = oldProbe
	}()

	var calls atomic.Int32
	windowsPTYBusyProbe = func(windows.Handle, uint32) bool {
		if calls.Add(1) == 1 {
			close(entered)
		}
		<-release
		return true
	}

	pty := &PTY{process: &windows.ProcessInformation{
		Process:   windows.Handle(1),
		ProcessId: 4242,
	}}
	result := make(chan bool, 1)
	go func() { result <- pty.IsBusy() }()

	select {
	case busy := <-result:
		if busy {
			t.Fatal("first cache read unexpectedly reported busy")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("IsBusy blocked on the process snapshot refresh")
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("background busy-state probe did not start")
	}

	for range 5 {
		if pty.IsBusy() {
			t.Fatal("pending refresh should still return the cached state")
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("pending calls started %d probes, want one", got)
	}

	releaseProbe()
	deadline := time.Now().Add(time.Second)
	for {
		pty.mu.Lock()
		pending, state := pty.busyCheckPending, pty.lastBusyState
		pty.mu.Unlock()
		if !pending {
			if !state || !pty.IsBusy() {
				t.Fatal("completed refresh did not publish the busy state")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background busy-state refresh did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}
