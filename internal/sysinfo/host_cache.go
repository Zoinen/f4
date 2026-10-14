package sysinfo

import (
	"context"
	"slices"
	"sync"
	"time"
)

// HostCache retains the last successful snapshot independently of any view.
// Reads never wait for OS collection; stale data remains usable during refresh.
type HostCache struct {
	mu      sync.RWMutex
	info    HostInfo
	updated time.Time
	gate    chan struct{}
	probe   func(context.Context) (HostInfo, error)
}

func NewHostCache() *HostCache {
	return &HostCache{gate: make(chan struct{}, 1), probe: Host}
}

// Snapshot returns a detached copy and whether it is less than 30 seconds old.
func (c *HostCache) Snapshot() (HostInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	info := c.info
	info.Displays = slices.Clone(info.Displays)
	return info, !c.updated.IsZero() && time.Since(c.updated) < 30*time.Second
}

// Refresh coalesces simultaneous callers without holding the snapshot lock
// during I/O. The owning view can cancel both waiting and active collection.
func (c *HostCache) Refresh(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.gate <- struct{}{}:
	}
	defer func() { <-c.gate }()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, fresh := c.Snapshot(); fresh {
		return nil
	}
	info, err := c.probe(ctx)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info.Displays = slices.Clone(info.Displays)
	c.mu.Lock()
	c.info, c.updated = info, time.Now()
	c.mu.Unlock()
	return nil
}
