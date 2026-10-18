package fileops

import "github.com/unxed/f4/vfs"

// withSourceBirthTime asks the destination to take over the source's creation
// time, so a copy keeps "Created" the way it keeps the modification time
// (f4#2035). It acts only where the stat did not already carry a birth time —
// Windows and macOS fill one in — and only between two local filesystems, where
// ReadBirthTime can look the source up (Linux; elsewhere it finds nothing, so
// nothing changes). Whether the destination can take it is its own business:
// most Linux filesystems cannot, and the write is best effort there.
func withSourceBirthTime(item *vfs.VFSItem, srcVfs vfs.VFS, srcPath string, dstVfs vfs.VFS) {
	if item.HasMetadata(vfs.MetadataBTime) || !IsLocalOSVFS(srcVfs) || !IsLocalOSVFS(dstVfs) {
		return
	}
	if bt, ok := vfs.ReadBirthTime(srcPath); ok {
		item.BTime, item.SetBTime = bt, true
	}
}
