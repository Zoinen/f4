package vfs

import (
	"errors"
	"os"

	"github.com/unxed/f4/vfs/hostfs"
)

// NotListableError is what OSVFS.SetPath returns for a directory that exists
// but whose contents the host refuses to list, when no elevation route can
// list it instead (#814).
//
// Stat succeeding is not enough to accept such a path. On Windows os.Stat
// answers from GetFileAttributesEx, which needs only the right to read
// attributes, while ReadDir opens the directory with GENERIC_READ, which
// needs the right to list it. Accepting the path let the panel draw the new
// title over an empty listing and take it back once ReadDir failed, so the
// user saw a folder being entered and left. Far refuses the change up front
// instead: its FarChDir fails before the panel is updated.
//
// The error unwraps to the host error, so errors.Is(err, os.ErrPermission)
// still holds and the message the user reads is the one ReadDir produced.
type NotListableError struct {
	Path string
	Err  error
}

func (e *NotListableError) Error() string { return e.Err.Error() }

func (e *NotListableError) Unwrap() error { return e.Err }

// checkOSDirListable performs the step ReadDir starts with -- opening the
// directory for reading -- without reading any entries, and retries the same
// reparse candidates ReadDir retries. hostfs.ReadDir opens a directory with the
// same access hostfs.Open asks for: on Windows both come down to CreateFile
// with GENERIC_READ and FILE_FLAG_BACKUP_SEMANTICS (os.ReadDir's O_DIRECTORY
// is not passed to CreateFile), elsewhere to open(2) with O_RDONLY.
func checkOSDirListable(dir string) error {
	err := openOSDirOnce(dir)
	// Same condition ReadDir uses before it tries the candidates.
	if err != nil && os.IsPermission(err) && WindowsPersonality() {
		for _, candidate := range resolveReparseCandidates(dir) {
			if openOSDirOnce(candidate) == nil {
				return nil
			}
		}
	}
	return err
}

func openOSDirOnce(dir string) error {
	f, err := hostfs.Open(prepareOSPath(dir))
	if err != nil {
		return err
	}
	_ = f.Close() // Only whether the open succeeds is in question; nothing was read.
	return nil
}

// refuseNotListable returns a NotListableError for dir when the host will
// not list it and no elevation route could, and nil otherwise. Errors other
// than a permission refusal are left to ReadDir, which already reports them.
func refuseNotListable(dir string) error {
	err := checkOSDirListable(dir)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return nil
	}
	// With elevation available ReadDir lists the directory through it, so
	// the path is still accepted as before.
	if globalSudoClient.IsAvailable() {
		return nil
	}
	return &NotListableError{Path: dir, Err: err}
}
