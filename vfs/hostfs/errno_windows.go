//go:build windows

package hostfs

import (
	"errors"
	"io/fs"

	winescape "github.com/unxed/libwinescape/go"
)

// hostfs promises os.* semantics on both sides of the personality switch --
// that is the whole point of the package -- and an error's semantics are
// part of that. In windows mode the errors come from os.* and answer
// errors.Is(err, fs.ErrNotExist) the way every caller in the tree expects.
// In posix mode they are raw libwinescape errnos, and whether they answer
// the same question depends on the version of that library linked in.
//
// The difference is not academic. A copy asks whether its destination
// already exists and reads the answer as
//
//	if err != nil && !errors.Is(err, fs.ErrNotExist) { return err }
//
// so an errno that will not admit to meaning "not there" turns every
// ordinary copy of a new file into a failure. libwinescape v0.1.5 answers
// the sentinels itself; posixError keeps f4 correct against older ones too,
// and costs one allocation on a path that is already returning an error.
type posixError struct{ errno winescape.Errno }

func (e posixError) Error() string { return e.errno.Error() }

// Unwrap keeps every existing comparison working: errors.Is against a
// syscall.Errno or a winescape.Errno still reaches the errno underneath.
func (e posixError) Unwrap() error { return e.errno }

func (e posixError) Is(target error) bool {
	switch target {
	case fs.ErrNotExist:
		return e.errno == winescape.ENOENT || e.errno == winescape.ENOTDIR
	case fs.ErrExist:
		return e.errno == winescape.EEXIST
	case fs.ErrPermission:
		return e.errno == winescape.EACCES || e.errno == winescape.EPERM
	case fs.ErrInvalid:
		return e.errno == winescape.EINVAL
	}
	return false
}

// hostErr normalizes an error coming out of libwinescape. Anything that is
// not a bare errno -- a wrapped error, an fs.PathError, nil -- is returned
// untouched, so this is safe to wrap around any posix-mode return.
func hostErr(err error) error {
	if err == nil {
		return nil
	}
	var errno winescape.Errno
	if errors.As(err, &errno) {
		return posixError{errno: errno}
	}
	return err
}
