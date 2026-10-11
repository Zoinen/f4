package k8sfs

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/unxed/f4/vfs"
)

// uriProvider opens k8s:///<path> as the Kubernetes panel at that folder, so a
// bookmark, a folder-history entry or a restored session (f4#1669) can bring
// the panel back. Which server it talks to is not in the URI: it is the same
// one the drive menu entry uses.
type uriProvider struct {
	open func() (*restClient, error)
	// openContext connects to a named kubeconfig context (nil: none).
	openContext func(name string) func() (*restClient, error)
}

func (uriProvider) Scheme() string { return "k8s" }

func (p uriProvider) OpenURI(ctx context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	if len(raw) < len(uriPrefix) || !strings.EqualFold(raw[:len(uriPrefix)], uriPrefix) {
		return nil, fmt.Errorf("Kubernetes: not a k8s:// address: %s", raw)
	}
	rest := raw[len(uriPrefix):]
	open, prefix := p.open, uriPrefix
	if rest != "" && rest[0] != '/' {
		// k8s://<context>/<path>
		name, tail, _ := strings.Cut(rest, "/")
		ctx, err := url.PathUnescape(name)
		if err != nil || ctx == "" || p.openContext == nil {
			return nil, fmt.Errorf("Kubernetes: bad kubeconfig context in %s", raw)
		}
		open, prefix, rest = p.openContext(ctx), contextPrefix(ctx), "/"+tail
	}
	plain := path.Clean("/" + strings.TrimPrefix(rest, "/"))
	v := newK8sVFS(open)
	v.prefix = prefix
	item, err := v.Stat(ctx, plain)
	if err != nil {
		_ = v.Close()
		return nil, err
	}
	if !item.IsDir {
		_ = v.Close()
		return nil, fmt.Errorf("%s: %w", plain, errNotADirectory)
	}
	v.mu.Lock()
	v.cwd = plain
	v.mu.Unlock()
	return v, nil
}
