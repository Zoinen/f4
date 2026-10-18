//go:build !windows

package dialog

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// errCompressionUnsupported is a variable, not an inline fmt.Errorf, so that
// callers' `err != nil` is not statically always true (staticcheck SA4023).
var errCompressionUnsupported = errors.New("NTFS compression is available only on Windows")

func setWindowsFileCompression(v vfs.VFS, path string, compressed bool) error {
	return errCompressionUnsupported
}
