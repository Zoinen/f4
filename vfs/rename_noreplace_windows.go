//go:build windows

package vfs

import (
	"errors"

	"golang.org/x/sys/windows"
)

func renameNoReplace(oldPath, newPath string) error {
	if sameObject(oldPath, newPath) {
		return renameSameObject(oldPath, newPath)
	}
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	if err := windows.MoveFile(from, to); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return ErrDestinationExists
		}
		return err
	}
	return nil
}
