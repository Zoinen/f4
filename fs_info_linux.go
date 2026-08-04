//go:build linux

package main

import (
	"bufio"
	"os"
	"strings"
	"syscall"
)

// fsInfo populates as many fields as the current OS can supply for
// the filesystem holding path. ok=false if the value can't be
// determined (path missing, non-local backend, etc.).
func fsInfo(path string) (FSInfo, bool) {
	if path == "" {
		return FSInfo{}, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSInfo{}, false
	}
	bs := uint64(st.Bsize)
	info := FSInfo{
		Total:       uint64(st.Blocks) * bs,
		Free:        uint64(st.Bavail) * bs,
		MaxFilename: int(st.Namelen),
		ClusterSize: bs,
	}
	// Enrich with mount point / fs type / flags from /proc/mounts when
	// available (Linux). Non-fatal on other unices — /proc simply
	// doesn't exist and the extra fields stay empty.
	enrichFromProcMounts(path, &info)
	return info, true
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
