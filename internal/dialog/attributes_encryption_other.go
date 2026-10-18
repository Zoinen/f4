//go:build !windows

package dialog

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

var errEncryptionUnsupported = errors.New("NTFS encryption is available only on Windows")

func setWindowsFileEncryption(v vfs.VFS, path string, encrypted bool) error {
	return errEncryptionUnsupported
}
