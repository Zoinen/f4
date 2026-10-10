//go:build darwin

package vfs

import (
	"encoding/binary"
	"time"

	"golang.org/x/sys/unix"
)

func applyPlatformAttributes(path string, item VFSItem) error {
	if item.SetBTime && !item.BTime.IsZero() {
		return setCreationTime(path, item.BTime)
	}
	return nil
}

// setCreationTime writes the birth time of path with setattrlist(2)
// (ATTR_CMN_CRTIME), which utimensat cannot reach. The attribute buffer is one
// struct timespec.
func setCreationTime(path string, t time.Time) error {
	// struct timespec is two little-endian int64s (tv_sec, tv_nsec) on both
	// darwin/amd64 and darwin/arm64.
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:], uint64(t.Unix()))       //nolint:gosec // seconds since the epoch, sign preserved
	binary.LittleEndian.PutUint64(buf[8:], uint64(t.Nanosecond())) //nolint:gosec // 0..999999999
	al := unix.Attrlist{Bitmapcount: unix.ATTR_BIT_MAP_COUNT, Commonattr: unix.ATTR_CMN_CRTIME}
	return unix.Setattrlist(path, &al, buf, 0)
}

// SupportsSetBTime is true on macOS: the birth time is settable.
func (v *OSVFS) SupportsSetBTime() bool { return true }
