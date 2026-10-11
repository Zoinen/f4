package netbrowse

import (
	"context"
	"net/url"
	"strings"

	"github.com/unxed/f4/vfs"
)

// uriProvider opens network://[provider/domain/server/share/path] as the
// network file system of netvfs.go (f4#1702 part 2b), so the network can be
// reached from a command line, a bookmark or the go-to dialog as well as from
// the drive menu.
type uriProvider struct {
	enum  enumerator
	open  func(string) vfs.VFS
	guess func(parent *resource, name string) *resource
}

func (p *uriProvider) Scheme() string { return "network" }

func (p *uriProvider) OpenURI(_ context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	v := newNetworkVFS(p.enum, p.open)
	v.guess = p.guess
	rest := strings.TrimPrefix(raw, "network:")
	rest = strings.TrimLeft(rest, "/")
	if unescaped, err := url.PathUnescape(rest); err == nil {
		rest = unescaped
	}
	if rest != "" {
		if err := v.SetPath("/" + rest); err != nil {
			_ = v.Close()
			return nil, err
		}
	}
	return v, nil
}
