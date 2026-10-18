//go:build !linux

package sysinfo

// FreeBytes reports the space available to an unprivileged user on the
// filesystem holding path. Where FS has no costly mount-table lookup (Linux
// is the only platform that reads /proc/mounts) it is simply FS's Free.
func FreeBytes(path string) (uint64, bool) {
	info, ok := FS(path)
	return info.Free, ok
}
