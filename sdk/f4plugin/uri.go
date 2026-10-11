package f4plugin

import (
	"context"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/vmihailenco/msgpack/v5"
)

// VFSURIProvider is an optional wire-only extension. Native VFS types stay
// inside standalone plugins and never become dependencies of this SDK package.
type VFSURIProvider interface {
	CallVFSURI(context.Context, URIRequest) (any, error)
	CloseVFSURIs() error
}
type URIRequest struct {
	Mount       uint64
	Operation   string
	Path, Other string
	Elements    []string
	File        uint64
	Offset      int64
	Length      int
	Data        []byte
	Item        any
}
type URIMount struct {
	ID           uint64
	Path         string
	Root         bool
	Capabilities any
}
type URIFile struct {
	ID   uint64
	Size int64
}

func registerVFSURIs(sess *f4rpc.Session, plugin Plugin) func() {
	provider, ok := plugin.(VFSURIProvider)
	if !ok {
		return func() {}
	}
	sess.Register("VFS.URI", func(raw msgpack.RawMessage) (any, error) {
		var req URIRequest
		if err := msgpack.Unmarshal(raw, &req); err != nil {
			return nil, err
		}
		return provider.CallVFSURI(context.Background(), req)
	})
	return func() { _ = provider.CloseVFSURIs() }
}
