//go:build !lite

// SMB support statically links github.com/cloudsoda/go-smb2 (smb_backend.go),
// which a lite build (f4#1178, f4#1671) exists to shed.

package netfox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

// errSMBNoShare is what a mutation reports when its path is the server's root
// or a share itself: shares are made and removed on the server, not through
// the file panel.
var errSMBNoShare = errors.New("this operation needs a path inside a share")

// errSMBOtherShare is what Rename reports between two shares: one rename is
// one tree connect on the wire, so it cannot cross them.
var errSMBOtherShare = errors.New("cannot rename between shares; copy and delete instead")

// smbFile is an open remote file.
type smbFile interface {
	ReadAt(p []byte, off int64) (int, error)
	Read(p []byte) (int, error)
	Close() error
	Size() int64
}

// smbBackend is what smbVFS needs from an SMB connection, so the VFS is
// tested against a fake and only smbClient (smb_backend.go) touches the
// network. Paths inside a share use "/" and never start with one; "" is the
// share's root.
type smbBackend interface {
	ListShares() ([]string, error)
	ReadDir(share, dir string) ([]fs.FileInfo, error)
	Stat(share, name string) (fs.FileInfo, error)
	OpenRead(share, name string) (smbFile, error)
	MkDir(share, dir string) error
	// RemoveAll deletes a file, or a directory with everything in it.
	RemoveAll(share, name string) error
	Rename(share, oldName, newName string) error
	// Create makes (or truncates) a file and returns a writer for it.
	Create(share, name string) (io.WriteCloser, error)
	Close() error
}

// smbSession shares one backend between a VFS and its clones, closing it with
// the last of them.
type smbSession struct {
	mu      sync.Mutex
	backend smbBackend
	refs    int
}

func (s *smbSession) retain() {
	s.mu.Lock()
	s.refs++
	s.mu.Unlock()
}

func (s *smbSession) release() error {
	s.mu.Lock()
	s.refs--
	last := s.refs <= 0
	s.mu.Unlock()
	if last {
		return s.backend.Close()
	}
	return nil
}

// smbVFS is a view of an SMB server: the root lists its shares as
// directories, "/share/dir/file" goes into one of them.
type smbVFS struct {
	mu      sync.Mutex
	parent  vfs.VFS
	session *smbSession
	cwd     string
	title   string
	closed  sync.Once
}

func newSMBVFS(parent vfs.VFS, backend smbBackend, title string) *smbVFS {
	return &smbVFS{parent: parent, session: &smbSession{backend: backend, refs: 1}, cwd: "/", title: title}
}

// splitSMBPath turns an absolute path into its share and the path inside it.
func splitSMBPath(p string) (share, rel string) {
	p = strings.Trim(path.Clean("/"+p), "/")
	if p == "" {
		return "", ""
	}
	share, rel, _ = strings.Cut(p, "/")
	return share, rel
}

func (v *smbVFS) backend() smbBackend { return v.session.backend }

func (v *smbVFS) GetTitle() string { return v.title }

func (v *smbVFS) IsAtRoot() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd == "/"
}

func (v *smbVFS) GetPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

func (v *smbVFS) IsAbs(p string) bool { return path.IsAbs(p) }

func (v *smbVFS) abs(p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	return path.Join(v.cwd, p)
}

func (v *smbVFS) Abs(p string) (string, error) { return v.abs(p), nil }

func (v *smbVFS) SetPath(p string) error {
	target := v.abs(p)
	item, err := v.Stat(context.Background(), target)
	if err != nil {
		return err
	}
	if !item.IsDir {
		return fmt.Errorf("%s is not a directory", target)
	}
	v.mu.Lock()
	v.cwd = target
	v.mu.Unlock()
	return nil
}

func smbItem(info fs.FileInfo) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataHidden | vfs.MetadataMTime,
		SizeKnown:     true,
		Name:          info.Name(),
		Size:          info.Size(),
		IsDir:         info.IsDir(),
		MTime:         info.ModTime(),
		IsHidden:      strings.HasPrefix(info.Name(), "."),
	}
}

func (v *smbVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	share, rel := splitSMBPath(v.abs(p))
	if share == "" {
		names, err := v.backend().ListShares()
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(names))
		for _, n := range names {
			items = append(items, vfs.VFSItem{
				KnownMetadata: vfs.MetadataExplicit, Name: n, IsDir: true, NoExtension: true,
				IsHidden: strings.HasSuffix(n, "$"),
			})
		}
		onChunk(items)
		return nil
	}
	entries, err := v.backend().ReadDir(share, rel)
	if err != nil {
		return err
	}
	const chunk = 500
	items := make([]vfs.VFSItem, 0, min(len(entries), chunk))
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Name() == "." || e.Name() == ".." {
			continue
		}
		items = append(items, smbItem(e))
		if len(items) == chunk {
			onChunk(items)
			items = make([]vfs.VFSItem, 0, chunk)
		}
	}
	if len(items) > 0 {
		onChunk(items)
	}
	return nil
}

func (v *smbVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs := v.abs(p)
	share, rel := splitSMBPath(abs)
	switch {
	case share == "":
		return vfs.VFSItem{Name: "/", IsDir: true, NoExtension: true}, nil
	case rel == "":
		names, err := v.backend().ListShares()
		if err != nil {
			return vfs.VFSItem{}, err
		}
		for _, n := range names {
			if strings.EqualFold(n, share) {
				return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: n, IsDir: true, NoExtension: true}, nil
			}
		}
		return vfs.VFSItem{}, os.ErrNotExist
	}
	info, err := v.backend().Stat(share, rel)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	return smbItem(info), nil
}

func (v *smbVFS) Join(e ...string) string { return path.Join(e...) }
func (v *smbVFS) Base(p string) string    { return path.Base(p) }
func (v *smbVFS) Dir(p string) string     { return path.Dir(p) }

// inShare splits an absolute path and refuses the root and a share itself.
func (v *smbVFS) inShare(p string) (share, rel string, err error) {
	share, rel = splitSMBPath(v.abs(p))
	if share == "" || rel == "" {
		return "", "", errSMBNoShare
	}
	return share, rel, nil
}

func (v *smbVFS) MkDir(_ context.Context, p string) error {
	share, rel, err := v.inShare(p)
	if err != nil {
		return err
	}
	return v.backend().MkDir(share, rel)
}

func (v *smbVFS) Remove(_ context.Context, p string) error {
	share, rel, err := v.inShare(p)
	if err != nil {
		return err
	}
	return v.backend().RemoveAll(share, rel)
}

func (v *smbVFS) Rename(_ context.Context, oldPath, newPath string) error {
	oldShare, oldRel, err := v.inShare(oldPath)
	if err != nil {
		return err
	}
	newShare, newRel, err := v.inShare(newPath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(oldShare, newShare) {
		return errSMBOtherShare
	}
	return v.backend().Rename(oldShare, oldRel, newRel)
}

// SetAttributes is not supported: SMB times and attributes are not mapped to
// f4's item fields yet.
func (v *smbVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errors.New("SetAttributes is not supported for SMB")
}

func (v *smbVFS) Create(_ context.Context, p string) (io.WriteCloser, error) {
	share, rel, err := v.inShare(p)
	if err != nil {
		return nil, err
	}
	return v.backend().Create(share, rel)
}

func (v *smbVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasWrite: true}
}

func (v *smbVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

// smbReadHandle adapts an smbFile to vfs.ReadAtCloser.
type smbReadHandle struct{ f smbFile }

func (h smbReadHandle) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	return h.f.ReadAt(p, off)
}
func (h smbReadHandle) Read(_ context.Context, p []byte) (int, error) { return h.f.Read(p) }
func (h smbReadHandle) Close() error                                  { return h.f.Close() }
func (h smbReadHandle) Size() int64                                   { return h.f.Size() }

func (v *smbVFS) Open(_ context.Context, p string) (vfs.ReadAtCloser, error) {
	share, rel := splitSMBPath(v.abs(p))
	if share == "" || rel == "" {
		return nil, fmt.Errorf("%s is not a file", v.abs(p))
	}
	f, err := v.backend().OpenRead(share, rel)
	if err != nil {
		return nil, err
	}
	return smbReadHandle{f: f}, nil
}

func (v *smbVFS) ParentVFS() vfs.VFS { return v.parent }

func (v *smbVFS) Close() error {
	if v == nil {
		return nil
	}
	var err error
	v.closed.Do(func() { err = v.session.release() })
	return err
}

func (v *smbVFS) Clone() vfs.VFS {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.session.retain()
	return &smbVFS{parent: v.parent, session: v.session, cwd: v.cwd, title: v.title}
}
