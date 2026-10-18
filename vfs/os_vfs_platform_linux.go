//go:build linux

package vfs

import (
	"encoding/binary"
	"time"

	"golang.org/x/sys/unix"
)

// ntfs3gCreationTimeXattr is the extended attribute through which the ntfs-3g
// driver reads and writes a file's creation time. The kernel's ntfs3 driver and
// every other Linux filesystem have no call that sets a birth time (utimensat
// reaches only atime and mtime), so this is the one route there is (f4#2035).
const ntfs3gCreationTimeXattr = "system.ntfs_crtime"

// filetimeEpochDelta is the number of 100 ns ticks from 1601-01-01, the NTFS
// epoch, to 1970-01-01.
const filetimeEpochDelta = 116444736000000000

// applyPlatformAttributes writes the creation time only when the caller asked
// for it (item.SetBTime). It is best effort and never fails: a filesystem that
// does not know the attribute answers ENOTSUP, which is the normal case, and
// an error here must not read as a permission problem that sends the whole
// attribute change through sudo.
func applyPlatformAttributes(path string, item VFSItem) error {
	if item.SetBTime && !item.BTime.IsZero() {
		_ = unix.Lsetxattr(path, ntfs3gCreationTimeXattr, ntfsFiletime(item.BTime), 0)
	}
	return nil
}

// ntfsFiletime encodes t as the 8-byte little-endian FILETIME that the ntfs-3g
// attribute carries: 100 ns ticks since 1601-01-01 UTC.
func ntfsFiletime(t time.Time) []byte {
	ticks := t.Unix()*10_000_000 + int64(t.Nanosecond())/100 + filetimeEpochDelta
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(ticks)) //nolint:gosec // dates before 1601 are not representable and were never valid
	return buf
}

// SupportsSetBTime is false here: only the ntfs-3g driver takes a creation
// time, and nothing tells ahead of time whether this is such a volume, so the
// attributes dialog keeps its Created row read-only. Copy sets it best effort.
func (v *OSVFS) SupportsSetBTime() bool { return false }
