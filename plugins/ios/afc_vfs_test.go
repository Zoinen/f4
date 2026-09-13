package iosfs

import (
	"context"
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
