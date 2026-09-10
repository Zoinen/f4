package viewer

import (
	"path/filepath"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// DisplayFileTitle returns the file identity used by the Editor and Viewer
// title bars. VFS paths are kept opaque: a remote or virtual filesystem owns
// the separator and any scheme prefix in its path, so the full-path setting
// must not run the value through the host filepath package.
func DisplayFileTitle(filesystem vfs.VFS, FilePath string) string {
	if FilePath == "" {
		return ""
	}
	if config.App.DisplayFullPathInTitle {
		return FilePath
	}
	if filesystem != nil {
		return filesystem.Base(FilePath)
	}
	return filepath.Base(FilePath)
}
