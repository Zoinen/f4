package fishplus

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestCancelledRequestKeepsTheSession is the whole point of the step: an
// impatient Escape used to cost a reconnect, because the client stopped
// reading in the middle of an answer and had no way of knowing where the
// next one began. The terminator is that way.
func TestCancelledRequestKeepsTheSession(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64", func(w io.Writer, token string, req mockRequest) {
		if _, err := fmt.Fprintf(w, "one line of an answer nobody is waiting for any more\n"); err != nil {
			t.Errorf("write canceled response: %v", err)
			return
		}
		if _, err := fmt.Fprintf(w, ".%s %s ok\n", token, req.ID); err != nil {
			t.Errorf("write canceled response terminator: %v", err)
		}
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
		if _, err := fmt.Fprintf(w, "S 300\n#%d\n", len(payload)); err != nil {
			t.Errorf("write canceled frame header: %v", err)
			return
		}
		if _, err := w.Write(payload); err != nil {
			t.Errorf("write canceled frame payload: %v", err)
			return
		}
		if _, err := fmt.Fprintf(w, ".%s %s ok\n", token, req.ID); err != nil {
			t.Errorf("write canceled frame terminator: %v", err)
		}
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

	err := sess.drainToTerminator(context.Background(), ".sometoken 1 ", false)
	closeErr := pw.Close()
	<-done
	if closeErr != nil {
		t.Fatalf("close endless response pipe: %v", closeErr)
	}
	if err == nil {
		t.Fatal("draining an answer that never ends reported success")
	}
}

// cancelCloser unblocks every pending write the moment it is closed, which is
// how a real transport (net.Conn/ShellStream) interrupts a blocked write when
// the session tears down.
type cancelCloser struct {
	ch   chan struct{}
	once sync.Once
}

func (c *cancelCloser) Close() error {
	c.once.Do(func() { close(c.ch) })
	return nil
}

// stagedBlockingWriter lets the first write(s) through and then blocks until
// the closer fires, modelling a patch body stuck on a frozen transport.
type stagedBlockingWriter struct {
	unblock chan struct{}
	mu      sync.Mutex
	calls   int
}

func (w *stagedBlockingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.calls++
	c := w.calls
	w.mu.Unlock()
	if c <= 1 {
		return len(p), nil
	}
	<-w.unblock
	return len(p), io.ErrClosedPipe
}

// TestCancelDuringInProgressReadKeepsSession is the case the older cancel
// tests never exercised: the context is cancelled while the client is already
// blocked reading the answer, not before the request went out. The session
// must survive (the peer is responsive, so the answer is drained to its
// terminator) and stay usable for the next request.
func TestCancelDuringInProgressReadKeepsSession(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64", func(w io.Writer, token string, req mockRequest) {
		// Answer only after a delay, so the client is already mid-read when
		// the cancellation lands.
		time.Sleep(50 * time.Millisecond)
		if _, err := fmt.Fprintf(w, ".%s %s ok\n", token, req.ID); err != nil {
			t.Errorf("write answer: %v", err)
		}
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond) // cancel while the read is in flight
		cancel()
	}()
	if _, err := sess.Exec(ctx, "noop"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Exec with a mid-read cancel = %v, want context.Canceled", err)
	}
	if sess.Broken() {
		t.Fatal("cancelling a request mid-read broke the session")
	}

	// The delayed peer answered; the next request must reuse the session.
	resp, err := sess.Exec(context.Background(), "noop")
	if err != nil {
		t.Fatalf("post-cancel request failed: %v", err)
	}
	if !resp.OK() {
		t.Errorf("post-cancel request answered %+v", resp)
	}
}

// TestExecStreamBodyWriteHonorsCancellation covers the main scenario the
// original code still left hanging: ExecStream hands the raw transport writer
// to the patch body callback, so a frozen connection could block the write
// forever. The body must now go through the cancellable single-owner write
// path, so a cancellation closes the transport and returns ErrBroken instead
// of hanging.
func TestExecStreamBodyWriteHonorsCancellation(t *testing.T) {
	unblock := make(chan struct{})
	closer := &cancelCloser{ch: unblock}
	body := &stagedBlockingWriter{unblock: unblock}
	sess := NewSession(body, strings.NewReader(""), closer)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := sess.ExecStream(ctx, "patch", []string{"/f"}, nil, func(bw io.Writer) error {
			_, e := bw.Write([]byte("a patch body that must never hang on a frozen transport"))
			return e
		})
		done <- err
	}()
	// Let the request line flush and the body write block.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, ErrBroken) {
			t.Fatalf("ExecStream with a hung body write = %v, want ErrBroken", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecStream with a hung body write hung instead of returning")
	}
	if !sess.Broken() {
		t.Fatal("a session whose body write was cancelled was left reusable")
	}
}
