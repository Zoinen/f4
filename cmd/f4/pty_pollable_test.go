//go:build darwin || linux || freebsd || dragonfly || netbsd || openbsd

package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

// TestPTYMasterPollable locks in the invariant the reader goroutine depends
// on: the master must be registered with Go's runtime poller. That only
// happens when the fd is already non-blocking at os.NewFile time. If it is
// wrapped first and flipped to non-blocking afterwards, os.File stays in
// raw-syscall mode and every Read returns EAGAIN immediately, which
// panels_frame's read loop treats as fatal — a permanently black console.
func TestPTYMasterPollable(t *testing.T) {
	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("close PTY: %v", err)
		}
	})

	// SetReadDeadline only works on poller-registered files.
	if err := p.Master.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("master is not registered with the runtime poller: %v", err)
	}

	// With no child attached nothing is writable, so a pollable master must
	// block until the deadline — not return EAGAIN instantly.
	buf := make([]byte, 1)
	_, err = p.Master.Read(buf)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("expected deadline-exceeded from pollable read, got: %v", err)
	}
	_ = p.Master.SetReadDeadline(time.Time{})
}
