//go:build linux

package vfs

import (
	"errors"

	"golang.org/x/sys/unix"
)

func renameNoReplace(oldPath, newPath string) error {
	if sameObject(oldPath, newPath) {
		return renameSameObject(oldPath, newPath)
	}
	err := unix.Renameat2(unix.AT_FDCWD, oldPath, unix.AT_FDCWD, newPath, unix.RENAME_NOREPLACE)
	if err == nil {
		return nil
	}
	if errors.Is(err, unix.EEXIST) {
		return ErrDestinationExists
	}
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		return renameNoReplacePortable(oldPath, newPath)
	}
	return err
}
