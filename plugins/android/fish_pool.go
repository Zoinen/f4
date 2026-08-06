package androidfs

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/unxed/f4/plugins/netfox"
	"github.com/unxed/f4/plugins/netfox/fishplus"
	"github.com/unxed/f4/vfs"
)

const fishPoolProbeTimeout = 2 * time.Second

var errFishPoolClosed = errors.New("android: FISH+ session pool is closed")

type fishOpenFunc func(context.Context, vfs.VFS, DeviceInfo) (vfs.VFS, error)

// fishSessionPool keeps one parentless view of each live Android FISH+
// connection. Closing a panel releases only its own view; the anchor keeps the
// shell helper alive so entering the same device again needs just one noop.
// Different serials have different locks and can still connect concurrently.
type fishSessionPool struct {
	mu      sync.Mutex
	entries map[string]*fishPoolEntry
	closed  bool
}

type fishPoolEntry struct {
	mu     sync.Mutex
	anchor *netfox.FishVFS
}

func newFishSessionPool() *fishSessionPool {
	return &fishSessionPool{entries: make(map[string]*fishPoolEntry)}
}

func (p *fishSessionPool) Open(ctx context.Context, parent vfs.VFS, device DeviceInfo, open fishOpenFunc) (vfs.VFS, error) {
	if open == nil {
		return nil, errors.New("android: FISH+ backend is unavailable")
	}

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, errFishPoolClosed
	}
	entry := p.entries[device.Serial]
	if entry == nil {
		entry = &fishPoolEntry{}
		p.entries[device.Serial] = entry
	}
	p.mu.Unlock()

	// Serialize only the validation/open transition for this serial. Without
	// it two simultaneous first entries would both pay for a helper upload and
	// whichever finished last would leak an unnecessary idle shell.
	entry.mu.Lock()
	defer entry.mu.Unlock()

	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return nil, errFishPoolClosed
	}

	if entry.anchor != nil {
		probeCtx, cancel := context.WithTimeout(ctx, fishPoolProbeTimeout)
		client := entry.anchor.Client()
		attempted := false
		var probeErr error
		if client == nil || client.Session() == nil {
			attempted = true
			probeErr = fishplus.ErrBroken
		} else {
			attempted, probeErr = client.Session().TryNoop(probeCtx)
		}
		cancel()
		if attempted && probeErr == nil {
			return entry.anchor.CloneForParent(parent), nil
		}
		if !attempted && probeErr != nil && ctx.Err() != nil {
			// Cancellation happened before a probe reached the wire. Keep the
			// otherwise untouched anchor for the next entry attempt.
			return nil, ctx.Err()
		}
		_ = entry.anchor.Close()
		entry.anchor = nil
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}

	mounted, err := open(ctx, parent, device)
	if err != nil {
		return nil, err
	}
	fish, ok := mounted.(*netfox.FishVFS)
	if !ok {
		// Injectable tests and any future non-FISH backend keep their ordinary
		// ownership semantics; only a real FishVFS can safely share fishConn.
		return mounted, nil
	}

	anchor := fish.CloneForParent(nil)
	p.mu.Lock()
	closed = p.closed
	p.mu.Unlock()
	if closed {
		_ = anchor.Close()
		_ = fish.Close()
		return nil, errFishPoolClosed
	}
	entry.anchor = anchor
	return fish, nil
}

func (p *fishSessionPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	entries := make([]*fishPoolEntry, 0, len(p.entries))
	for _, entry := range p.entries {
		entries = append(entries, entry)
	}
	p.entries = nil
	p.mu.Unlock()

	var result error
	for _, entry := range entries {
		entry.mu.Lock()
		if entry.anchor != nil {
			result = errors.Join(result, entry.anchor.Close())
			entry.anchor = nil
		}
		entry.mu.Unlock()
	}
	return result
}
