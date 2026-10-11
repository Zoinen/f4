package pdfview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/unxed/f4/internal/pdftext"
	"github.com/unxed/f4/vfs"
)

var errReadOnly = errors.New("PDF panel is read-only")

// node is one folder or file of the document's tree.
type node struct {
	dir   bool
	data  []byte
	names []string
	kids  map[string]*node
}

func newDir() *node { return &node{dir: true, kids: make(map[string]*node)} }

// add puts a child under n, keeping the first of two equal names apart from
// the second by a numeric suffix.
func (n *node) add(name string, child *node) {
	name = strings.NewReplacer("/", "_", "\\", "_", "\x00", "_").Replace(name)
	if name == "" {
		name = "_"
	}
	unique := name
	for i := 2; n.kids[unique] != nil && i < 1000; i++ {
		unique = fmt.Sprintf("%s (%d)", name, i)
	}
	if n.kids[unique] != nil {
		return
	}
	n.kids[unique] = child
	n.names = append(n.names, unique)
}

func file(text string) *node { return &node{data: []byte(text)} }

// buildTree lays a PDF out as folders: text.md (the whole text as the F3
// viewer shows it), Pages/page-NNN.txt and Images/pNNN-<name>.<ext>.
func buildTree(res *pdftext.Result, images []pdftext.Image, name string) *node {
	root := newDir()
	root.add("text.md", file(pdftext.Report(res, name)))
	pages := newDir()
	for i, text := range res.Pages {
		pages.add(fmt.Sprintf("page-%03d.txt", i+1), file(text+"\n"))
	}
	root.add("Pages", pages)
	if len(images) > 0 {
		dir := newDir()
		for _, img := range images {
			dir.add(fmt.Sprintf("p%03d-%s.%s", img.Page, img.Name, img.Ext), &node{data: img.Data})
		}
		root.add("Images", dir)
	}
	return root
}

// documentVFS is a read-only vfs.VFS over the tree of one PDF.
type documentVFS struct {
	parent vfs.VFS
	name   string
	root   *node
	path   string
}

func newDocumentVFS(parent vfs.VFS, name string, res *pdftext.Result, images []pdftext.Image) *documentVFS {
	return &documentVFS{parent: parent, name: name, root: buildTree(res, images, name), path: "/"}
}

func (v *documentVFS) GetTitle() string { return v.name }

func (v *documentVFS) PanelTitle(p string) string {
	key := v.key(p)
	if key == "" {
		return "PDF:" + v.name
	}
	return "PDF:" + v.name + "/" + key
}

func (v *documentVFS) IsAtRoot() bool             { return v.path == "" || v.path == "/" }
func (v *documentVFS) GetPath() string            { return v.path }
func (v *documentVFS) IsAbs(p string) bool        { return path.IsAbs(p) }
func (v *documentVFS) Join(elem ...string) string { return path.Join(elem...) }
func (v *documentVFS) Base(p string) string       { return path.Base(p) }
func (v *documentVFS) Dir(p string) string        { return path.Dir(p) }

func (v *documentVFS) abs(p string) string {
	if p == "" {
		return v.path
	}
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(v.path, p)
}

func (v *documentVFS) key(p string) string { return strings.Trim(v.abs(p), "/") }

func (v *documentVFS) Abs(p string) (string, error) { return v.abs(p), nil }

// lookup finds the node at a slash-separated key ("" is the root).
func (v *documentVFS) lookup(key string) *node {
	n := v.root
	if key == "" {
		return n
	}
	for _, part := range strings.Split(key, "/") {
		if n == nil || !n.dir {
			return nil
		}
		n = n.kids[part]
	}
	return n
}

func (v *documentVFS) SetPath(p string) error {
	key := v.key(p)
	if n := v.lookup(key); n == nil || !n.dir {
		return os.ErrInvalid
	}
	v.path = "/" + key
	return nil
}

func itemOf(name string, n *node) vfs.VFSItem {
	item := vfs.VFSItem{Name: name, IsDir: n.dir}
	if !n.dir {
		item.Size = int64(len(n.data))
		item.SizeKnown = true
	}
	return item
}

func (v *documentVFS) ReadDir(_ context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	n := v.lookup(v.key(p))
	if n == nil || !n.dir {
		return os.ErrNotExist
	}
	items := make([]vfs.VFSItem, 0, len(n.names))
	for _, name := range n.names {
		items = append(items, itemOf(name, n.kids[name]))
	}
	if onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (v *documentVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	key := v.key(p)
	n := v.lookup(key)
	if n == nil {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	name := path.Base("/" + key)
	if key == "" {
		name = v.name
	}
	return itemOf(name, n), nil
}

func (v *documentVFS) MkDir(context.Context, string) error          { return errReadOnly }
func (v *documentVFS) Remove(context.Context, string) error         { return errReadOnly }
func (v *documentVFS) Rename(context.Context, string, string) error { return errReadOnly }
func (v *documentVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errReadOnly
}
func (v *documentVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, errReadOnly
}

func (v *documentVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}

func (v *documentVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

// memFile serves the bytes of one node as a vfs.ReadAtCloser.
type memFile struct {
	data []byte
	pos  int64
}

func (m *memFile) Size() int64  { return int64(len(m.data)) }
func (m *memFile) Close() error { return nil }

func (m *memFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memFile) Read(ctx context.Context, p []byte) (int, error) {
	n, err := m.ReadAt(ctx, p, m.pos)
	m.pos += int64(n)
	return n, err
}

func (v *documentVFS) Open(_ context.Context, p string) (vfs.ReadAtCloser, error) {
	n := v.lookup(v.key(p))
	if n == nil || n.dir {
		return nil, os.ErrNotExist
	}
	return &memFile{data: n.data}, nil
}

func (v *documentVFS) ParentVFS() vfs.VFS { return v.parent }

func (v *documentVFS) Clone() vfs.VFS {
	clone := *v
	return &clone
}

func (v *documentVFS) Close() error { return nil }

var (
	_ vfs.VFS                = (*documentVFS)(nil)
	_ vfs.TitleProvider      = (*documentVFS)(nil)
	_ vfs.PanelTitleProvider = (*documentVFS)(nil)
)
