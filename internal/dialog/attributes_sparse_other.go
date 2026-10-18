//go:build !windows

package dialog

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// errSparseUnsupported is a variable so callers' err != nil checks are not
// statically known to be always true (staticcheck SA4023).
var errSparseUnsupported = errors.New("NTFS sparse files are unavailable on this platform")

func setWindowsFileSparse(v vfs.VFS, path string, sparse bool) error {
	return errSparseUnsupported
}
