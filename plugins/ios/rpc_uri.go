package iosfs

import (
	"context"
	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/sdk/f4vfs"
	"github.com/unxed/f4/vfs"
	"sync"
)

func (p *RPCPlugin) OpenVFSURI(ctx context.Context, raw string) (vfs.VFS, error) {
	return (&uriProvider{source: nativeDeviceSource{}, opener: p.backend}).OpenURI(ctx, nil, raw)
}

type uriRPCState struct {
	once   sync.Once
	bridge *f4vfs.Bridge
}

func (p *RPCPlugin) CallVFSURI(ctx context.Context, request f4plugin.URIRequest) (any, error) {
	p.uriRPCState.once.Do(func() { p.uriRPCState.bridge = f4vfs.NewBridge(p.OpenVFSURI) })
	return p.uriRPCState.bridge.CallVFSURI(ctx, request)
}
func (p *RPCPlugin) CloseVFSURIs() error {
	p.uriRPCState.once.Do(func() { p.uriRPCState.bridge = f4vfs.NewBridge(p.OpenVFSURI) })
	return p.uriRPCState.bridge.CloseVFSURIs()
}
