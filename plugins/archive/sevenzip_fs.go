package archive

import (
	"errors"
	"io/fs"
	"path"
	"sync"

	"github.com/unxed/sevenzip"
	zipperarchive "github.com/unxed/zipper/archive"
)

// sevenZipFileSystem keeps one sevenzip.Reader for the whole life of an
// opened 7z. The generic filesystem built by zipper parses the archive
// headers again, and starts a new decoder, for every member that is opened
// (see #1731); here the headers are read once, and a decoder left in the
// middle of a solid block by the previous member is reused by the next one.
//
// The Reader and the input behind it are released when the last member opened
// from this filesystem is closed after Close: installing a password replaces
// the filesystem while a handle taken from the old one may still be read.
type sevenZipFileSystem struct {
	in *zipperarchive.Input
	zr *sevenzip.Reader

	mu     sync.Mutex
	open   int
	closed bool
	done   bool
	err    error
}

// newSevenZipFileSystem reads the headers of the 7z at localPath. A failure
// (including a wrong password for encrypted headers) is returned as is, so the
// caller can fall back to the generic filesystem and keep its error handling.
func newSevenZipFileSystem(localPath, password string) (*sevenZipFileSystem, error) {
	in, err := zipperarchive.OpenInput(localPath)
	if err != nil {
		return nil, err
	}
	zr, err := sevenzip.NewReaderWithPassword(in.SectionReader, in.Size(), password)
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	return &sevenZipFileSystem{in: in, zr: zr}, nil
}

var errSevenZipClosed = errors.New("7z filesystem is closed")

func (s *sevenZipFileSystem) Open(name string) (fs.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, &fs.PathError{Op: "open", Path: name, Err: errSevenZipClosed}
	}
	file, err := s.zr.Open(name)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.IsDir() {
		return file, nil
	}
	s.open++
	return &sevenZipMember{File: file, owner: s}, nil
}

func (s *sevenZipFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errSevenZipClosed}
	}
	return fs.ReadDir(s.zr, name)
}

// Stat answers from the parent directory listing: fs.Stat would open the
// member, which starts a decoder for it.
func (s *sevenZipFileSystem) Stat(name string) (fs.FileInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: errSevenZipClosed}
	}
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		dir, err := s.zr.Open(name)
		if err != nil {
			return nil, err
		}
		defer func() { _ = dir.Close() }()
		return dir.Stat()
	}
	entries, err := fs.ReadDir(s.zr, path.Dir(name))
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	base := path.Base(name)
	for _, entry := range entries {
		if entry.Name() == base {
			return entry.Info()
		}
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// Close releases the decoders and the input once no member is open.
func (s *sevenZipFileSystem) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.releaseLocked()
}

func (s *sevenZipFileSystem) releaseLocked() error {
	if s.open > 0 || s.done {
		return s.err
	}
	s.done = true
	s.err = errors.Join(s.zr.ReleasePool(), s.in.Close())
	return s.err
}

func (s *sevenZipFileSystem) memberClosed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open--
	if s.closed {
		_ = s.releaseLocked()
	}
}

// sevenZipMember tells the filesystem when a member handle is closed, so the
// decoder it leaves in the pool is released after the filesystem was closed.
type sevenZipMember struct {
	fs.File
	owner *sevenZipFileSystem
	once  sync.Once
	err   error
}

func (m *sevenZipMember) Close() error {
	m.once.Do(func() {
		m.err = m.File.Close()
		m.owner.memberClosed()
	})
	return m.err
}

var _ zipperarchive.FileSystem = (*sevenZipFileSystem)(nil)
