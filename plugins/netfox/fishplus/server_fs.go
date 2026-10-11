package fishplus

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The listing the server answers with is the "find" backend's format, the one
// a client reads for free (Features.ListingMode "find"): one line per entry,
//
//	<type> <target type> <size> <mtime> <atime> <ctime> <octal perm> <uid> <gid> <name>
//
// after a "M find" marker line. The times are all the modification time: the
// server does not read the other two (see statOwner).
const listingMarker = "M find"

func typeChar(m fs.FileMode) byte {
	switch {
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m.IsDir():
		return 'd'
	case m&fs.ModeNamedPipe != 0:
		return 'p'
	case m&fs.ModeSocket != 0:
		return 's'
	case m&fs.ModeCharDevice != 0:
		return 'c'
	case m&fs.ModeDevice != 0:
		return 'b'
	}
	return 'f'
}

// permBits is the octal permission field, setuid, setgid and sticky included.
func permBits(m fs.FileMode) uint32 {
	perm := uint32(m.Perm())
	if m&fs.ModeSetuid != 0 {
		perm |= 04000
	}
	if m&fs.ModeSetgid != 0 {
		perm |= 02000
	}
	if m&fs.ModeSticky != 0 {
		perm |= 01000
	}
	return perm
}

// listingLine renders one entry. path is where the entry lives, used to see
// whether a symlink points at a directory.
func listingLine(fi fs.FileInfo, name, path string) string {
	target := typeChar(fi.Mode())
	if fi.Mode()&fs.ModeSymlink != 0 {
		target = 'f'
		if ti, err := os.Stat(path); err == nil && ti.IsDir() {
			target = 'd'
		}
	}
	uid, gid := statOwner(fi)
	mtime := fmt.Sprintf("%d.%09d", fi.ModTime().Unix(), fi.ModTime().Nanosecond())
	return fmt.Sprintf("%c %c %d %s %s %s %o %d %d %s",
		typeChar(fi.Mode()), target, fi.Size(), mtime, mtime, mtime, permBits(fi.Mode()), uid, gid, name)
}

// cleanAbs refuses what a path line must not be: relative paths (the client
// always sends absolute ones, and the same rule guards the mutations of
// helper.sh).
func cleanAbs(p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path is not absolute: %s", p)
	}
	return filepath.Clean(p), nil
}

// infoLines answers info (follow symlinks) and linfo (the link itself).
func infoLines(p string, follow bool) ([]string, error) {
	p, err := cleanAbs(p)
	if err != nil {
		return nil, err
	}
	stat := os.Lstat
	if follow {
		stat = os.Stat
	}
	fi, err := stat(p)
	if err != nil {
		return nil, err
	}
	return []string{listingMarker, listingLine(fi, filepath.Base(p), p)}, nil
}

// enumLines answers enum: every entry of the directory, hidden ones included,
// "." and ".." left out.
func enumLines(dir string) ([]string, error) {
	dir, err := cleanAbs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	lines := []string{listingMarker}
	for _, e := range entries {
		full := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(full)
		if err != nil {
			continue // vanished between the read and the stat
		}
		lines = append(lines, listingLine(fi, e.Name(), full))
	}
	return lines, nil
}

// isdirsLines answers isdirs: 1 for a path that resolves to a directory, else 0.
func isdirsLines(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = "0"
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out[i] = "1"
		}
	}
	return out
}

// errText renders an error on one line. A missing path is always reported in
// the words the client's not-found detection reads, "no such file or
// directory", whatever the operating system calls it (Windows says "The system
// cannot find the file specified").
func errText(err error) string {
	var pe *fs.PathError
	if errors.Is(err, fs.ErrNotExist) && errors.As(err, &pe) {
		return fmt.Sprintf("%s %s: no such file or directory", pe.Op, pe.Path)
	}
	return strings.ReplaceAll(err.Error(), "\n", " ")
}

func atoiArg(args []string, i int) (int, bool) {
	if i >= len(args) {
		return 0, false
	}
	n, err := strconv.Atoi(args[i])
	return n, err == nil && n >= 0
}

// readRange answers read: the size the file has now and the bytes of the range
// that exist. A length of zero means "to the end", and one longer than
// MaxReadLen is cut to it, so that a bad request cannot make the server load a
// whole disk.
func readRange(p string, off, length int64) (size int64, data []byte, err error) {
	p, err = cleanAbs(p)
	if err != nil {
		return 0, nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return 0, nil, err
	}
	if fi.IsDir() {
		return 0, nil, fmt.Errorf("read %s: is a directory", p)
	}
	size = fi.Size()
	if off >= size {
		return size, nil, nil
	}
	n := size - off
	if length > 0 && length < n {
		n = length
	}
	if n > MaxReadLen {
		n = MaxReadLen
	}
	data = make([]byte, n)
	got, err := f.ReadAt(data, off)
	if err != nil && err != io.EOF {
		return size, nil, err
	}
	return size, data[:got], nil
}
