package vfs

// Reparse point tags from the Windows documentation ("Reparse Point Tags").
// They are spelled out here instead of taken from golang.org/x/sys/windows so
// that code on every platform can classify an item a Windows VFS produced.
const (
	// ReparseTagMountPoint is IO_REPARSE_TAG_MOUNT_POINT: a directory
	// junction or a volume mount point.
	ReparseTagMountPoint uint32 = 0xA0000003
	// ReparseTagSymlink is IO_REPARSE_TAG_SYMLINK.
	ReparseTagSymlink uint32 = 0xA000000C
	// reparseTagNameSurrogate is the tag's N bit: the reparse point stands
	// for another named entity in the system, i.e. it is a link of some
	// kind. Cloud placeholders, deduplicated files and AF_UNIX sockets are
	// reparse points without this bit; they are the object itself.
	reparseTagNameSurrogate uint32 = 0x20000000
)

// LinkKind says which kind of link an item is, the distinction Far Manager's
// panel draws between "Symlink" and "Junction".
type LinkKind uint8

const (
	// LinkNone is not a link.
	LinkNone LinkKind = iota
	// LinkSymlink is a symbolic link, or a link whose exact kind the VFS
	// does not report.
	LinkSymlink
	// LinkJunction is a Windows directory junction or volume mount point.
	LinkJunction
)

// LinkKindOf classifies item. A mount point tag is a junction. Any other
// IsSymlink item is a symlink when its tag is unknown (every non-Windows
// VFS) or names a surrogate; a reparse point that is not a surrogate is the
// object itself and not a link at all.
func LinkKindOf(item *VFSItem) LinkKind {
	if item.ReparseTag == ReparseTagMountPoint {
		return LinkJunction
	}
	if !item.IsSymlink {
		return LinkNone
	}
	if item.ReparseTag == 0 || item.ReparseTag&reparseTagNameSurrogate != 0 {
		return LinkSymlink
	}
	return LinkNone
}
