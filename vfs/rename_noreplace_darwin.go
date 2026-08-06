//go:build darwin

package vfs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldPath, newPath string) error {
	if sameObject(oldPath, newPath) {
		return renameSameObject(oldPath, newPath)
	}
	err := unix.RenamexNp(oldPath, newPath, unix.RENAME_EXCL)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return ErrDestinationExists
	}
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EINVAL) {
		return renameNoReplacePortable(oldPath, newPath)
	}
	return err
}
