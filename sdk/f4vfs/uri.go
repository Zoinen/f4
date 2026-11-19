package f4vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

var (
	errURIMountClosed = errors.New("URI mount is closed")
	errURIFileClosed  = errors.New("URI file is closed")
)

// Bridge adapts native VFS mounts to an independent SDK wire contract.
type Bridge struct {
	opener  func(context.Context, string) (vfs.VFS, error)
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	next    uint64
	mounts  map[uint64]*uriMount
	readers map[uint64]*uriReadHandle
	writers map[uint64]*uriFileHandle[io.WriteCloser]
}

type uriMount struct {
	mu      sync.Mutex
	fs      vfs.VFS
	retired []vfs.VFS
}

// URI mount and file operations serialize only within one mount or open file.
// An opened reader owns its I/O independently of the mount used to open it.
type uriFileHandle[T io.Closer] struct {
	mount  uint64
	mu     sync.Mutex
	file   T
	closed bool
}

func (h *uriFileHandle[T]) close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	return h.file.Close()
}

type uriReadHandle struct {
	uriFileHandle[vfs.ReadAtCloser]
	ctx    context.Context
	cancel context.CancelFunc
}

func (h *uriReadHandle) close() error {
	// A close may arrive while the native reader is waiting for the provider.
	h.cancel()
	return h.uriFileHandle.close()
}

func NewBridge(opener func(context.Context, string) (vfs.VFS, error)) *Bridge {
	ctx, cancel := context.WithCancel(context.Background())
	return &Bridge{
		opener: opener, ctx: ctx, cancel: cancel,
		mounts:  make(map[uint64]*uriMount),
		readers: make(map[uint64]*uriReadHandle),
		writers: make(map[uint64]*uriFileHandle[io.WriteCloser]),
	}
}

func (b *Bridge) CloseVFSURIs() error {
	b.cancel()
	b.mu.Lock()
	readers, writers, mounts := b.readers, b.writers, b.mounts
	b.readers, b.writers, b.mounts = nil, nil, nil
	b.mu.Unlock()
	for _, reader := range readers {
		_ = reader.close()
	}
	for _, writer := range writers {
		_ = writer.close()
	}
	for _, mount := range mounts {
		mount.mu.Lock()
		_ = mount.close()
		mount.mu.Unlock()
	}
	return nil
}

// close runs under the mount's own lock.
func (m *uriMount) close() error {
	if m.fs == nil {
		return nil
	}
	err := m.fs.Close()
	m.fs = nil
	for _, retired := range m.retired {
		_ = retired.Close()
	}
	m.retired = nil
	return err
}

func (b *Bridge) retain(mounted vfs.VFS) (f4plugin.URIMount, error) {
	result := snapshot(0, mounted)
	b.mu.Lock()
	if b.ctx.Err() != nil {
		b.mu.Unlock()
		_ = mounted.Close()
		return f4plugin.URIMount{}, errURIMountClosed
	}
	b.next++
	result.ID = b.next
	b.mounts[result.ID] = &uriMount{fs: mounted}
	b.mu.Unlock()
	return result, nil
}

func snapshot(id uint64, mounted vfs.VFS) f4plugin.URIMount {
	hasParent := mounted.ParentVFS() != nil
	return f4plugin.URIMount{
		ID: id, Path: mounted.GetPath(), Root: mounted.IsAtRoot(),
		HasParent: &hasParent, Capabilities: mounted.GetCapabilities(),
	}
}

func (b *Bridge) CallVFSURI(ctx context.Context, req f4plugin.URIRequest) (any, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(b.ctx, cancel)
	defer stop()
	if b.ctx.Err() != nil {
		return nil, errURIMountClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch req.Operation {
	case "openURI":
		mounted, err := b.opener(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return b.retain(mounted)
	case "readAt":
		return b.readAt(ctx, req)
	case "write":
		return b.write(req)
	case "closeFile":
		return nil, b.closeFile(req)
	}

	b.mu.Lock()
	mount := b.mounts[req.Mount]
	b.mu.Unlock()
	if mount == nil {
		return nil, errURIMountClosed
	}
	mount.mu.Lock()
	defer mount.mu.Unlock()
	if mount.fs == nil {
		return nil, errURIMountClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return b.callMount(ctx, mount, req)
}

func (b *Bridge) callMount(ctx context.Context, mount *uriMount, req f4plugin.URIRequest) (any, error) {
	mounted := mount.fs
	switch req.Operation {
	case "set", "setOptimistic":
		var err error
		setter, optimistic := mounted.(vfs.OptimisticPathSetter)
		if req.Operation == "setOptimistic" && optimistic {
			err = setter.SetPathOptimistic(req.Path)
		} else {
			err = mounted.SetPath(req.Path)
		}
		if err != nil {
			if next, openErr := b.opener(ctx, req.Path); openErr == nil {
				mount.retired = append(mount.retired, mounted)
				mount.fs, mounted, err = next, next, nil
			}
		}
		return snapshot(req.Mount, mounted), err
	case "join":
		return mounted.Join(req.Elements...), nil
	case "abs":
		return mounted.Abs(req.Path)
	case "base":
		return mounted.Base(req.Path), nil
	case "dir":
		return mounted.Dir(req.Path), nil
	case "isabs":
		return mounted.IsAbs(req.Path), nil
	case "clone":
		return b.retain(mounted.Clone())
	case "parent":
		parent := mounted.ParentVFS()
		if parent == nil {
			return f4plugin.URIMount{}, nil
		}
		return b.retain(parent.Clone())
	case "close":
		err := mount.close()
		b.mu.Lock()
		delete(b.mounts, req.Mount)
		b.mu.Unlock()
		return nil, err
	case "readDir":
		items := []vfs.VFSItem{}
		err := mounted.ReadDir(ctx, req.Path, func(chunk []vfs.VFSItem) { items = append(items, chunk...) })
		return items, err
	case "stat":
		return mounted.Stat(ctx, req.Path)
	case "mkdir":
		return nil, mounted.MkDir(ctx, req.Path)
	case "remove":
		return nil, mounted.Remove(ctx, req.Path)
	case "rename":
		return nil, mounted.Rename(ctx, req.Path, req.Other)
	case "attributes":
		var item vfs.VFSItem
		encoded, err := msgpack.Marshal(req.Item)
		if err != nil {
			return nil, err
		}
		if err := msgpack.Unmarshal(encoded, &item); err != nil {
			return nil, err
		}
		return nil, mounted.SetAttributes(ctx, req.Path, item)
	case "open":
		reader, err := mounted.Open(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return b.keepReader(req.Mount, reader)
	case "create":
		writer, err := mounted.Create(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return b.keepWriter(req.Mount, writer)
	default:
		return nil, fmt.Errorf("unknown URI operation %q", req.Operation)
	}
}

func (b *Bridge) keepReader(mount uint64, reader vfs.ReadAtCloser) (f4plugin.URIFile, error) {
	size := reader.Size()
	b.mu.Lock()
	if b.ctx.Err() != nil {
		b.mu.Unlock()
		_ = reader.Close()
		return f4plugin.URIFile{}, errURIMountClosed
	}
	defer b.mu.Unlock()
	b.next++
	ctx, cancel := context.WithCancel(b.ctx)
	b.readers[b.next] = &uriReadHandle{
		uriFileHandle: uriFileHandle[vfs.ReadAtCloser]{mount: mount, file: reader},
		ctx:           ctx, cancel: cancel,
	}
	return f4plugin.URIFile{ID: b.next, Size: size}, nil
}

func (b *Bridge) keepWriter(mount uint64, writer io.WriteCloser) (f4plugin.URIFile, error) {
	b.mu.Lock()
	if b.ctx.Err() != nil {
		b.mu.Unlock()
		_ = writer.Close()
		return f4plugin.URIFile{}, errURIMountClosed
	}
	defer b.mu.Unlock()
	b.next++
	b.writers[b.next] = &uriFileHandle[io.WriteCloser]{mount: mount, file: writer}
	return f4plugin.URIFile{ID: b.next}, nil
}

func (b *Bridge) readAt(ctx context.Context, req f4plugin.URIRequest) ([]byte, error) {
	if req.Length < 0 || req.Length > 64<<20 {
		return nil, fmt.Errorf("invalid URI read length")
	}
	b.mu.Lock()
	reader := b.readers[req.File]
	b.mu.Unlock()
	if reader == nil || reader.mount != req.Mount {
		return nil, errURIFileClosed
	}
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed || reader.ctx.Err() != nil {
		return nil, errURIFileClosed
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(reader.ctx, cancel)
	defer stop()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data := make([]byte, req.Length)
	n, err := reader.file.ReadAt(ctx, data, req.Offset)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return data[:n], err
}

func (b *Bridge) write(req f4plugin.URIRequest) (int, error) {
	b.mu.Lock()
	writer := b.writers[req.File]
	b.mu.Unlock()
	if writer == nil || writer.mount != req.Mount {
		return 0, errURIFileClosed
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.closed {
		return 0, errURIFileClosed
	}
	return writer.file.Write(req.Data)
}

func (b *Bridge) closeFile(req f4plugin.URIRequest) error {
	b.mu.Lock()
	reader, writer := b.readers[req.File], b.writers[req.File]
	b.mu.Unlock()
	// Keep the handle registered until the native close finishes so shutdown
	// can wait for the same close before tearing down its source mount.
	if reader != nil && reader.mount == req.Mount {
		err := reader.close()
		b.mu.Lock()
		delete(b.readers, req.File)
		b.mu.Unlock()
		return err
	}
	if writer != nil && writer.mount == req.Mount {
		err := writer.close()
		b.mu.Lock()
		delete(b.writers, req.File)
		b.mu.Unlock()
		return err
	}
	return nil
}
