//go:build windows

package vfs

import (
	"errors"
	"syscall"

	winescape "github.com/unxed/libwinescape/go"
	"golang.org/x/sys/windows"

	"github.com/unxed/f4/vfs/hostmode"
)

func renameNoReplace(oldPath, newPath string) error {
	if sameObject(oldPath, newPath) {
		return renameSameObject(oldPath, newPath)
	}
	if hostmode.Posix() {
		// winescape.RenameNoReplace (renameat2/RENAME_NOREPLACE) is the
		// same atomic, race-free primitive vfs/rename_noreplace_linux.go
		// already uses on a real POSIX build -- posix mode is supposed to
		// behave identically, not just similarly (WINE.md §13.10).
		err := winescape.RenameNoReplace(oldPath, newPath)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EEXIST) {
			return ErrDestinationExists
		}
		if errors.Is(err, syscall.ENOSYS) {
			// libwinescape's own fallback for a host where renameat2 isn't
			// available (see its Renameat2 doc comment) -- same portable
			// fallback the Linux build uses in the equivalent case.
			return renameNoReplacePortable(oldPath, newPath)
		}
		return err
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
