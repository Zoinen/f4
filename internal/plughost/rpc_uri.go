package plughost

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

type deferredURIProvider struct {
	scheme    string
	ready     chan struct{}
	once      sync.Once
	mu        sync.RWMutex
	transport PluginTransport
	err       error
}

func newDeferredURIProvider(scheme string) *deferredURIProvider {
	return &deferredURIProvider{scheme: scheme, ready: make(chan struct{})}
}

func (p *deferredURIProvider) Scheme() string { return p.scheme }

func (p *deferredURIProvider) complete(transport PluginTransport, err error) {
	p.mu.Lock()
	// A permission refusal ends this restore, but must not poison a later
	// explicit approval/hot-load of the same installed provider.
	if transport != nil || p.transport == nil {
		p.transport, p.err = transport, err
	}
	p.mu.Unlock()
	p.once.Do(func() { close(p.ready) })
}

func (p *deferredURIProvider) OpenURI(ctx context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-p.ready:
	}
	p.mu.RLock()
	transport, err := p.transport, p.err
	p.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	var mounted f4plugin.URIMount
	if err := callURI(ctx, transport, f4plugin.URIRequest{Operation: "openURI", Path: raw}, &mounted); err != nil {
		return nil, err
	}
	return &rpcURIVFS{transport: transport, mount: mounted}, nil
}

func callURI(ctx context.Context, transport PluginTransport, request f4plugin.URIRequest, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if caller, ok := transport.(interface {
		CallContext(context.Context, string, any, any) error
	}); ok {
		return rpcVFSError(caller.CallContext(ctx, "VFS.URI", request, result))
	}
	return rpcVFSError(transport.Call("VFS.URI", request, result))
}

type rpcURIVFS struct {
	transport PluginTransport
	mu        sync.RWMutex
	mount     f4plugin.URIMount
}

func (v *rpcURIVFS) call(ctx context.Context, request f4plugin.URIRequest, result any) error {
	v.mu.RLock()
	request.Mount = v.mount.ID
	v.mu.RUnlock()
	return callURI(ctx, v.transport, request, result)
}
func (v *rpcURIVFS) SetPath(p string) error           { return v.setPath(p, "set") }
func (v *rpcURIVFS) SetPathOptimistic(p string) error { return v.setPath(p, "setOptimistic") }
func (v *rpcURIVFS) setPath(p, operation string) error {
	var mounted f4plugin.URIMount
	if err := v.call(context.Background(), f4plugin.URIRequest{Operation: operation, Path: p}, &mounted); err != nil {
		return err
	}
	v.mu.Lock()
	v.mount = mounted
	v.mu.Unlock()
	return nil
}
func (v *rpcURIVFS) GetPath() string { v.mu.RLock(); defer v.mu.RUnlock(); return v.mount.Path }
func (v *rpcURIVFS) IsAtRoot() bool  { v.mu.RLock(); defer v.mu.RUnlock(); return v.mount.Root }
func (v *rpcURIVFS) GetCapabilities() vfs.VFSCapabilities {
	var capabilities vfs.VFSCapabilities
	v.mu.RLock()
	wire := v.mount.Capabilities
	v.mu.RUnlock()
	encoded, err := msgpack.Marshal(wire)
	if err == nil {
		_ = msgpack.Unmarshal(encoded, &capabilities)
	}
	return capabilities
}
func (v *rpcURIVFS) pathOperation(operation, p string, elements []string) string {
	var result string
	if err := v.call(context.Background(), f4plugin.URIRequest{Operation: operation, Path: p, Elements: elements}, &result); err != nil {
		return ""
	}
	return result
}
func (v *rpcURIVFS) Join(elem ...string) string { return v.pathOperation("join", "", elem) }
func (v *rpcURIVFS) Base(p string) string       { return v.pathOperation("base", p, nil) }
func (v *rpcURIVFS) Dir(p string) string        { return v.pathOperation("dir", p, nil) }
func (v *rpcURIVFS) Abs(p string) (string, error) {
	var result string
	err := v.call(context.Background(), f4plugin.URIRequest{Operation: "abs", Path: p}, &result)
	return result, err
}
func (v *rpcURIVFS) IsAbs(p string) bool {
	var result bool
	_ = v.call(context.Background(), f4plugin.URIRequest{Operation: "isabs", Path: p}, &result)
	return result
}
func (v *rpcURIVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	items := []vfs.VFSItem{}
	err := v.call(ctx, f4plugin.URIRequest{Operation: "readDir", Path: p}, &items)
	if err == nil && onChunk != nil {
		onChunk(items)
	}
	return err
}
func (v *rpcURIVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	var item vfs.VFSItem
	err := v.call(ctx, f4plugin.URIRequest{Operation: "stat", Path: p}, &item)
	return item, err
}
func (v *rpcURIVFS) MkDir(ctx context.Context, p string) error {
	return v.call(ctx, f4plugin.URIRequest{Operation: "mkdir", Path: p}, nil)
}
func (v *rpcURIVFS) Remove(ctx context.Context, p string) error {
	return v.call(ctx, f4plugin.URIRequest{Operation: "remove", Path: p}, nil)
}
func (v *rpcURIVFS) Rename(ctx context.Context, old, new string) error {
	return v.call(ctx, f4plugin.URIRequest{Operation: "rename", Path: old, Other: new}, nil)
}
func (v *rpcURIVFS) SetAttributes(ctx context.Context, p string, item vfs.VFSItem) error {
	return v.call(ctx, f4plugin.URIRequest{Operation: "attributes", Path: p, Item: item}, nil)
}
func (v *rpcURIVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	var file f4plugin.URIFile
	if err := v.call(ctx, f4plugin.URIRequest{Operation: "open", Path: p}, &file); err != nil {
		return nil, err
	}
	return &rpcFileWrapper{sess: &uriFileTransport{mount: v, file: file.ID}, size: file.Size}, nil
}
func (v *rpcURIVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	var file f4plugin.URIFile
	if err := v.call(ctx, f4plugin.URIRequest{Operation: "create", Path: p}, &file); err != nil {
		return nil, err
	}
	return &rpcWriteWrapper{sess: &uriFileTransport{mount: v, file: file.ID}}, nil
}
func (v *rpcURIVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }
func (v *rpcURIVFS) Close() error {
	return v.call(context.Background(), f4plugin.URIRequest{Operation: "close"}, nil)
}
func (v *rpcURIVFS) related(operation string) vfs.VFS {
	var mounted f4plugin.URIMount
	if err := v.call(context.Background(), f4plugin.URIRequest{Operation: operation}, &mounted); err != nil || mounted.ID == 0 {
		return nil
	}
	return &rpcURIVFS{transport: v.transport, mount: mounted}
}
func (v *rpcURIVFS) Clone() vfs.VFS     { return v.related("clone") }
func (v *rpcURIVFS) ParentVFS() vfs.VFS { return v.related("parent") }

type uriFileTransport struct {
	mount *rpcURIVFS
	file  uint64
}

func (t *uriFileTransport) Call(method string, params any, result any) error {
	request := f4plugin.URIRequest{File: t.file}
	switch method {
	case "VFS.ReadAt":
		req := params.(ReadAtReq)
		request.Operation, request.Length, request.Offset = "readAt", req.Len, req.Off
	case "VFS.CloseFile":
		request.Operation = "closeFile"
	case "VFS.Write":
		req := params.(WriteReq)
		request.Operation, request.Data = "write", req.Data
		var written int
		if err := t.mount.call(context.Background(), request, &written); err != nil {
			return err
		}
		if written != len(req.Data) {
			return io.ErrShortWrite
		}
		return nil
	default:
		return fmt.Errorf("unsupported URI file operation %q", method)
	}
	return t.mount.call(context.Background(), request, result)
}
