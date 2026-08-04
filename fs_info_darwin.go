//go:build darwin

package main

import "syscall"

// fsInfo populates FSInfo for the filesystem holding path on macOS via
// syscall.Statfs. Darwin's Statfs_t exposes fs type name (e.g. "apfs",
// "hfs") and mount point, so those fields land too. Namelen isn't in
// Darwin's Statfs_t, so MaxFilename stays 0. `ok=false` if path is
// empty or the syscall fails.
func fsInfo(path string) (FSInfo, bool) {
	if path == "" {
		return FSInfo{}, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSInfo{}, false
	}
	bs := uint64(st.Bsize)
	return FSInfo{
		Total:       uint64(st.Blocks) * bs,
		Free:        uint64(st.Bavail) * bs,
		Type:        int8SliceToString(st.Fstypename[:]),
		Mount:       int8SliceToString(st.Mntonname[:]),
		ClusterSize: bs,
	}, true
}

// int8SliceToString converts a NUL-terminated C-style char buffer
// (BSD APIs return them as [N]int8) to a Go string.
func int8SliceToString(s []int8) string {
	n := 0
	for n < len(s) && s[n] != 0 {
		n++
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[i] = byte(s[i])
	}
	return string(b)
}
