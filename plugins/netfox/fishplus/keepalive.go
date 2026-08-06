package fishplus

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultKeepaliveInterval is how long a session may sit idle before the client
// sends a noop. A minute is well inside the usual NAT and sshd idle timeouts,
// and the noop costs one line in each direction.
const DefaultKeepaliveInterval = 60 * time.Second

// keepaliveTimeout bounds one keepalive round trip. A remote that has not
// answered a noop in this long is not going to, and waiting longer only delays
// the moment the user is told.
const keepaliveTimeout = 20 * time.Second

// Keepalive holds an idle session open. Nothing crosses the wire between two
// requests, so a panel left open while the user reads something else is a
// silent connection, and a silent connection is exactly what a NAT table drops
// and what sshd's ClientAliveInterval reaps. The failure is invisible from
// here: the next request fails, and the user learns about a connection that
// died an hour ago at the moment they wanted to use it.
//
// The loop only ever sends a noop, which is the cheapest round trip the
// protocol has, and it sends it through the session's own mutex like any other
// request — so it cannot interleave with one, and a busy session never sees it
// at all.
type Keepalive struct {
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
	pings int64
}

// StartKeepalive begins pinging c whenever it has been idle for every. A nil
// client or a non-positive interval means no keepalive at all, and every method
// of the nil result is safe, so a caller that does not want one does not need
// to branch around it.
func StartKeepalive(c *Client, every time.Duration) *Keepalive {
	if c == nil || every <= 0 {
		return nil
	}
	k := &Keepalive{stop: make(chan struct{}), done: make(chan struct{})}
	go k.run(c, every)
	return k
}

// Pings counts the round trips completed so far. It exists for the tests and
// for anyone asking why a session is or is not still alive.
func (k *Keepalive) Pings() int {
	if k == nil {
		return 0
	}
	return int(atomic.LoadInt64(&k.pings))
}

// Done is closed once the loop has left, whether because it was stopped or
// because the session it was watching is gone.
func (k *Keepalive) Done() <-chan struct{} {
	if k == nil {
		return nil
	}
	return k.done
}

// Stop asks the loop to leave and returns without waiting for it. Waiting
// would mean blocking whoever closes a panel for as long as an unanswered noop
// takes, and the caller closing the session is about to make that noop fail
// anyway.
func (k *Keepalive) Stop() {
	if k == nil {
		return
	}
	k.once.Do(func() { close(k.stop) })
}

func (k *Keepalive) run(c *Client, every time.Duration) {
	defer close(k.done)

	// Checking four times per interval keeps the ping close to the deadline it
	// is meant to beat without turning the loop into a busy one.
	tick := every / 4
	if tick <= 0 {
		tick = every
	}
	t := time.NewTicker(tick)
	defer t.Stop()

	for {
		select {
		case <-k.stop:
			return
		case <-t.C:
		}

		s := c.Session()
		if s.Broken() {
			// Close poisons the session too, so this covers both a session that
			// lost synchronization and one whose last user has gone.
			return
		}
		if s.IdleFor() < every {
			continue
		}
		select {
		case <-k.stop:
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), keepaliveTimeout)
		attempted, err := s.TryNoop(ctx)
		cancel()
		if !attempted && err == nil {
			// A request won the race after IdleFor. It is activity, not a
			// reason to queue a keepalive behind it.
			continue
		}
		if err != nil {
			// The point of the keepalive is to find this out now rather than
			// under the user's next request. Marking it is all that can be done
			// until step 14 grows a reconnect.
			s.MarkBroken()
			return
		}
		atomic.AddInt64(&k.pings, 1)
	}
}
