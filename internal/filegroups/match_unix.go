//go:build !windows

package filegroups

import "github.com/unxed/f4/vfs"

func platformIgnoreCase() bool { return false }
func platformAttributes(item *vfs.VFSItem) uint32 {
	if item.UnixMode&0222 == 0 {
		return attrReadOnly
	}
	return 0
}
