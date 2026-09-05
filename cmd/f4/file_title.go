package main

import (
	"path/filepath"

	"github.com/unxed/f4/vfs"
)

// displayFileTitle returns the file identity used by the Editor and Viewer
// title bars. VFS paths are kept opaque: a remote or virtual filesystem owns
// the separator and any scheme prefix in its path, so the full-path setting
// must not run the value through the host filepath package.
func displayFileTitle(filesystem vfs.VFS, filePath string) string {
	if filePath == "" {
		return ""
	}
	if AppConfig.DisplayFullPathInTitle {
		return filePath
	}
	if filesystem != nil {
		return filesystem.Base(filePath)
	}
	return filepath.Base(filePath)
}
