package ap

// On-disk undo snapshots (docs/VTVIBE.md §7.4, f4#1606): a transaction's
// journal is written to <project>/.vtvibe/undo/<UTC timestamp>/ so that an
// applied patch can still be undone after f4 was restarted.
//
//	manifest.json  version, and for every recorded path its relative name,
//	               the hash of what the patch left there and the tree that
//	               was there before (kinds, permission bits, and the name of
//	               the blob holding a file's bytes or a symlink's target)
//	blobs/N        the bytes themselves
//
// Nothing here changes what Revert does; it only lets an Undo outlive the
// process. A transaction that has been reverted has its directory removed.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

// UndoDir is where a project keeps its snapshots, relative to the project.
var undoRelDir = filepath.Join(".vtvibe", "undo")

const undoManifestVersion = 1

type diskNode struct {
	Kind     fsNodeKind           `json:"kind"`
	Perm     uint32               `json:"perm,omitempty"`
	Blob     string               `json:"blob,omitempty"`
	Children map[string]*diskNode `json:"children,omitempty"`
}

type diskEntry struct {
	Rel    string    `json:"rel"`
	After  string    `json:"after"`
	Before *diskNode `json:"before"`
}

type diskManifest struct {
	Version int         `json:"version"`
	Entries []diskEntry `json:"entries"`
}

// SaveUndo writes u's journal into a new snapshot directory under its
// project's .vtvibe/undo/ and keeps only the newest keep of them (keep <= 0
// keeps all). The directory is remembered by u, so a later successful Revert
// removes it.
func SaveUndo(u *Undo, keep int) error {
	if u == nil {
		return nil
	}
	root := filepath.Join(u.projectDir, undoRelDir)
	if err := os.MkdirAll(root, 0o750); err != nil {
		return err
	}
	// The snapshots hold copies of project files; keep them out of version
	// control without touching the project's own ignore rules.
	ignore := filepath.Join(u.projectDir, ".vtvibe", ".gitignore")
	if _, err := os.Lstat(ignore); errors.Is(err, fs.ErrNotExist) {
		_ = os.WriteFile(ignore, []byte("*\n"), 0o600)
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	dir := filepath.Join(root, stamp)
	if err := os.Mkdir(dir, 0o750); err != nil {
		return err
	}
	if err := u.writeDisk(dir); err != nil {
		_ = os.RemoveAll(dir)
		return err
	}
	u.dir = dir
	if keep > 0 {
		pruneUndoDirs(root, keep)
	}
	return nil
}

func (u *Undo) writeDisk(dir string) error {
	blobs := filepath.Join(dir, "blobs")
	if err := os.Mkdir(blobs, 0o750); err != nil {
		return err
	}
	n := 0
	var conv func(*fsNode) (*diskNode, error)
	conv = func(node *fsNode) (*diskNode, error) {
		d := &diskNode{Kind: node.kind, Perm: uint32(node.perm)}
		switch node.kind {
		case nodeFile, nodeSymlink:
			n++
			d.Blob = strconv.Itoa(n)
			if err := os.WriteFile(filepath.Join(blobs, d.Blob), node.data, 0o600); err != nil {
				return nil, err
			}
		case nodeDir:
			d.Children = make(map[string]*diskNode, len(node.children))
			for name, c := range node.children {
				cd, err := conv(c)
				if err != nil {
					return nil, err
				}
				d.Children[name] = cd
			}
		}
		return d, nil
	}
	m := diskManifest{Version: undoManifestVersion}
	for _, en := range u.entries {
		before, err := conv(en.before)
		if err != nil {
			return err
		}
		m.Entries = append(m.Entries, diskEntry{Rel: en.rel, After: hex.EncodeToString(en.after[:]), Before: before})
	}
	data, err := json.MarshalIndent(&m, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o600)
}

// LoadUndo reads one snapshot directory back as an Undo for projectDir.
func LoadUndo(projectDir, dir string) (*Undo, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m diskManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Version != undoManifestVersion {
		return nil, fmt.Errorf("undo snapshot %s: unsupported version %d", dir, m.Version)
	}
	blobs := filepath.Join(dir, "blobs")
	var conv func(*diskNode) (*fsNode, error)
	conv = func(d *diskNode) (*fsNode, error) {
		if d == nil {
			return nil, errors.New("undo snapshot: missing node")
		}
		node := &fsNode{kind: d.Kind, perm: fs.FileMode(d.Perm)}
		switch d.Kind {
		case nodeAbsent:
		case nodeFile, nodeSymlink:
			if d.Blob == "" || d.Blob != filepath.Base(d.Blob) {
				return nil, fmt.Errorf("undo snapshot: bad blob name %q", d.Blob)
			}
			b, err := os.ReadFile(filepath.Join(blobs, d.Blob))
			if err != nil {
				return nil, err
			}
			node.data = b
		case nodeDir:
			node.children = make(map[string]*fsNode, len(d.Children))
			for name, c := range d.Children {
				if name == "" || name != filepath.Base(name) {
					return nil, fmt.Errorf("undo snapshot: bad entry name %q", name)
				}
				cn, err := conv(c)
				if err != nil {
					return nil, err
				}
				node.children[name] = cn
			}
		default:
			return nil, fmt.Errorf("undo snapshot: bad node kind %d", d.Kind)
		}
		return node, nil
	}
	u := &Undo{projectDir: projectDir, dir: dir}
	for _, de := range m.Entries {
		rel := filepath.FromSlash(de.Rel)
		if de.Rel == "" || filepath.IsAbs(rel) || rel == ".." || len(rel) > 2 && rel[:3] == ".."+string(filepath.Separator) {
			return nil, fmt.Errorf("undo snapshot: path %q leaves the project", de.Rel)
		}
		before, err := conv(de.Before)
		if err != nil {
			return nil, err
		}
		var after [sha256.Size]byte
		raw, err := hex.DecodeString(de.After)
		if err != nil || len(raw) != len(after) {
			return nil, errors.New("undo snapshot: bad hash")
		}
		copy(after[:], raw)
		u.entries = append(u.entries, undoEntry{rel: de.Rel, path: filepath.Join(projectDir, rel), before: before, after: after})
	}
	return u, nil
}

// LoadSavedUndos returns the transactions kept under projectDir/.vtvibe/undo,
// oldest first. A snapshot that cannot be read is skipped: a damaged journal
// must not stop the others from being offered.
func LoadSavedUndos(projectDir string) []*Undo {
	root := filepath.Join(projectDir, undoRelDir)
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(ents))
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	var out []*Undo
	for _, name := range names {
		if u, err := LoadUndo(projectDir, filepath.Join(root, name)); err == nil {
			out = append(out, u)
		}
	}
	return out
}

func pruneUndoDirs(root string, keep int) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for len(names) > keep {
		_ = os.RemoveAll(filepath.Join(root, names[0]))
		names = names[1:]
	}
}
