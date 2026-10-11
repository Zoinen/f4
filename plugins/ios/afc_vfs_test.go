package iosfs

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/ios/internal/afcproto"
)

type poolTestConnection struct{ closes atomic.Int32 }

func TestAFCDeviceQualifiedPublicPaths(t *testing.T) {
	v := &AFCVFS{
		device: DeviceInfo{UDID: "stable-id", Name: "Alexander's iPhone"},
		title:  "Alexander's iPhone:/", path: "/DCIM/100APPLE",
	}
	want := "ios://Alexander's iPhone/DCIM/100APPLE"
	if got := v.GetPath(); got != want {
		t.Fatalf("GetPath() = %q, want %q", got, want)
	}
	file := v.Join(v.GetPath(), "IMG_0007.JPG")
	if got, err := v.resolve(file); err != nil || got != "/DCIM/100APPLE/IMG_0007.JPG" {
		t.Fatalf("transport path = %q, %v", got, err)
	}
	if got := v.Dir(file); got != want {
		t.Fatalf("Dir() = %q, want %q", got, want)
	}
	if _, err := v.resolve("ios://Another iPhone/DCIM/file"); err == nil {
		t.Fatal("foreign device path accepted")
	}
}

func (*poolTestConnection) Read([]byte) (int, error)    { return 0, io.EOF }
func (*poolTestConnection) Write(p []byte) (int, error) { return len(p), nil }
func (c *poolTestConnection) Close() error {
	c.closes.Add(1)
	return nil
}

func TestAFCTransferWaiterWakesWhenLostClientReleasesCapacity(t *testing.T) {
	var dials atomic.Int32
	session := newAFCSession("test", func(context.Context) (io.ReadWriteCloser, error) {
		dials.Add(1)
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	first, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan *afcproto.Client, 1)
	errs := make(chan error, 1)
	go func() {
		client, leaseErr := session.leaseTransfer(ctx)
		result <- client
		errs <- leaseErr
	}()
	select {
	case err := <-errs:
		t.Fatalf("third lease unexpectedly completed before capacity was released: %v", err)
	default:
	}

	session.returnTransfer(first, true)
	var replacement *afcproto.Client
	select {
	case replacement = <-result:
		if err := <-errs; err != nil {
			t.Fatalf("replacement lease: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("waiter was not woken after a lost client released capacity")
	}
	if replacement == nil || replacement == first || dials.Load() != 3 {
		t.Fatalf("replacement=%p first=%p dials=%d", replacement, first, dials.Load())
	}
	session.returnTransfer(second, false)
	session.returnTransfer(replacement, false)
}

func TestAFCTransferDialStartedBeforeResetIsDiscarded(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var dials atomic.Int32
	session := newAFCSession("test", func(context.Context) (io.ReadWriteCloser, error) {
		if dials.Add(1) == 1 {
			close(started)
			<-release
		}
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan *afcproto.Client, 1)
	errs := make(chan error, 1)
	go func() {
		client, err := session.leaseTransfer(ctx)
		result <- client
		errs <- err
	}()
	<-started

	// Simulate reset's atomic pool invalidation while the first dial is still
	// outside transferMu. That pre-reset connection must not enter the new pool.
	session.transferMu.Lock()
	session.transferAll = make(map[*afcproto.Client]struct{})
	session.transferCount = 0
	session.transferEpoch++
	session.transferMu.Unlock()
	close(release)

	client := <-result
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	if client == nil || dials.Load() != 2 {
		t.Fatalf("client=%p dials=%d, want a fresh post-reset dial", client, dials.Load())
	}
	session.transferMu.Lock()
	count, members := session.transferCount, len(session.transferAll)
	session.transferMu.Unlock()
	if count != 1 || members != 1 {
		t.Fatalf("pool count=%d members=%d, want one live client", count, members)
	}
	session.returnTransfer(client, false)
}

// TestAFCTransferPoolRejectsStaleQueuedClient covers the branch where
// tryLeaseTransfer dequeues a client that reset() has already invalidated: it
// must discard that client without counting it against the pool's capacity
// and retry, rather than handing a dead client to the caller.
func TestAFCTransferPoolRejectsStaleQueuedClient(t *testing.T) {
	var dials atomic.Int32
	session := newAFCSession("stale-pool", func(context.Context) (io.ReadWriteCloser, error) {
		dials.Add(1)
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	bogus := afcproto.New(&poolTestConnection{})
	session.transferMu.Lock()
	session.transferIdle <- bogus
	session.transferMu.Unlock()

	client, acquired, err := session.tryLeaseTransfer(context.Background())
	if err != nil || !acquired {
		t.Fatalf("tryLeaseTransfer returned (%v, %v, %v)", client, acquired, err)
	}
	if client == bogus {
		t.Fatal("a stale, unregistered client was leased instead of being discarded")
	}
	if dials.Load() != 1 {
		t.Fatalf("dials = %d, want exactly one fresh dial for the retried lease", dials.Load())
	}
	session.transferMu.Lock()
	count := session.transferCount
	session.transferMu.Unlock()
	if count != 1 {
		t.Fatalf("transferCount = %d, want 1 live client after discarding the stale one", count)
	}
	session.returnTransfer(client, false)
}

// TestAFCTransferPoolExhaustedReturnsWithoutError covers tryLeaseTransfer's
// "pool is full but nothing is idle" branch: it must report failure without
// an error so the caller (leaseTransfer) knows to wait for capacity instead
// of giving up.
func TestAFCTransferPoolExhaustedReturnsWithoutError(t *testing.T) {
	session := newAFCSession("full-pool", func(context.Context) (io.ReadWriteCloser, error) {
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})

	ctx := context.Background()
	first, err := session.leaseTransfer(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.leaseTransfer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	client, acquired, err := session.tryLeaseTransfer(ctx)
	if err != nil || acquired || client != nil {
		t.Fatalf("tryLeaseTransfer on a full pool = (%v, %v, %v), want (nil, false, nil)", client, acquired, err)
	}

	session.returnTransfer(first, false)
	session.returnTransfer(second, false)
}

// TestAFCSessionLeaseTransferUnblocksOnContextTimeout covers leaseTransfer's
// wait loop: with the pool exhausted and no client ever returned, a caller's
// context deadline must wake it up rather than blocking forever.
func TestAFCSessionLeaseTransferUnblocksOnContextTimeout(t *testing.T) {
	session := newAFCSession("ctx-timeout", func(context.Context) (io.ReadWriteCloser, error) {
		return &poolTestConnection{}, nil
	})
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("close AFC session: %v", err)
		}
	})
	first, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := session.leaseTransfer(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer session.returnTransfer(first, false)
	defer session.returnTransfer(second, false)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := session.leaseTransfer(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("leaseTransfer on an exhausted, timed-out pool = %v, want DeadlineExceeded", err)
	}
}

// TestAFCSessionLeaseTransferUnblocksOnClose covers leaseTransfer's other
// wake source: a waiter blocked on an exhausted pool must be released with
// ErrClosed as soon as the session itself is closed, instead of leaking.
func TestAFCSessionLeaseTransferUnblocksOnClose(t *testing.T) {
	session := newAFCSession("closed-wake", func(context.Context) (io.ReadWriteCloser, error) {
		return &poolTestConnection{}, nil
	})
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

	result := make(chan error, 1)
	go func() {
		_, leaseErr := session.leaseTransfer(context.Background())
		result <- leaseErr
	}()
	// Give the waiter goroutine time to reach the blocking select before the
	// session closes, so this exercises the <-s.done wake path rather than
	// the closed-registry short-circuit at the top of tryLeaseTransfer.
	time.Sleep(100 * time.Millisecond)
	if err := session.Close(); err != nil {
		t.Fatalf("close AFC session: %v", err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, afcproto.ErrClosed) {
			t.Fatalf("leaseTransfer after close = %v, want ErrClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leaseTransfer waiter was not woken by session Close")
	}
}
