package fishplus

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// guardPath is the rule every mutation of helper.sh applies: the path is
// absolute, carries no ".." component and is not a root directory. The client
// always sends absolute paths, so the rule costs nothing in normal use; it is
// there because rmtree turns one mistake in path assembly into a lot of lost
// data. A name that merely begins with dots is not a ".." component.
func guardPath(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path is not absolute: %s", p)
	}
	for _, part := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if part == ".." {
			return "", fmt.Errorf("path has a .. component: %s", p)
		}
	}
	clean := filepath.Clean(p)
	if clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return "", fmt.Errorf("refusing the root directory: %s", p)
	}
	return clean, nil
}

func mutMkdir(p string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	// mkdir -p: the umask decides the final permissions.
	return os.MkdirAll(p, 0o777) //nolint:gosec // permissions are left to the umask, as mkdir -p does
}

func mutRemove(p string, dirOnly bool) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		return err
	}
	if dirOnly && !fi.IsDir() {
		return fmt.Errorf("rmdir %s: not a directory", p)
	}
	if !dirOnly && fi.IsDir() {
		return fmt.Errorf("rm %s: is a directory", p)
	}
	return os.Remove(p)
}

func mutRemoveAll(p string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(p); err != nil {
		return err
	}
	return os.RemoveAll(p)
}

func mutRename(from, to string) error {
	from, err := guardPath(from)
	if err != nil {
		return err
	}
	to, err = guardPath(to)
	if err != nil {
		return err
	}
	return os.Rename(from, to)
}

func mutCopy(from, to string) error {
	from, err := guardPath(from)
	if err != nil {
		return err
	}
	to, err = guardPath(to)
	if err != nil {
		return err
	}
	return copyTree(from, to)
}

func copyTree(from, to string) error {
	fi, err := os.Lstat(from)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	case fi.IsDir():
		if err := os.MkdirAll(to, fi.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(from)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	in, err := os.Open(from) //nolint:gosec // the client's path, guarded above
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm()) //nolint:gosec // the client's path
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// mutSymlink creates a symbolic link at link pointing at target. Only the link
// path is guarded: the target is a string to store, not a path on this host,
// and a relative one, one that does not exist yet or one with ".." are all
// ordinary. An existing link path is refused rather than replaced.
func mutSymlink(link, target string) error {
	link, err := guardPath(link)
	if err != nil {
		return err
	}
	return os.Symlink(target, link)
}

func mutChmod(p, octal string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	if octal == "" || strings.Trim(octal, "01234567") != "" {
		return fmt.Errorf("bad mode")
	}
	m, err := strconv.ParseUint(octal, 8, 32)
	if err != nil {
		return fmt.Errorf("bad mode")
	}
	mode := fs.FileMode(m & 0o777)
	if m&0o4000 != 0 {
		mode |= fs.ModeSetuid
	}
	if m&0o2000 != 0 {
		mode |= fs.ModeSetgid
	}
	if m&0o1000 != 0 {
		mode |= fs.ModeSticky
	}
	return os.Chmod(p, mode)
}

// parseOwner reads one half of chown's pair: a number, or "-" for "leave it alone".
func parseOwner(s string) (int, error) {
	if s == "-" {
		return -1, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("bad id %q", s)
	}
	return n, nil
}

func mutChown(p, uidArg, gidArg string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	uid, err := parseOwner(uidArg)
	if err != nil {
		return err
	}
	gid, err := parseOwner(gidArg)
	if err != nil {
		return err
	}
	return os.Chown(p, uid, gid)
}

// mutUtime sets the modification and access times from epoch seconds; "-" leaves
// a time alone.
func mutUtime(p, mtimeArg, atimeArg string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	parse := func(s string) (time.Time, error) {
		if s == "-" {
			return time.Time{}, nil // Chtimes: a zero time leaves it unchanged
		}
		sec, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad time %q", s)
		}
		return time.Unix(sec, 0), nil
	}
	m, err := parse(mtimeArg)
	if err != nil {
		return err
	}
	a, err := parse(atimeArg)
	if err != nil {
		return err
	}
	return os.Chtimes(p, a, m)
}

// mutTruncate sets a file's size, creating an empty one where there was none.
func mutTruncate(p, sizeArg string) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	size, err := strconv.ParseInt(sizeArg, 10, 64)
	if err != nil || size < 0 {
		return fmt.Errorf("bad size %q", sizeArg)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o600) //nolint:gosec // the client's path, guarded above
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// writeAt puts data at an offset of a file, creating it when it is not there
// and leaving whatever follows the range alone; a gap becomes a hole.
func writeAt(p string, off int64, data []byte) error {
	p, err := guardPath(p)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE, 0o600) //nolint:gosec // the client's path, guarded above
	if err != nil {
		return err
	}
	if _, err := f.WriteAt(data, off); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
