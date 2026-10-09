package archive

import (
	"path"
	"strings"

	"github.com/unxed/f4/vfs"
)

// archiveHandlerName is how the panel title names what opened the archive.
const archiveHandlerName = "Zipper"

// PanelTitle names an archive by what reads it and the format it is in,
// followed by the folder inside it: "Zipper:7z:Far.7z" at the top of the
// archive, "Zipper:7z:Far.7z\Plugins" further in (#1383). The folder the
// archive file lies in is where ".." at the top returns to.
//
// A path that is not inside this archive is left to the panel's own title.
func (v *ArchiveVFS) PanelTitle(p string) string {
	if v == nil || v.displayName == "" {
		return ""
	}
	inner, ok := archiveRelativePath(p, v.arcPath)
	if !ok {
		return ""
	}
	title := archivePathJoin(v.displayName, inner)
	if format := archiveFormatLabel(v.displayName, v.format, v.sfxSuffix); format != "" {
		return archiveHandlerName + ":" + format + ":" + title
	}
	return archiveHandlerName + ":" + title
}

// archiveFormatLabel is the format the title shows. It comes from what the
// plugin itself went by when it opened the archive:
//
//   - for an archive found behind an executable stub, the kind of archive the
//     probe found there;
//   - for a name that ends in an archive extension the plugin opens archives
//     by, that extension -- tar.gz for a compressed tar, gz for a lone gzip;
//   - for an archive recognised by its content alone -- a .jar, a split .z01
//     volume -- the reader that reads it, zip or tar.
//
// When none of these says, the format is left out of the title rather than
// made up.
func archiveFormatLabel(name, format, sfxSuffix string) string {
	if suffix := strings.TrimPrefix(strings.ToLower(sfxSuffix), "."); suffix != "" {
		return suffix
	}
	ext := strings.ToLower(path.Ext(strings.ReplaceAll(name, "\\", "/")))
	switch ext {
	case ".gz", ".bz2", ".xz", ".zst":
		if format == "tar" {
			return "tar" + ext
		}
		return ext[1:]
	case ".zip", ".7z", ".rar", ".tar", ".tgz", ".txz", ".tbz2", ".tzst":
		return ext[1:]
	}
	if format == "zip" || format == "tar" {
		return format
	}
	return ""
}

var _ vfs.PanelTitleProvider = (*ArchiveVFS)(nil)
