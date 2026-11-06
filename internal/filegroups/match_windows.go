package filegroups

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
)

func platformIgnoreCase() bool { return !hostmode.Posix() }
func platformAttributes(item *vfs.VFSItem) uint32 {
	var attrs uint32
	if item.WinAttrs&1 != 0 {
		attrs |= attrReadOnly
	}
	if item.WinAttrs&4 != 0 {
		attrs |= attrSystem
	}
	if item.WinAttrs&32 != 0 {
		attrs |= attrArchive
	}
	return attrs
}
