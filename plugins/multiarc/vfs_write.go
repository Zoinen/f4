package multiarc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

// errNoRename answers Rename: zip, tar and gzip have no way to rename a
// member in place, and a delete-then-add stand-in would recompress the
// member and, for a directory, every member under it.
var errNoRename = errors.New("multiarc: members cannot be renamed inside an archive")

// The writes below are what panel operations on an opened archive turn
// into: F5 onto it is Stat, MkDir and Create for each copied item, F7 is
// MkDir, F8 is one Remove per selected item, and saving a member from the
// editor is Create. Each is one archiver run (see archiveWriter), made
// under the archive's lock and followed by a fresh listing that every
// clone of this VFS then sees.

// writer returns the archiveWriter side of the backend.
func (v *MultiArcVFS) writer() (archiveWriter, error) {
	if w, ok := v.backend.(archiveWriter); ok {
		return w, nil
	}
	return nil, fmt.Errorf("multiarc: %s archives cannot be changed", v.backendID)
}

// writableKey lists the archive if nothing has yet and returns the member
// key p names, refusing the archive root: it is the archive file itself,
// not a member of it.
func (v *MultiArcVFS) writableKey(ctx context.Context, p string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := v.state.ensureListed(ctx, v.backend, v.localPath); err != nil {
		return "", err
	}
	key := v.key(p)
	if key == "" {
		return "", errors.New("multiarc: the archive root itself cannot be changed")
	}
	return key, nil
}

// lookupLocked reports whether the listing has anything at key and whether
// that is a directory, one the archive names or one only its members'
// paths imply. The caller holds v.state.mu.
func (v *MultiArcVFS) lookupLocked(key string) (exists, isDir bool) {
	info, isEntry := v.state.entries[key]
	_, hasChildren := v.state.children[key]
	if isEntry {
		return true, info.IsDir || hasChildren
	}
	return hasChildren, hasChildren
}

// fileAncestorLocked returns the first parent of key that is a file member:
// "a.txt/b" cannot be created while "a.txt" is a file. The caller holds
// v.state.mu.
func (v *MultiArcVFS) fileAncestorLocked(key string) (string, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] != '/' {
			continue
		}
		if exists, isDir := v.lookupLocked(key[:i]); exists && !isDir {
			return key[:i], true
		}
	}
	return "", false
}

// subtreeRawsLocked returns the raw name of every entry at key or under it,
// sorted. The caller holds v.state.mu.
func (v *MultiArcVFS) subtreeRawsLocked(key string) []string {
	var raws []string
	for p, names := range v.state.raws {
		if p == key || strings.HasPrefix(p, key+"/") {
			raws = append(raws, names...)
		}
	}
	sort.Strings(raws)
	return raws
}

// checkNewMember is the shared precondition of MkDir and Create: the
// member's parents must not run through a file.
func (v *MultiArcVFS) checkNewMember(key string) (exists, isDir bool, err error) {
	v.state.mu.Lock()
	defer v.state.mu.Unlock()
	if parent, blocked := v.fileAncestorLocked(key); blocked {
		return false, false, fmt.Errorf("multiarc: %s is a file, not a directory", parent)
	}
	exists, isDir = v.lookupLocked(key)
	return exists, isDir, nil
}

// commit runs change under the archive's lock -- vfs.GlobalArchiveLockManager,
// which every archive write in f4 takes for the file it writes, so no two
// writes to it interleave -- and then lists the archive again. It relists after a failure too: a change split over several
// command lines (runChunked) may have committed its first ones before a
// later one failed. The relisting does not stop when ctx is canceled, so
// the panel never keeps showing members that are gone.
func (v *MultiArcVFS) commit(ctx context.Context, change func() error) error {
	vfs.GlobalArchiveLockManager.Lock(v.localPath)
	defer vfs.GlobalArchiveLockManager.Unlock(v.localPath)
	err := change()
	if relistErr := v.state.relist(context.WithoutCancel(ctx), v.backend, v.localPath); err == nil {
		err = relistErr
	}
	return err
}

// stageDirFor makes the private directory a change is staged in. It sits
// next to the archive rather than in the system temp directory for the
// reason workDirNextTo gives: on a router the latter is RAM.
func (v *MultiArcVFS) stageDirFor() (string, error) {
	return workDirNextTo(v.localPath)
}

// MkDir adds an empty directory member.
func (v *MultiArcVFS) MkDir(ctx context.Context, p string) error {
	key, err := v.writableKey(ctx, p)
	if err != nil {
		return err
	}
	exists, _, err := v.checkNewMember(key)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("multiarc: %s: %w", p, os.ErrExist)
	}
	w, err := v.writer()
	if err != nil {
		return err
	}
	if err := w.checkWrite(ctx, v.localPath, writeMkDir, key); err != nil {
		return err
	}
	stageDir, err := v.stageDirFor()
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stageDir) }() // Scratch; the archive has its own copy now.
	// #nosec G301 -- this is the mode the archive records for the new directory, not a directory f4 keeps.
	if err := os.MkdirAll(filepath.Join(stageDir, filepath.FromSlash(key)), 0o755); err != nil {
		return err
	}
	return v.commit(ctx, func() error {
		return w.add(ctx, v.localPath, stageDir, []string{key}, nil)
	})
}

// Remove deletes the member at p, and everything under it when it is a
// directory: the delete engine calls Remove once for each selected item
// and leaves the recursion to the VFS.
func (v *MultiArcVFS) Remove(ctx context.Context, p string) error {
	key, err := v.writableKey(ctx, p)
	if err != nil {
		return err
	}
	v.state.mu.Lock()
	raws := v.subtreeRawsLocked(key)
	v.state.mu.Unlock()
	if len(raws) == 0 {
		return fmt.Errorf("multiarc: %s: %w", p, os.ErrNotExist)
	}
	w, err := v.writer()
	if err != nil {
		return err
	}
	if err := w.checkWrite(ctx, v.localPath, writeRemove, key); err != nil {
		return err
	}
	return v.commit(ctx, func() error {
		return w.remove(ctx, v.localPath, raws)
	})
}

func (v *MultiArcVFS) Rename(context.Context, string, string) error { return errNoRename }

// Create stages a new file member, or new content for an existing one. The
// bytes go to a file in a staging directory; Close hands that file to the
// archiver, Abort throws it away. Whether the backend can do this at all is
// checked here, before the caller has copied a byte.
func (v *MultiArcVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	key, err := v.writableKey(ctx, p)
	if err != nil {
		return nil, err
	}
	exists, isDir, err := v.checkNewMember(key)
	if err != nil {
		return nil, err
	}
	if isDir {
		return nil, fmt.Errorf("multiarc: %s is a directory", p)
	}
	w, err := v.writer()
	if err != nil {
		return nil, err
	}
	op := writeAdd
	if exists {
		op = writeReplace
	}
	if err := w.checkWrite(ctx, v.localPath, op, key); err != nil {
		return nil, err
	}
	stageDir, err := v.stageDirFor()
	if err != nil {
		return nil, err
	}
	staged := filepath.Join(stageDir, filepath.FromSlash(key))
	// #nosec G301 -- only the file inside is archived; its parents are scratch.
	if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
		_ = os.RemoveAll(stageDir)
		return nil, err
	}
	// os.Create's 0666 less the umask is the mode a new member gets, the
	// same as a file any other program creates.
	f, err := os.Create(filepath.Clean(staged))
	if err != nil {
		_ = os.RemoveAll(stageDir)
		return nil, err
	}
	return &stagedMember{v: v, w: w, ctx: ctx, stageDir: stageDir, key: key, file: f}, nil
}

// stagedMember is the writer Create returns.
type stagedMember struct {
	v        *MultiArcVFS
	w        archiveWriter
	ctx      context.Context
	stageDir string
	key      string
	file     *os.File

	once sync.Once
	err  error
}

func (s *stagedMember) Write(p []byte) (int, error) { return s.file.Write(p) }

// Close stores the staged file in the archive, replacing any member of the
// same name. Which raw names it replaces is read under the archive's lock,
// so a member another clone added meanwhile is replaced too, not left as a
// duplicate.
func (s *stagedMember) Close() error {
	s.once.Do(func() {
		defer func() { _ = os.RemoveAll(s.stageDir) }() // Scratch; committed or not, it is done.
		if err := s.file.Close(); err != nil {
			s.err = err
			return
		}
		s.err = s.v.commit(s.ctx, func() error {
			s.v.state.mu.Lock()
			replaced := append([]string(nil), s.v.state.raws[s.key]...)
			s.v.state.mu.Unlock()
			return s.w.add(s.ctx, s.v.localPath, s.stageDir, []string{s.key}, replaced)
		})
	})
	return s.err
}

// Abort discards the staged file without touching the archive, which is
// what the copy engine asks for when the source failed mid-file. A Close
// after it does nothing.
func (s *stagedMember) Abort() error {
	s.once.Do(func() {
		_ = s.file.Close() // Discarded with its directory right below.
		s.err = os.RemoveAll(s.stageDir)
	})
	return s.err
}

var _ vfs.AbortableWriter = (*stagedMember)(nil)
