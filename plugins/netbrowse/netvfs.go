package netbrowse

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

// networkVFS is the network as a file system (f4#1702 part 2): the top of the
// path is the network itself -- providers, then domains or workgroups, then
// servers, then shares, each level enumerated the way the panel of part 1 does
// -- and inside a share the path continues into the share's own files, which
// are read and written through the ordinary file system at the share's UNC
// name. So a share is opened "as a normal directory", and F3, F5 and the rest
// of the file panel work on it.
//
// A path is "/" + segments; a resource's segment is the last part of its name
// ("Microsoft Windows Network", "WORKGROUP", "alpha" for \\alpha, "docs" for
// \\alpha\docs), matched without regard to case.
type networkVFS struct {
	mu   sync.Mutex
	enum enumerator
	open func(unc string) vfs.VFS // the file system at a UNC path
	// guess names a resource the enumeration does not list but a typed path
	// asks for (off Windows, a server the session has not met yet); nil means
	// only listed names exist.
	guess func(parent *resource, name string) *resource
	cwd   string
}

// errNotInShare is what a change to the network tree itself (a provider, a
// domain, a server) reports: only files inside a share can be changed.
var errNotInShare = errors.New("the network itself cannot be changed; only files inside a share can")

func newNetworkVFS(enum enumerator, open func(string) vfs.VFS) *networkVFS {
	return &networkVFS{enum: enum, open: open, cwd: "/"}
}

func openUNC(unc string) vfs.VFS { return vfs.NewOSVFS(unc) }

// segmentOf is a resource's name in a VFS path.
func segmentOf(r resource) string {
	name := strings.TrimLeft(r.Remote, `\`)
	if i := strings.LastIndex(name, `\`); i >= 0 {
		name = name[i+1:]
	}
	if name == "" {
		return r.Remote
	}
	return name
}

// node is where a path leads: a container of the network (res nil at the top),
// or a place inside a share (unc set).
type node struct {
	res *resource
	unc string
}

func splitPath(p string) []string {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// resolve walks a path down the network.
func (v *networkVFS) resolve(p string) (node, error) {
	var cur *resource
	parts := splitPath(p)
	for i, part := range parts {
		children, err := v.enum(cur)
		if err != nil {
			return node{}, err
		}
		var next *resource
		for j := range children {
			if strings.EqualFold(segmentOf(children[j]), part) {
				next = &children[j]
				break
			}
		}
		if next == nil && v.guess != nil {
			next = v.guess(cur, part)
		}
		if next == nil {
			return node{}, os.ErrNotExist
		}
		if !next.Container {
			// A share (or any leaf): the rest of the path is inside it.
			unc := next.Remote
			if rest := parts[i+1:]; len(rest) > 0 {
				unc += `\` + strings.Join(rest, `\`)
			}
			return node{unc: unc}, nil
		}
		cur = next
	}
	return node{res: cur}, nil
}

func (v *networkVFS) abs(p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return path.Join(v.cwd, p)
}

func (v *networkVFS) GetTitle() string { return "Network" }

func (v *networkVFS) IsAtRoot() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd == "/"
}

func (v *networkVFS) GetPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

func (v *networkVFS) IsAbs(p string) bool { return path.IsAbs(p) }
func (v *networkVFS) Join(e ...string) string {
	return path.Join(e...)
}
func (v *networkVFS) Base(p string) string { return path.Base(p) }
func (v *networkVFS) Dir(p string) string  { return path.Dir(p) }

func (v *networkVFS) Abs(p string) (string, error) { return v.abs(p), nil }

func (v *networkVFS) SetPath(p string) error {
	target := v.abs(p)
	n, err := v.resolve(target)
	if err != nil {
		return err
	}
	if n.unc != "" {
		item, err := v.open(n.unc).Stat(context.Background(), n.unc)
		if err != nil {
			return err
		}
		if !item.IsDir {
			return errors.New(target + " is not a directory")
		}
	}
	v.mu.Lock()
	v.cwd = target
	v.mu.Unlock()
	return nil
}

func networkItem(r resource) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit,
		Name:          segmentOf(r),
		IsDir:         true,
		NoExtension:   true,
	}
}

func (v *networkVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	n, err := v.resolve(v.abs(p))
	if err != nil {
		return err
	}
	if n.unc != "" {
		return v.open(n.unc).ReadDir(ctx, n.unc, onChunk)
	}
	children, err := v.enum(n.res)
	if err != nil {
		return err
	}
	items := make([]vfs.VFSItem, 0, len(children))
	for _, c := range children {
		items = append(items, networkItem(c))
	}
	onChunk(items)
	return nil
}

func (v *networkVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	n, err := v.resolve(v.abs(p))
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if n.unc != "" {
		return v.open(n.unc).Stat(ctx, n.unc)
	}
	if n.res == nil {
		return vfs.VFSItem{Name: "/", IsDir: true, NoExtension: true}, nil
	}
	return networkItem(*n.res), nil
}

// inShare resolves a path that must lie inside a share.
func (v *networkVFS) inShare(p string) (string, error) {
	abs := v.abs(p)
	n, err := v.resolve(abs)
	if errors.Is(err, os.ErrNotExist) {
		// A new name directly in the network tree (say a folder next to the
		// servers) is a change of the tree, not a missing entry.
		if parent, perr := v.resolve(path.Dir(abs)); perr == nil && parent.unc == "" {
			return "", errNotInShare
		}
	}
	if err != nil {
		return "", err
	}
	if n.unc == "" {
		return "", errNotInShare
	}
	return n.unc, nil
}

func (v *networkVFS) MkDir(ctx context.Context, p string) error {
	unc, err := v.inShare(p)
	if err != nil {
		return err
	}
	return v.open(unc).MkDir(ctx, unc)
}

func (v *networkVFS) Remove(ctx context.Context, p string) error {
	unc, err := v.inShare(p)
	if err != nil {
		return err
	}
	return v.open(unc).Remove(ctx, unc)
}

func (v *networkVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	from, err := v.inShare(oldPath)
	if err != nil {
		return err
	}
	to, err := v.inShare(newPath)
	if err != nil {
		return err
	}
	return v.open(from).Rename(ctx, from, to)
}

func (v *networkVFS) SetAttributes(ctx context.Context, p string, item vfs.VFSItem) error {
	unc, err := v.inShare(p)
	if err != nil {
		return err
	}
	return v.open(unc).SetAttributes(ctx, unc, item)
}

func (v *networkVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	unc, err := v.inShare(p)
	if err != nil {
		return nil, err
	}
	return v.open(unc).Open(ctx, unc)
}

func (v *networkVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	unc, err := v.inShare(p)
	if err != nil {
		return nil, err
	}
	return v.open(unc).Create(ctx, unc)
}

func (v *networkVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasWrite: true}
}

func (v *networkVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }
func (v *networkVFS) ParentVFS() vfs.VFS                                         { return nil }
func (v *networkVFS) Close() error                                               { return nil }

func (v *networkVFS) Clone() vfs.VFS {
	v.mu.Lock()
	defer v.mu.Unlock()
	return &networkVFS{enum: v.enum, open: v.open, guess: v.guess, cwd: v.cwd}
}
