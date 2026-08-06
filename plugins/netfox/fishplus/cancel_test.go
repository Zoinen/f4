package fishplus

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

// TestCancelledRequestKeepsTheSession is the whole point of the step: an
// impatient Escape used to cost a reconnect, because the client stopped
// reading in the middle of an answer and had no way of knowing where the
// next one began. The terminator is that way.
func TestCancelledRequestKeepsTheSession(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64", func(w io.Writer, token string, req mockRequest) {
		fmt.Fprintf(w, "one line of an answer nobody is waiting for any more\n")
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
	}, 0)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sess.Exec(cancelled, "noop"); err != context.Canceled {
		t.Fatalf("a cancelled request returned %v, want context.Canceled", err)
	}
	if sess.Broken() {
		t.Fatal("a cancelled request broke the session")
	}

	// The next request has to get its own answer, not the tail of the one
	// that was abandoned.
	resp, err := sess.Exec(context.Background(), "noop")
	if err != nil {
		t.Fatalf("the request after a cancellation failed: %v", err)
	}
	if !resp.OK() {
		t.Errorf("the request after a cancellation answered %+v", resp)
	}
	if len(resp.Lines) != 1 {
		t.Errorf("the answer carried %d lines, want the one line of its own", len(resp.Lines))
	}
}

// TestCancelledRequestDrainsItsFrames checks the same thing for an answer
// carrying binary payload, where a discarded byte that happens to be a
// newline would otherwise look like a line boundary.
func TestCancelledRequestDrainsItsFrames(t *testing.T) {
	payload := make([]byte, 300)
	for i := range payload {
		payload[i] = '\n' // the worst possible payload for a line reader
	}
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64", func(w io.Writer, token string, req mockRequest) {
		fmt.Fprintf(w, "S 300\n#%d\n", len(payload))
		w.Write(payload)
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
	}, 1)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sess.ExecPathData(cancelled, "read", "/x", "0", "0"); err != context.Canceled {
		t.Fatal("a cancelled data request did not report being cancelled")
	}
	if sess.Broken() {
		t.Fatal("a cancelled data request broke the session")
	}
	resp, err := sess.ExecPathData(context.Background(), "read", "/x", "0", "0")
	if err != nil {
		t.Fatalf("the request after a cancellation failed: %v", err)
	}
	if len(resp.Data) != len(payload) {
		t.Errorf("got %d bytes, want %d: the drain left part of a frame behind", len(resp.Data), len(payload))
	}
}

// TestDrainGivesUpEventually covers the other half: a remote that never
// finishes the answer cannot be waited for forever, and past the deadline
// the session is worth less than the wait.
//
// It calls the drain directly. Going through a cancelled request would also
// depend on how far such a request gets before anybody looks at the context,
// which is a different question from the one being asked here.
func TestDrainGivesUpEventually(t *testing.T) {
	old := DrainAfterCancelTimeout
	DrainAfterCancelTimeout = 20 * time.Millisecond
	defer func() { DrainAfterCancelTimeout = old }()

	pr, pw := io.Pipe()
	sess := NewSession(io.Discard, pr, nil)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			// Lines forever, and never a terminator.
			if _, err := io.WriteString(pw, "still working on it\n"); err != nil {
				return
			}
		}
	}()

	err := sess.drainToTerminator(".sometoken 1 ", false)
	pw.Close()
	<-done
	if err == nil {
		t.Fatal("draining an answer that never ends reported success")
	}
}
