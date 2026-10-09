package netfox

import (
	"io"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox/fishplus"
)

func testFishPoolConn() *fishConn {
	sess := fishplus.NewSession(io.Discard, nil, nil)
	return &fishConn{client: fishplus.NewClient(sess)}
}

func testFishConnClosed(conn *fishConn) bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.closed
}

func testFishPoolEntryCount(pool *fishPool) int {
	pool.mu.Lock()
	defer pool.mu.Unlock()
	return len(pool.entries)
}

func TestFishPoolKeyValidity(t *testing.T) {
	if (fishPoolKey{}).valid() {
		t.Fatal("the zero pool key is valid")
	}
	if !(fishPoolKey{host: "example.org"}).valid() {
		t.Fatal("a key with a host is invalid")
	}
}

func TestFishPoolTakeMissingAndInvalid(t *testing.T) {
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	if got := p.take(fishPoolKey{}); got != nil {
		t.Fatal("take returned a connection for the zero key")
	}
	if got := p.take(fishPoolKey{host: "missing"}); got != nil {
		t.Fatal("take returned a connection for a missing key")
	}
}

func TestFishPoolParkAndTakeRetainsConnection(t *testing.T) {
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	key := fishPoolKey{host: "example.org", port: "22"}
	conn := testFishPoolConn()
	conn.refs = 0
	p.park(conn, key)

	got := p.take(key)
	if got != conn {
		t.Fatal("take did not return the parked connection")
	}
	if conn.refs != 1 {
		t.Fatalf("refs after take = %d, want 1", conn.refs)
	}
	if testFishPoolEntryCount(p) != 0 {
		t.Fatal("take left an entry in the pool")
	}
	if err := conn.shutdown(); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestFishPoolTakeDiscardsBrokenConnection(t *testing.T) {
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	key := fishPoolKey{host: "broken"}
	conn := testFishPoolConn()
	conn.client.Session().MarkBroken()
	p.park(conn, key)

	if got := p.take(key); got != nil {
		t.Fatal("take returned a broken connection")
	}
	if !testFishConnClosed(conn) {
		t.Fatal("discarded broken connection was not shut down")
	}
}

func TestFishPoolParkReplacesOldEntry(t *testing.T) {
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	key := fishPoolKey{host: "replace"}
	old := testFishPoolConn()
	newConn := testFishPoolConn()
	p.park(old, key)
	p.park(newConn, key)

	if !testFishConnClosed(old) {
		t.Fatal("superseded connection was not shut down")
	}
	if got := p.take(key); got != newConn {
		t.Fatal("take did not return the replacement connection")
	}
	_ = newConn.shutdown()
}

func TestFishPoolCloseAllClosesEveryEntry(t *testing.T) {
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	one, two := testFishPoolConn(), testFishPoolConn()
	p.park(one, fishPoolKey{host: "one"})
	p.park(two, fishPoolKey{host: "two"})

	p.closeAll()
	if testFishPoolEntryCount(p) != 0 {
		t.Fatal("closeAll left pool entries behind")
	}
	if !testFishConnClosed(one) || !testFishConnClosed(two) {
		t.Fatal("closeAll did not shut down every connection")
	}
}

func TestFishPoolIdleTimerShutsDownEntry(t *testing.T) {
	oldTimeout := FishPoolIdleTimeout
	FishPoolIdleTimeout = 10 * time.Millisecond
	t.Cleanup(func() { FishPoolIdleTimeout = oldTimeout })
	p := &fishPool{entries: make(map[fishPoolKey]*fishPoolEntry)}
	conn := testFishPoolConn()
	p.park(conn, fishPoolKey{host: "idle"})

	deadline := time.After(2 * time.Second)
	for !testFishConnClosed(conn) {
		select {
		case <-deadline:
			t.Fatal("idle timer did not shut down the connection")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if testFishPoolEntryCount(p) != 0 {
		t.Fatal("idle timer left an entry in the pool")
	}
}

func TestFishConnectionReleaseShutsUnpoolableSession(t *testing.T) {
	conn := testFishPoolConn()
	conn.refs = 1
	if err := conn.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !testFishConnClosed(conn) {
		t.Fatal("an unpoolable session was not shut down")
	}
}

func TestFishConnectionReleaseParksPoolableSession(t *testing.T) {
	oldTimeout := FishPoolIdleTimeout
	FishPoolIdleTimeout = time.Hour
	t.Cleanup(func() { FishPoolIdleTimeout = oldTimeout })
	globalFishPool.closeAll()

	conn := testFishPoolConn()
	conn.refs = 1
	conn.key = fishPoolKey{host: "release"}
	if err := conn.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if testFishConnClosed(conn) {
		t.Fatal("a poolable session was shut down instead of parked")
	}
	if got := globalFishPool.take(conn.key); got != conn {
		t.Fatal("release did not park the poolable session")
	}
	_ = conn.shutdown()
}

func TestFishConnectionShutdownIsIdempotent(t *testing.T) {
	conn := testFishPoolConn()
	if err := conn.shutdown(); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := conn.shutdown(); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
	if !testFishConnClosed(conn) {
		t.Fatal("shutdown did not mark the connection closed")
	}
}
