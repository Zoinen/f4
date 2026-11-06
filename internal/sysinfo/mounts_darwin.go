//go:build darwin

package sysinfo

import (
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// MountEntry describes a mounted user volume, not a raw physical disk.
type MountEntry struct {
	Device     string
	MountPoint string
	FSType     string
}

// The VFS unmount implementation is currently Linux-only.
func (m MountEntry) unmountDevice() string { return "" }

// UserMounts reads the live kernel table on each drive-menu opening. NOWAIT
// avoids waiting for filesystem statistics from slow or unreachable volumes.
func UserMounts() []MountEntry {
	count, err := syscall.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		darwinInfoDebug("[FIX:drives] counting mounts: %v", err)
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		stats := make([]syscall.Statfs_t, count+16)
		count, err = syscall.Getfsstat(stats, unix.MNT_NOWAIT)
		if err != nil {
			darwinInfoDebug("[FIX:drives] reading mounts: %v", err)
			return nil
		}
		if count >= len(stats) {
			continue // A volume arrived between sizing and reading the table.
		}
		mounts := darwinUserMounts(stats[:count])
		darwinInfoDebug("[FIX:drives] %d user volumes in %d mounts", len(mounts), count)
		return mounts
	}
	darwinInfoDebug("[FIX:drives] mount table kept growing; retry on next menu opening")
	return nil
}

func darwinUserMounts(stats []syscall.Statfs_t) []MountEntry {
	mounts := make([]MountEntry, 0)
	seen := make(map[string]bool)
	for _, stat := range stats {
		path := int8SliceToString(stat.Mntonname[:])
		if !strings.HasPrefix(path, "/Volumes/") || path == "/Volumes/" {
			continue
		}
		if stat.Flags&unix.MNT_DONTBROWSE != 0 || seen[path] {
			continue
		}
		seen[path] = true
		mounts = append(mounts, MountEntry{
			Device:     int8SliceToString(stat.Mntfromname[:]),
			MountPoint: path,
			FSType:     int8SliceToString(stat.Fstypename[:]),
		})
	}
	return mounts
}
