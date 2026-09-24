package panel

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
)

// entrySizeText is what an entry shows where a size goes: the Size column,
// the status line, the bottom frame and the semantic panel model. It follows
// far2l's FormatStr_Size with the default DirNameStyle and ShowSymlinkSize=0,
// and Far3's, which also tells a junction from a symlink: a counted folder
// shows its size, ".." shows Up, a link shows its kind instead of the size
// of the link itself, any other directory shows Folder, and a file shows
// its size.
func entrySizeText(e *FileEntry) string {
	if e.IsDir && e.SizeCalculated {
		return fileops.FormatIntWithSpaces(e.Size)
	}
	if e.Name == ".." {
		return i18n.Msg("Panel.UpDir")
	}
	switch vfs.LinkKindOf(&e.VFSItem) {
	case vfs.LinkJunction:
		return i18n.Msg("Panel.Junction")
	case vfs.LinkSymlink:
		return i18n.Msg("Panel.Symlink")
	}
	if e.IsDir {
		return i18n.Msg("Panel.Folder")
	}
	return fileops.FormatIntWithSpaces(e.Size)
}
