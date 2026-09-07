package archive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/unxed/archives"
	zipperarchive "github.com/unxed/zipper/archive"
)

// rarArchiveFileSystem keeps the volume-aware RAR reader on the filesystem
// path. zipper's generic fallback receives only one input stream, while
// rardecode needs the first volume name and its containing directory to find
// the following volumes.
type rarArchiveFileSystem struct {
	root   *archives.ArchiveFS
	format archives.Rar

	mu     sync.RWMutex
	closed bool
}

func newRARArchiveFileSystem(localPath, password string) (zipperarchive.FileSystem, error) {
	if _, err := os.Stat(localPath); err != nil {
		return nil, err
	}

	format := archives.Rar{
		Name:     filepath.Base(localPath),
		FS:       archives.DirFS(filepath.Dir(localPath)),
		Password: password,
	}
	return &rarArchiveFileSystem{
		root: &archives.ArchiveFS{
			Path:    localPath,
			Format:  format,
			Context: context.Background(),
		},
		format: format,
	}, nil
}

func (r *rarArchiveFileSystem) rootFS() (*archives.ArchiveFS, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return nil, errors.New("RAR filesystem is closed")
	}
	return r.root, nil
}

func (r *rarArchiveFileSystem) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	root, err := r.rootFS()
	if err != nil {
		return nil, err
	}

	info, err := root.Stat(name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		entries, err := root.ReadDir(name)
		if err != nil {
			return nil, err
		}
		return &rarDirectoryFile{info: info, entries: append([]fs.DirEntry(nil), entries...)}, nil
	}

	tmp, err := os.CreateTemp("", "f4arc-rar-member-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	target := path.Clean(name)
	found := false
	err = r.format.Extract(context.Background(), nil, func(_ context.Context, entry archives.FileInfo) error {
		if path.Clean(entry.NameInArchive) != target {
			return nil
		}
		if entry.IsDir() {
			return fmt.Errorf("RAR entry %q is a directory", name)
		}

		member, err := entry.Open()
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(tmp, member)
		closeErr := member.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		found = true
		return fs.SkipAll
	})
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fmt.Errorf("extract: %w", err)}
	}
	if !found {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}
	file, err := os.Open(tmpName)
	if err != nil {
		return nil, err
	}
	removeTemp = false
	return &rarMemberFile{File: file, info: info, tempPath: tmpName}, nil
}

func (r *rarArchiveFileSystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	root, err := r.rootFS()
	if err != nil {
		return nil, err
	}
	return root.ReadDir(name)
}

func (r *rarArchiveFileSystem) Stat(name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	root, err := r.rootFS()
	if err != nil {
		return nil, err
	}
	return root.Stat(name)
}

func (r *rarArchiveFileSystem) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}

type rarMemberFile struct {
	*os.File
	info     fs.FileInfo
	tempPath string
	once     sync.Once
	err      error
}

func (f *rarMemberFile) Stat() (fs.FileInfo, error) { return f.info, nil }

func (f *rarMemberFile) Close() error {
	f.once.Do(func() {
		closeErr := f.File.Close()
		removeErr := os.Remove(f.tempPath)
		f.err = errors.Join(closeErr, removeErr)
	})
	return f.err
}

type rarDirectoryFile struct {
	info        fs.FileInfo
	entries     []fs.DirEntry
	entriesRead int
}

func (rarDirectoryFile) Read([]byte) (int, error) {
	return 0, errors.New("cannot read a directory file")
}

func (f *rarDirectoryFile) Stat() (fs.FileInfo, error) { return f.info, nil }
func (rarDirectoryFile) Close() error                  { return nil }

func (f *rarDirectoryFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		entries := f.entries[f.entriesRead:]
		f.entriesRead = len(f.entries)
		return entries, nil
	}
	if f.entriesRead >= len(f.entries) {
		return nil, io.EOF
	}
	end := f.entriesRead + n
	if end > len(f.entries) {
		end = len(f.entries)
	}
	entries := f.entries[f.entriesRead:end]
	f.entriesRead = end
	return entries, nil
}

func openArchiveFileSystem(ctx context.Context, localPath, displayName, password string) (zipperarchive.FileSystem, error) {
	format, err := identifyArchiveFormat(ctx, localPath, displayName)
	if err == nil {
		switch format.(type) {
		case archives.Rar, *archives.Rar:
			return newRARArchiveFileSystem(localPath, password)
		}
	}
	return zipperarchive.OpenFS(localPath, zipperarchive.Options{Password: password})
}

func identifyArchiveFormat(ctx context.Context, localPath, displayName string) (archives.Format, error) {
	file, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	if displayName == "" {
		displayName = filepath.Base(localPath)
	}
	format, _, err := archives.Identify(ctx, displayName, file)
	return format, err
}

func configureRARArchiveFormat(format archives.Format, localPath, password string) (archives.Format, bool) {
	switch value := format.(type) {
	case archives.Rar:
		value.Name = filepath.Base(localPath)
		value.FS = archives.DirFS(filepath.Dir(localPath))
		value.Password = password
		return value, true
	case *archives.Rar:
		if value == nil {
			return format, false
		}
		copy := *value
		copy.Name = filepath.Base(localPath)
		copy.FS = archives.DirFS(filepath.Dir(localPath))
		copy.Password = password
		return &copy, true
	default:
		return format, false
	}
}

var _ zipperarchive.FileSystem = (*rarArchiveFileSystem)(nil)
