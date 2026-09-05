package androidfs

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
)

type poolFishPeer struct {
	noops       atomic.Int64
	closed      chan struct{}
	blockNoop   <-chan struct{}
	noopStarted chan struct{}
	startOnce   sync.Once
}

func newPoolTestFish(t *testing.T, parent vfs.VFS, blockNoop ...<-chan struct{}) (*netfox.FishVFS, *poolFishPeer) {
	t.Helper()
	clientConn, peerConn := net.Pipe()
	peer := &poolFishPeer{closed: make(chan struct{}), noopStarted: make(chan struct{})}
	if len(blockNoop) != 0 {
		peer.blockNoop = blockNoop[0]
	}
	go func() {
		defer close(peer.closed)
		defer func() {
			_ = peerConn.Close() // connection cleanup only
		}()
		scanner := bufio.NewScanner(peerConn)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		if !scanner.Scan() {
			return
		}
		const markerSource = `echo F4R"DY"`
		bootstrap := scanner.Text()
		at := strings.Index(bootstrap, markerSource)
		if at < 0 {
			return
		}
		token, _, ok := strings.Cut(bootstrap[at+len(markerSource):], ";")
		if !ok || token == "" {
			return
		}
		_, _ = fmt.Fprintf(peerConn, "%s\n", fishplus.ReadyMarker(token))
		for scanner.Scan() && scanner.Text() != fishplus.HelperEndMarker {
		}
		_, _ = fmt.Fprintf(peerConn, ".%s 0 ok FISHPLUS 1 mode:stat read:dd write:b64\n", token)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}
			switch fields[1] {
			case "pwd":
				_, _ = fmt.Fprintln(peerConn, "/")
			case "noop":
				peer.noops.Add(1)
				if peer.blockNoop != nil {
					peer.startOnce.Do(func() { close(peer.noopStarted) })
					<-peer.blockNoop
					peer.blockNoop = nil
				}
			default:
				_, _ = fmt.Fprintf(peerConn, ".%s %s err unsupported\n", token, fields[0])
				continue
			}
			_, _ = fmt.Fprintf(peerConn, ".%s %s ok\n", token, fields[0])
		}
	}()

	fish, err := netfox.NewFishVFSOnStream(context.Background(), parent, clientConn, clientConn, clientConn, "pool-test")
	if err != nil {
		_ = clientConn.Close() // connection cleanup only
		t.Fatalf("create test FishVFS: %v", err)
	}
	return fish, peer
}

func TestFishSessionPoolReusesConnectionAndReparentsView(t *testing.T) {
	pool := newFishSessionPool()
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline}
	firstParent := vfs.NewNullVFS(0)
	secondParent := vfs.NewNullVFS(0)
	var opens atomic.Int64
	var peer *poolFishPeer
	open := func(_ context.Context, parent vfs.VFS, _ DeviceInfo) (vfs.VFS, error) {
		opens.Add(1)
		var fish *netfox.FishVFS
		fish, peer = newPoolTestFish(t, parent)
		return fish, nil
	}

	first, err := pool.Open(context.Background(), firstParent, device, open)
	if err != nil {
		t.Fatalf("first pool open: %v", err)
	}
	firstKey := first.(vfs.SessionIdentity).SessionKey()
	if err := first.Close(); err != nil {
		t.Fatalf("close first view: %v", err)
	}

	second, err := pool.Open(context.Background(), secondParent, device, open)
	if err != nil {
		t.Fatalf("second pool open: %v", err)
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("backend opens = %d, want 1", got)
	}
	if got := peer.noops.Load(); got != 1 {
		t.Fatalf("pool validation noops = %d, want 1", got)
	}
	if second.(vfs.SessionIdentity).SessionKey() != firstKey {
		t.Fatal("second view did not reuse the first FISH+ connection")
	}
	if second.ParentVFS() != secondParent {
		t.Fatal("pooled view retained the first manager parent")
	}
	if err := second.Close(); err != nil {
		t.Fatalf("close second view: %v", err)
	}
	if err := pool.Close(); err != nil {
		t.Fatalf("close pool: %v", err)
	}
	select {
	case <-peer.closed:
	case <-time.After(time.Second):
		t.Fatal("pool anchor did not close the FISH+ transport")
	}
}

func TestFishSessionPoolDoesNotRetainNonFishVFS(t *testing.T) {
	pool := newFishSessionPool()
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("close FISH+ session pool: %v", err)
		}
	})
	var opens atomic.Int64
	open := func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error) {
		opens.Add(1)
		return vfs.NewNullVFS(0), nil
	}
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline}
	for i := 0; i < 2; i++ {
		mounted, err := pool.Open(context.Background(), nil, device, open)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		_ = mounted.Close()
	}
	if got := opens.Load(); got != 2 {
		t.Fatalf("non-FISH backend opens = %d, want 2", got)
	}
}

func TestFishSessionPoolDoesNotWaitBehindBusyConnection(t *testing.T) {
	pool := newFishSessionPool()
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline}
	releaseNoop := make(chan struct{})
	var opens atomic.Int64
	var firstPeer *poolFishPeer
	open := func(_ context.Context, parent vfs.VFS, _ DeviceInfo) (vfs.VFS, error) {
		if opens.Add(1) == 1 {
			fish, peer := newPoolTestFish(t, parent, releaseNoop)
			firstPeer = peer
			return fish, nil
		}
		fish, _ := newPoolTestFish(t, parent)
		return fish, nil
	}

	first, err := pool.Open(context.Background(), nil, device, open)
	if err != nil {
		t.Fatalf("first pool open: %v", err)
	}
	oldKey := first.(vfs.SessionIdentity).SessionKey()
	busyDone := make(chan error, 1)
	go func() {
		busyDone <- first.(*netfox.FishVFS).Client().Session().Noop(context.Background())
	}()
	select {
	case <-firstPeer.noopStarted:
	case <-time.After(time.Second):
		t.Fatal("the first session did not become busy")
	}

	start := time.Now()
	second, err := pool.Open(context.Background(), nil, device, open)
	if err != nil {
		t.Fatalf("open while first session is busy: %v", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("pool waited %s behind the busy connection", elapsed)
	}
	if got := opens.Load(); got != 2 {
		t.Fatalf("backend opens = %d, want a separate second connection", got)
	}
	if second.(vfs.SessionIdentity).SessionKey() == oldKey {
		t.Fatal("pool attached the new panel to the busy connection")
	}

	close(releaseNoop)
	select {
	case err := <-busyDone:
		if err != nil {
			t.Fatalf("busy request after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("busy request did not finish after release")
	}
	_ = first.Close()
	_ = second.Close()
	if err := pool.Close(); err != nil {
		t.Fatalf("close pool: %v", err)
	}
}

func TestFishSessionPoolReopensClosingConnection(t *testing.T) {
	pool := newFishSessionPool()
	t.Cleanup(func() {
		if err := pool.Close(); err != nil {
			t.Errorf("close FISH+ session pool: %v", err)
		}
	})
	device := DeviceInfo{Serial: "serial", State: DeviceStateOnline}
	var opens atomic.Int64
	open := func(_ context.Context, parent vfs.VFS, _ DeviceInfo) (vfs.VFS, error) {
		opens.Add(1)
		fish, _ := newPoolTestFish(t, parent)
		return fish, nil
	}

	first, err := pool.Open(context.Background(), nil, device, open)
	if err != nil {
		t.Fatalf("first pool open: %v", err)
	}
	oldKey := first.(vfs.SessionIdentity).SessionKey()
	if err := first.(*netfox.FishVFS).Client().Session().Close(); err != nil {
		t.Fatalf("poison first session: %v", err)
	}
	_ = first.Close()

	second, err := pool.Open(context.Background(), nil, device, open)
	if err != nil {
		t.Fatalf("reopen closing pooled session: %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close reopened pooled session: %v", err)
		}
	})
	if got := opens.Load(); got != 2 {
		t.Fatalf("backend opens = %d, want 2", got)
	}
	if second.(vfs.SessionIdentity).SessionKey() == oldKey {
		t.Fatal("pool reused a closing FISH+ connection")
	}
}
