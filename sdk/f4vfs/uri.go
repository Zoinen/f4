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

// Bridge adapts native VFS mounts to an independent SDK wire contract.
type Bridge struct {
	opener  func(context.Context, string) (vfs.VFS, error)
	mu      sync.Mutex
	next    uint64
	mounts  map[uint64]vfs.VFS
	readers map[uint64]vfs.ReadAtCloser
	writers map[uint64]io.WriteCloser
	retired []vfs.VFS
}

func NewBridge(opener func(context.Context, string) (vfs.VFS, error)) *Bridge {
	return &Bridge{opener: opener, mounts: map[uint64]vfs.VFS{}, readers: map[uint64]vfs.ReadAtCloser{}, writers: map[uint64]io.WriteCloser{}}
}
func (b *Bridge) CloseVFSURIs() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, reader := range b.readers {
		_ = reader.Close()
	}
	for _, writer := range b.writers {
		_ = writer.Close()
	}
	for _, mount := range b.mounts {
		_ = mount.Close()
	}
	for _, mount := range b.retired {
		_ = mount.Close()
	}
	b.mounts = map[uint64]vfs.VFS{}
	b.readers = map[uint64]vfs.ReadAtCloser{}
	b.writers = map[uint64]io.WriteCloser{}
	b.retired = nil
	return nil
}
func (b *Bridge) retain(mounted vfs.VFS) f4plugin.URIMount {
	b.next++
	b.mounts[b.next] = mounted
	return b.snapshot(b.next, mounted)
}

func (b *Bridge) snapshot(id uint64, mounted vfs.VFS) f4plugin.URIMount {
	return f4plugin.URIMount{ID: id, Path: mounted.GetPath(), Root: mounted.IsAtRoot(), Capabilities: mounted.GetCapabilities()}
}

func (b *Bridge) CallVFSURI(ctx context.Context, req f4plugin.URIRequest) (any, error) {
	// Every mutable mount and file handle belongs to this bridge. Serialize
	// requests so SetPath/Clone and a concurrent close cannot race a native VFS.
	b.mu.Lock()
	defer b.mu.Unlock()
	if req.Operation == "openURI" {
		mounted, err := b.opener(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		return b.retain(mounted), nil
	}
	mounted := b.mounts[req.Mount]
	if mounted == nil {
		return nil, fmt.Errorf("URI mount is closed")
	}
	switch req.Operation {
	case "set":
		err := mounted.SetPath(req.Path)
		if err != nil {
			if next, openErr := b.opener(ctx, req.Path); openErr == nil {
				b.retired = append(b.retired, mounted)
				b.mounts[req.Mount], mounted, err = next, next, nil
			}
		}
		return b.snapshot(req.Mount, mounted), err
	case "setOptimistic":
		var err error
		if setter, ok := mounted.(vfs.OptimisticPathSetter); ok {
			err = setter.SetPathOptimistic(req.Path)
		} else {
			err = mounted.SetPath(req.Path)
		}
		if err != nil {
			if next, openErr := b.opener(ctx, req.Path); openErr == nil {
				b.retired = append(b.retired, mounted)
				b.mounts[req.Mount], mounted, err = next, next, nil
			}
		}
		return b.snapshot(req.Mount, mounted), err
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
		return b.retain(mounted.Clone()), nil
	case "parent":
		parent := mounted.ParentVFS()
		if parent == nil {
			return f4plugin.URIMount{}, nil
		}
		return b.retain(parent.Clone()), nil
	case "close":
		delete(b.mounts, req.Mount)
		return nil, mounted.Close()
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
		b.next++
		b.readers[b.next] = reader
		return f4plugin.URIFile{ID: b.next, Size: reader.Size()}, nil
	case "create":
		writer, err := mounted.Create(ctx, req.Path)
		if err != nil {
			return nil, err
		}
		b.next++
		b.writers[b.next] = writer
		return f4plugin.URIFile{ID: b.next}, nil
	case "readAt":
		reader := b.readers[req.File]
		if reader == nil {
			return nil, fmt.Errorf("URI file is closed")
		}
		if req.Length < 0 || req.Length > 64<<20 {
			return nil, fmt.Errorf("invalid URI read length")
		}
		data := make([]byte, req.Length)
		n, err := reader.ReadAt(ctx, data, req.Offset)
		if errors.Is(err, io.EOF) {
			err = nil
		}
		return data[:n], err
	case "write":
		writer := b.writers[req.File]
		if writer == nil {
			return nil, fmt.Errorf("URI file is closed")
		}
		return writer.Write(req.Data)
	case "closeFile":
		if reader := b.readers[req.File]; reader != nil {
			delete(b.readers, req.File)
			return nil, reader.Close()
		}
		if writer := b.writers[req.File]; writer != nil {
			delete(b.writers, req.File)
			return nil, writer.Close()
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unknown URI operation %q", req.Operation)
	}
}
