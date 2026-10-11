package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/ios/internal/afcproto"
)

// TestAFCSessionRetainReleaseIdleState exercises the reference-counting used
// by afcRegistry to decide which sessions are candidates for eviction: refs
// track live borrowers, and idleAt records when the last borrower let go.
func TestAFCSessionRetainReleaseIdleState(t *testing.T) {
	session := newAFCSession("retain", func(context.Context) (io.ReadWriteCloser, error) {
		return &poolTestConnection{}, nil
	})

	if refs, idleAt := session.idleState(); refs != 1 || !idleAt.IsZero() {
		t.Fatalf("initial idleState = (%d, %v), want (1, zero)", refs, idleAt)
	}

	if !session.retain() {
		t.Fatal("retain on a fresh session returned false")
	}
	if refs, _ := session.idleState(); refs != 2 {
		t.Fatalf("refs after retain = %d, want 2", refs)
	}

	session.release()
	session.release()
	refs, idleAt := session.idleState()
	if refs != 0 || idleAt.IsZero() {
		t.Fatalf("idleState after releasing every borrower = (%d, %v), want (0, non-zero)", refs, idleAt)
	}

	if !session.retain() {
		t.Fatal("retain on an idle-but-open session returned false")
	}
	if refs, idleAt := session.idleState(); refs != 1 || !idleAt.IsZero() {
		t.Fatalf("idleState after re-retain = (%d, %v), want (1, zero)", refs, idleAt)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if session.retain() {
		t.Error("retain on a closed session unexpectedly succeeded")
	}
}

// TestAFCSessionMetadataCachingAndErrors covers the fast path of metadata():
// the dialer only runs once per session, and every later caller observes the
// same cached client.
func TestAFCSessionMetadataCachingAndErrors(t *testing.T) {
	var dials atomic.Int32
	session := newAFCSession("meta", func(context.Context) (io.ReadWriteCloser, error) {
		dials.Add(1)
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	ctx := context.Background()
	first, err := session.metadata(ctx)
	if err != nil {
		t.Fatalf("metadata: %v", err)
	}
	second, err := session.metadata(ctx)
	if err != nil {
		t.Fatalf("metadata (cached): %v", err)
	}
	if first != second {
		t.Error("metadata dialed a second connection instead of reusing the cached client")
	}
	if dials.Load() != 1 {
		t.Fatalf("dials = %d, want exactly one", dials.Load())
	}
}

// TestAFCSessionMetadataDialFailure checks that a dial failure is surfaced to
// the caller without being cached as a client.
func TestAFCSessionMetadataDialFailure(t *testing.T) {
	dialErr := errors.New("dial refused")
	session := newAFCSession("meta-fail", func(context.Context) (io.ReadWriteCloser, error) {
		return nil, dialErr
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})
	if _, err := session.metadata(context.Background()); !errors.Is(err, dialErr) {
		t.Fatalf("metadata dial error = %v, want %v", err, dialErr)
	}
}

// TestAFCSessionMetadataOnClosedSession checks that a session which is
// already closed refuses to dial at all.
func TestAFCSessionMetadataOnClosedSession(t *testing.T) {
	session := newAFCSession("meta-closed", func(context.Context) (io.ReadWriteCloser, error) {
		t.Fatal("dial must not run on an already-closed session")
		return nil, nil
	})
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.metadata(context.Background()); !errors.Is(err, afcproto.ErrClosed) {
		t.Fatalf("metadata on closed session = %v, want ErrClosed", err)
	}
}

// TestAFCSessionMetadataRaceWithClose covers the narrow window where a
// session is torn down while its metadata dial is still in flight: the
// freshly dialed connection must be discarded rather than adopted.
func TestAFCSessionMetadataRaceWithClose(t *testing.T) {
	var session *afcSession
	session = newAFCSession("meta-race", func(context.Context) (io.ReadWriteCloser, error) {
		// Simulate the session closing concurrently while this dial is
		// outside any lock, the same race markMetadataError/reset guard
		// against for the transfer pool.
		session.closed.Store(true)
		return &poolTestConnection{}, nil
	})
	client, err := session.metadata(context.Background())
	if client != nil || !errors.Is(err, afcproto.ErrClosed) {
		t.Fatalf("metadata racing with close = (%v, %v), want (nil, ErrClosed)", client, err)
	}
}

// TestAFCSessionMarkMetadataError covers the three cases markMetadataError
// must distinguish: an ordinary error that leaves the cache alone, a
// connection-lost error on a client that is no longer cached, and a
// connection-lost error on the currently cached client.
func TestAFCSessionMarkMetadataError(t *testing.T) {
	session := newAFCSession("mark", func(context.Context) (io.ReadWriteCloser, error) {
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	cached, err := session.metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	session.markMetadataError(cached, errors.New("a plain, non-fatal protocol error"))
	if again, err := session.metadata(context.Background()); err != nil || again != cached {
		t.Fatalf("an ordinary error evicted the cached metadata client: again=%v err=%v", again, err)
	}

	stale := afcproto.New(&poolTestConnection{})
	session.markMetadataError(stale, afcproto.ErrConnectionLost)
	if again, err := session.metadata(context.Background()); err != nil || again != cached {
		t.Fatalf("a connection-lost error on a foreign client evicted the cache: again=%v err=%v", again, err)
	}

	session.markMetadataError(cached, afcproto.ErrConnectionLost)
	replacement, err := session.metadata(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if replacement == cached {
		t.Error("a connection-lost error did not evict the cached metadata client")
	}
}

// TestAFCSessionCloseAggregatesClients checks that Close reaches every
// connection the session ever opened - the cached metadata client and every
// pooled transfer client, whether idle or still leased out - and that a
// second Close is a cheap no-op.
func TestAFCSessionCloseAggregatesClients(t *testing.T) {
	var conns []*poolTestConnection
	session := newAFCSession("close-agg", func(context.Context) (io.ReadWriteCloser, error) {
		c := &poolTestConnection{}
		conns = append(conns, c)
		return c, nil
	})

	if _, err := session.metadata(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = first
	_ = second

	if len(conns) != 3 {
		t.Fatalf("dialed %d connections, want 3 (metadata + 2 leased transfer)", len(conns))
	}

	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for i, c := range conns {
		if c.closes.Load() != 1 {
			t.Errorf("connection %d closed %d times, want exactly 1", i, c.closes.Load())
		}
	}
	if err := session.Close(); err != nil {
		t.Fatalf("second Close is not idempotent: %v", err)
	}
}

// TestAFCRegistryAcquireReuseForgetAndClose walks the registry through its
// whole session lifecycle: creating a session on first acquire, reusing it on
// later acquires of the same key, evicting it through forget, tolerating a
// forget of a now-stale session pointer, and closing every held session once
// the registry itself shuts down.
func TestAFCRegistryAcquireReuseForgetAndClose(t *testing.T) {
	registry := newAFCRegistry()
	dial := func(context.Context) (io.ReadWriteCloser, error) { return &poolTestConnection{}, nil }

	sessionA1, created, err := registry.acquire(context.Background(), "a", dial)
	if err != nil || !created {
		t.Fatalf(`first acquire of "a": session=%v created=%v err=%v`, sessionA1, created, err)
	}
	sessionA2, created, err := registry.acquire(context.Background(), "a", dial)
	if err != nil || created || sessionA2 != sessionA1 {
		t.Fatalf(`second acquire of "a" did not reuse the session: session=%v created=%v err=%v`, sessionA2, created, err)
	}
	sessionA2.release()

	sessionB, created, err := registry.acquire(context.Background(), "b", dial)
	if err != nil || !created || sessionB == sessionA1 {
		t.Fatalf(`acquire of a distinct key "b": session=%v created=%v err=%v`, sessionB, created, err)
	}
	sessionB.release()
	sessionA1.release()

	registry.forget("a", sessionA1)
	if !sessionA1.closed.Load() {
		t.Error("forget did not close the removed session")
	}
	sessionA3, created, err := registry.acquire(context.Background(), "a", dial)
	if err != nil || !created || sessionA3 == sessionA1 {
		t.Fatalf(`acquire after forget did not create a fresh session: session=%v created=%v err=%v`, sessionA3, created, err)
	}

	// forget with a now-stale session pointer must not disturb whichever
	// session currently owns the key, but it must still close the stale one.
	registry.forget("a", sessionA1)
	if again, ok := registry.sessions["a"]; !ok || again != sessionA3 {
		t.Error("forget with a stale session pointer evicted the current session")
	}

	if err := registry.Close(); err != nil {
		t.Fatalf("registry Close: %v", err)
	}
	if !sessionA3.closed.Load() || !sessionB.closed.Load() {
		t.Error("registry Close did not close every session it held")
	}
	if err := registry.Close(); err != nil {
		t.Fatalf("second registry Close is not idempotent: %v", err)
	}
	if _, _, err := registry.acquire(context.Background(), "c", dial); !errors.Is(err, afcproto.ErrClosed) {
		t.Fatalf("acquire on a closed registry = %v, want ErrClosed", err)
	}
}

// TestAFCRegistryPruneIdleSessions covers pruneLocked's two independent
// eviction rules: sessions idle past afcIdleTTL are always removed, and once
// more than afcMaxIdleSessions are idle, the oldest excess ones are removed
// too. Sessions that are still in use (refs != 0) are never touched.
func TestAFCRegistryPruneIdleSessions(t *testing.T) {
	registry := newAFCRegistry()
	now := time.Now()

	active := newAFCSession("active", nil)
	active.refs = 1 // in use: pruneLocked must skip it regardless of idleAt.
	registry.sessions["active"] = active

	stale := newAFCSession("stale", nil)
	stale.refs = 0
	stale.idleAt = now.Add(-afcIdleTTL - time.Second)
	registry.sessions["stale"] = stale

	idleKeys := make([]string, 0, afcMaxIdleSessions+2)
	for i := 0; i < afcMaxIdleSessions+2; i++ {
		key := fmt.Sprintf("idle-%d", i)
		s := newAFCSession(key, nil)
		s.refs = 0
		// Comfortably within the TTL, but staggered so pruneLocked's
		// oldest-first eviction order is deterministic.
		s.idleAt = now.Add(time.Duration(i) * time.Millisecond)
		registry.sessions[key] = s
		idleKeys = append(idleKeys, key)
	}

	removed := registry.pruneLocked(now)
	removedKeys := make(map[string]bool, len(removed))
	for _, s := range removed {
		removedKeys[s.key] = true
	}

	if !removedKeys["stale"] {
		t.Error("a session idle past afcIdleTTL was not pruned")
	}
	if removedKeys["active"] {
		t.Error("an in-use session was pruned")
	}
	if _, ok := registry.sessions["active"]; !ok {
		t.Error("an in-use session was removed from the registry")
	}

	wantRemaining := 1 + afcMaxIdleSessions // active + the newest idle sessions
	if len(registry.sessions) != wantRemaining {
		t.Fatalf("sessions remaining = %d, want %d", len(registry.sessions), wantRemaining)
	}

	excess := len(idleKeys) - afcMaxIdleSessions
	for i, key := range idleKeys {
		wantRemoved := i < excess
		if removedKeys[key] != wantRemoved {
			t.Errorf("%s pruned=%v, want %v", key, removedKeys[key], wantRemoved)
		}
	}
}
