package plughost

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const uriMediaTimeout = 10 * time.Second

// A catalog registers media before Qt requests it. Resolve the independent
// mount only when a media worker needs it, outside the broker and UI locks.
type rpcURIMediaVFS struct {
	*rpcURIVFS
	mu     sync.Mutex
	mount  *rpcURIVFS
	flight chan struct{}
	closed bool
}

func (m *rpcURIMediaVFS) acquire(ctx context.Context) (*rpcURIVFS, error) {
	for {
		m.mu.Lock()
		if m.closed || m.mount != nil {
			mount := m.mount
			m.mu.Unlock()
			if mount == nil {
				return nil, io.ErrClosedPipe
			}
			return mount, nil
		}
		if flight := m.flight; flight != nil {
			m.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-flight:
			}
			continue
		}
		m.flight = make(chan struct{})
		m.mu.Unlock()

		cloneCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		mount, err := m.rpcURIVFS.relatedContext(cloneCtx, "clone")
		cancel()
		m.mu.Lock()
		closed := m.closed
		if !closed {
			m.mount = mount
		}
		close(m.flight)
		m.flight = nil
		m.mu.Unlock()
		if closed && mount != nil {
			mount.ClosePanelMount()
		}
		if err == nil && (closed || mount == nil) {
			err = io.ErrClosedPipe
		}
		return mount, err
	}
}

func (m *rpcURIMediaVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	ctx, cancel := context.WithTimeout(ctx, uriMediaTimeout)
	defer cancel()
	mount, err := m.acquire(ctx)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	return mount.Stat(ctx, path)
}

func (m *rpcURIMediaVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, uriMediaTimeout)
	defer cancel()
	mount, err := m.acquire(ctx)
	if err != nil {
		return nil, err
	}
	file, err := mount.Open(ctx, path)
	if err != nil {
		return nil, err
	}
	return uriMediaReader{ReadAtCloser: file}, nil
}

type uriMediaReader struct{ vfs.ReadAtCloser }

func (r uriMediaReader) ReadAt(ctx context.Context, data []byte, offset int64) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, uriMediaTimeout)
	defer cancel()
	return r.ReadAtCloser.ReadAt(ctx, data, offset)
}

func (m *rpcURIMediaVFS) ReadDir(ctx context.Context, path string, onChunk func([]vfs.VFSItem)) error {
	mount, err := m.acquire(ctx)
	if err != nil {
		return err
	}
	return mount.ReadDir(ctx, path, onChunk)
}

func (m *rpcURIMediaVFS) Close() error {
	m.mu.Lock()
	mount := m.mount
	m.mount = nil
	m.closed = true
	m.mu.Unlock()
	if mount != nil {
		mount.ClosePanelMount()
	}
	return nil
}
