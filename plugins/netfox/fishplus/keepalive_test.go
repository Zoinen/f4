package fishplus

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

// TestKeepaliveLeavesABrokenSessionAlone: a session that lost synchronization
// must not be written to at all, keepalive included, or the noop lands in the
// middle of somebody else's answer.
func TestKeepaliveLeavesABrokenSessionAlone(t *testing.T) {
	sess := NewSession(io.Discard, strings.NewReader(""), nil)
	sess.MarkBroken()

	k := StartKeepalive(NewClient(sess), 10*time.Millisecond)
	select {
	case <-k.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the keepalive kept running on a broken session")
	}
	if k.Pings() != 0 {
		t.Fatalf("a broken session was pinged %d times", k.Pings())
	}
}

// TestKeepaliveStopIsSafeTwice covers the teardown path: a panel may be closed
// by both its own teardown and the frame that owned it, so Stop is called more
// than once on the same keepalive.
func TestKeepaliveStopIsSafeTwice(t *testing.T) {
	sess := NewSession(io.Discard, strings.NewReader(""), nil)
	k := StartKeepalive(NewClient(sess), time.Hour)

	k.Stop()
	k.Stop()

	select {
	case <-k.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not end the keepalive loop")
	}
}

// TestKeepaliveNilIsInert keeps the "no keepalive" case free of branching at
// every call site.
func TestKeepaliveNilIsInert(t *testing.T) {
	var k *Keepalive
	k.Stop()
	if k.Pings() != 0 {
		t.Fatal("a nil keepalive reported pings")
	}
	if k.Done() != nil {
		t.Fatal("a nil keepalive handed out a channel")
	}
	if StartKeepalive(nil, time.Second) != nil {
		t.Fatal("a nil client got a keepalive")
	}
	if StartKeepalive(NewClient(NewSession(io.Discard, strings.NewReader(""), nil)), 0) != nil {
		t.Fatal("a zero interval got a keepalive")
	}
}

// TestKeepalivePingsAnIdleSessionAgainstLocalShell drives the real helper. The
// property that matters is not that the noop was sent but that the session is
// still usable afterwards: a keepalive that desynchronized the stream would be
// worse than no keepalive at all, and only a request issued after it can show
// that it did not.
func TestKeepalivePingsAnIdleSessionAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)

	k := StartKeepalive(c, 30*time.Millisecond)
	defer k.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for k.Pings() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the keepalive never reached the remote helper")
		}
		time.Sleep(5 * time.Millisecond)
	}
	k.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got, err := c.Session().Ping(ctx, "after-keepalive")
	if err != nil {
		t.Fatalf("session unusable after a keepalive: %v", err)
	}
	if got != "after-keepalive" {
		t.Fatalf("ping answered %q, want the payload back", got)
	}
}

// TestSessionIdleForCountsFromTheLastRequest is the clock the keepalive reads.
func TestSessionIdleForCountsFromTheLastRequest(t *testing.T) {
	c := newLocalShellClient(t)
	s := c.Session()

	time.Sleep(40 * time.Millisecond)
	if s.IdleFor() < 30*time.Millisecond {
		t.Fatalf("idle for %v after doing nothing for 40ms", s.IdleFor())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Noop(ctx); err != nil {
		t.Fatalf("noop: %v", err)
	}
	if s.IdleFor() > 30*time.Millisecond {
		t.Fatalf("idle for %v right after a request", s.IdleFor())
	}
}
