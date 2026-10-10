package ap

// Transactions and undo (docs/VTVIBE.md §7.4, f4#1606): before a real Apply
// writes anything it snapshots every path the write plan is about to touch,
// and the snapshot becomes Result.Undo once the writes are done. Undo.Revert
// puts all of them back - or, if anything touched has changed since the
// patch was applied, refuses and puts back nothing. A write that fails
// halfway through the commit is rolled back from the same snapshot at once.
//
// Not ported from the Python reference, which has no undo.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrUndoReverted is what a second Revert of the same Undo returns.
var ErrUndoReverted = errors.New("this patch has already been undone")

// UndoConflictError is Revert's refusal: Paths (relative to the project
// root, "/"-separated, sorted) changed after the patch was applied - edited,
// created, deleted or replaced by something other than this Undo - so
// restoring them would destroy that later work. Nothing was restored.
type UndoConflictError struct {
	Paths []string
}

func (e *UndoConflictError) Error() string {
	return "changed after the patch was applied: " + strings.Join(e.Paths, ", ")
}

// Undo is the journal of one real Apply that wrote something: for every
// path the patch touched, what was there before (file bytes and permission
// bits, a whole directory tree, a symlink, or nothing at all) and a hash of
// what the patch left there. It lives in memory; SaveUndo (undo_disk.go) also
// writes it to .vtvibe/undo/ so it survives a restart.
//
// A touched path is recorded at the highest level the patch changed: a
// write into a directory the patch had to create is recorded as that
// directory (absent before), so undoing it removes the directory too; a
// DELETE of a directory records the whole tree it removed; a RENAME records
// its source and its destination. Recorded paths never nest.
type Undo struct {
	projectDir string
	entries    []undoEntry
	reverted   bool
	dir        string // on-disk snapshot (undo_disk.go), "" when none
}

type undoEntry struct {
	rel    string // relative to projectDir, "/"-separated, for messages
	path   string
	before *fsNode
	after  [sha256.Size]byte
}

type fsNodeKind byte

const (
	nodeAbsent fsNodeKind = iota
	nodeFile
	nodeDir
	nodeSymlink
)

// fsNode is one path's state as captured from disk: kind, permission bits,
// content (file bytes or symlink target) and, for a directory, its entries.
type fsNode struct {
	kind     fsNodeKind
	perm     fs.FileMode
	data     []byte
	children map[string]*fsNode
}

// ProjectDir is the tree the patch was applied to.
func (u *Undo) ProjectDir() string { return u.projectDir }

// Paths lists the recorded paths, relative to ProjectDir and "/"-separated,
// in the order they were first touched - what an undo will put back.
func (u *Undo) Paths() []string {
	out := make([]string, len(u.entries))
	for i, en := range u.entries {
		out[i] = en.rel
	}
	return out
}

// Reverted reports whether Revert has already run to the end.
func (u *Undo) Reverted() bool { return u.reverted }

// Changed lists the recorded paths whose state on disk is no longer what
// the patch left there, sorted. An error means a path could not be read at
// all (permissions, I/O), which Revert treats as a refusal too.
func (u *Undo) Changed() ([]string, error) {
	var changed []string
	for _, en := range u.entries {
		n, err := captureNode(en.path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", en.rel, err)
		}
		if n.hash() != en.after {
			changed = append(changed, en.rel)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// Revert restores every recorded path to its state before the patch. It
// first checks all of them against the state the patch left: if any
// changed since, it returns *UndoConflictError and touches nothing, so
// there is never a half-undone patch because of someone else's edit. Once
// the check passes it restores every path, carrying on past a failing one,
// and returns the joined I/O errors, if any; the Undo counts as spent
// either way (a second call returns ErrUndoReverted), since what is on disk
// after a partial restore no longer matches the recorded "after" state.
func (u *Undo) Revert() error {
	if u.reverted {
		return ErrUndoReverted
	}
	changed, err := u.Changed()
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return &UndoConflictError{Paths: changed}
	}
	u.reverted = true
	err = u.restore()
	if u.dir != "" {
		// Spent, whatever came of the restore: it cannot be tried again.
		_ = os.RemoveAll(u.dir)
		u.dir = ""
	}
	return err
}

// restore writes every recorded "before" state back without any check - the
// second half of Revert, and the whole of the rollback after a failed
// commit.
func (u *Undo) restore() error {
	var errs []error
	for i := len(u.entries) - 1; i >= 0; i-- {
		en := u.entries[i]
		if err := restoreNode(en.path, en.before); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", en.rel, err))
		}
	}
	return errors.Join(errs...)
}

// seal records the "after" hash of every entry, right after a successful
// commit.
func (u *Undo) seal() error {
	for i := range u.entries {
		n, err := captureNode(u.entries[i].path)
		if err != nil {
			return fmt.Errorf("%s: %w", u.entries[i].rel, err)
		}
		u.entries[i].after = n.hash()
	}
	return nil
}

// beginUndo snapshots every path the write plan touches, before any of it
// is written.
func beginUndo(projectDir string, plan []writeOp) (*Undo, error) {
	var roots []string
	for _, op := range plan {
		roots = append(roots, undoRoot(projectDir, op.path))
		if op.kind == opRename {
			roots = append(roots, undoRoot(projectDir, op.newPath))
		}
	}
	u := &Undo{projectDir: projectDir}
	for _, p := range disjointRoots(roots) {
		n, err := captureNode(p)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(projectDir, p)
		if err != nil {
			rel = p
		}
		u.entries = append(u.entries, undoEntry{rel: filepath.ToSlash(rel), path: p, before: n})
	}
	return u, nil
}

// undoRoot is the highest path the patch creates on the way to p: p itself
// if its parent exists, otherwise the topmost missing ancestor below
// projectDir (commit creates missing parents with MkdirAll, and an undo has
// to remove those too).
func undoRoot(projectDir, p string) string {
	base := filepath.Clean(projectDir)
	p = filepath.Clean(p)
	for {
		parent := filepath.Dir(p)
		if parent == p || parent == base {
			return p
		}
		if rel, err := filepath.Rel(base, parent); err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return p
		}
		if _, err := os.Lstat(parent); err == nil {
			return p
		}
		p = parent
	}
}

// disjointRoots drops duplicates and every path lying inside another one
// (the outer snapshot already covers it), keeping first-seen order.
func disjointRoots(paths []string) []string {
	var out []string
	for i, p := range paths {
		covered := false
		for j, q := range paths {
			if i == j {
				continue
			}
			if (q == p && j < i) || strings.HasPrefix(p, q+string(filepath.Separator)) {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, p)
		}
	}
	return out
}

// captureNode reads path's current state without following symlinks. A
// path under a non-directory counts as absent, like a missing one.
func captureNode(path string) (*fsNode, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || underNonDir(path) {
			return &fsNode{kind: nodeAbsent}, nil
		}
		return nil, err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(path)
		if err != nil {
			return nil, err
		}
		return &fsNode{kind: nodeSymlink, data: []byte(target)}, nil
	case fi.IsDir():
		ents, err := os.ReadDir(path)
		if err != nil {
			return nil, err
		}
		n := &fsNode{kind: nodeDir, perm: fi.Mode().Perm(), children: make(map[string]*fsNode, len(ents))}
		for _, ent := range ents {
			c, err := captureNode(filepath.Join(path, ent.Name()))
			if err != nil {
				return nil, err
			}
			n.children[ent.Name()] = c
		}
		return n, nil
	case fi.Mode().IsRegular():
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return &fsNode{kind: nodeFile, perm: fi.Mode().Perm(), data: data}, nil
	}
	return nil, fmt.Errorf("%s: not a regular file, directory or symlink", path)
}

// underNonDir reports whether some ancestor of path is missing or is not a
// directory - the portable spelling of ENOTDIR, which not every GOOS has.
func underNonDir(path string) bool {
	parent := filepath.Dir(path)
	if parent == path {
		return false
	}
	fi, err := os.Lstat(parent)
	if err == nil {
		return !fi.IsDir()
	}
	return errors.Is(err, fs.ErrNotExist) || underNonDir(parent)
}

// hash is a digest of n's kind, content and tree shape. Permission bits are
// left out on purpose: they are restored, but a chmod after the patch is
// not a reason to refuse the undo, and Windows reports them only coarsely.
func (n *fsNode) hash() [sha256.Size]byte {
	h := sha256.New()
	n.writeTo(h)
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func (n *fsNode) writeTo(h hash.Hash) {
	var lenBuf [8]byte
	writeBytes := func(b []byte) {
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(b)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write(b)
	}
	_, _ = h.Write([]byte{byte(n.kind)})
	switch n.kind {
	case nodeFile, nodeSymlink:
		writeBytes(n.data)
	case nodeDir:
		names := make([]string, 0, len(n.children))
		for name := range n.children {
			names = append(names, name)
		}
		sort.Strings(names)
		binary.BigEndian.PutUint64(lenBuf[:], uint64(len(names)))
		_, _ = h.Write(lenBuf[:])
		for _, name := range names {
			writeBytes([]byte(name))
			n.children[name].writeTo(h)
		}
	}
}

// restoreNode replaces whatever is at path with n.
func restoreNode(path string, n *fsNode) error {
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if n.kind == nodeAbsent {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return createNode(path, n)
}

func createNode(path string, n *fsNode) error {
	switch n.kind {
	case nodeFile:
		if err := os.WriteFile(path, n.data, n.perm|0o200); err != nil {
			return err
		}
		return os.Chmod(path, n.perm)
	case nodeSymlink:
		return os.Symlink(string(n.data), path)
	case nodeDir:
		// Owner-writable while the entries go in, the recorded bits after.
		if err := os.Mkdir(path, n.perm|0o700); err != nil {
			return err
		}
		var errs []error
		for name, c := range n.children {
			if c.kind != nodeAbsent {
				if err := createNode(filepath.Join(path, name), c); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if err := os.Chmod(path, n.perm); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}
	return nil
}
