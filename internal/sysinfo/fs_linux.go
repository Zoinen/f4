//go:build linux

package sysinfo

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

// FsInfo populates as many fields as the current OS can supply for
// the filesystem holding path. ok=false if the value can't be
// determined (path missing, non-local backend, etc.).
func FS(path string) (FSInfo, bool) {
	info, ok := statfsInfo(path)
	if !ok {
		return FSInfo{}, false
	}
	// Enrich with mount point / fs type / flags from /proc/mounts when
	// available (Linux). Non-fatal on other unices — /proc simply
	// doesn't exist and the extra fields stay empty.
	enrichFromProcMounts(path, &info)
	return info, true
}

// FreeBytes reports the space available to an unprivileged user on the
// filesystem holding path. It does a single statfs and, unlike FS, does not
// read and parse /proc/mounts, so it is cheap enough for code that runs on
// every redraw (the panel footer): on a slow CPU the mount table lookup made
// each frame cost milliseconds (unxed/f4#1758).
func FreeBytes(path string) (uint64, bool) {
	info, ok := statfsInfo(path)
	return info.Free, ok
}

// statfsInfo fills the fields that one statfs call supplies.
func statfsInfo(path string) (FSInfo, bool) {
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
		MaxFilename: int(st.Namelen),
		ClusterSize: bs,
	}, true
}

func enrichFromProcMounts(path string, info *FSInfo) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return
	}
	defer f.Close()
	// Pick the entry whose mount-point is the longest prefix of path,
	// so "/home/user" resolves to "/home" if /home is a separate mount
	// and just "/" otherwise.
	bestLen := -1
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 4 {
			continue
		}
		mp := fields[1]
		if !mountCovers(mp, path) {
			continue
		}
		if len(mp) > bestLen {
			bestLen = len(mp)
			info.Mount = mp
			info.Type = fields[2]
			info.Flags = fields[3]
		}
	}
}

// mountCovers reports whether mount point mp is an ancestor of path.
func mountCovers(mp, path string) bool {
	if mp == path {
		return true
	}
	if mp == "/" {
		return true
	}
	return strings.HasPrefix(path, mp+"/")
}
