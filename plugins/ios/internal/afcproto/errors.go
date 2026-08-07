package afcproto

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"syscall"
)

var (
	ErrProtocol       = errors.New("afc protocol error")
	ErrConnectionLost = errors.New("afc connection lost")
	ErrClosed         = errors.New("afc client closed")
	ErrUnsafePath     = errors.New("unsafe AFC path")
	ErrIsDirectory    = errors.New("AFC object is a directory")
)

// StatusError reports a non-success status returned by the AFC service.
type StatusError struct {
	Code uint64
	Name string
	err  error
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("afc status %d (%s)", e.Code, e.Name)
}

func (e *StatusError) Unwrap() error { return e.err }

// IsConnectionLost reports errors for which the owning service connection must
// be discarded rather than returned to a pool.
func IsConnectionLost(err error) bool {
	return errors.Is(err, ErrConnectionLost)
}

type connectionError struct{ cause error }

func (e *connectionError) Error() string { return "afc connection lost: " + e.cause.Error() }
func (e *connectionError) Unwrap() []error {
	return []error{ErrConnectionLost, e.cause}
}

func statusError(code uint64) error {
	name := "unknown"
	var mapped error
	switch code {
	case 1:
		name = "unknown error"
	case 2:
		name, mapped = "invalid operation header", ErrProtocol
	case 3:
		name, mapped = "no resources", syscall.ENOMEM
	case 4:
		name, mapped = "read error", syscall.EIO
	case 5:
		name, mapped = "write error", syscall.EIO
	case 6:
		name, mapped = "unknown packet type", errors.ErrUnsupported
	case 7:
		name, mapped = "invalid argument", fs.ErrInvalid
	case 8:
		name, mapped = "object not found", fs.ErrNotExist
	case 9:
		name, mapped = "object is a directory", ErrIsDirectory
	case 10:
		name, mapped = "permission denied", fs.ErrPermission
	case 11:
		name, mapped = "service not connected", ErrConnectionLost
	case 12:
		name, mapped = "operation timed out", context.DeadlineExceeded
	case 13:
		name, mapped = "too much data", syscall.EFBIG
	case 14:
		name, mapped = "end of data", io.EOF
	case 15:
		name, mapped = "operation not supported", errors.ErrUnsupported
	case 16:
		name, mapped = "object exists", fs.ErrExist
	case 17:
		name, mapped = "object busy", syscall.EBUSY
	case 18:
		name, mapped = "no space left", syscall.ENOSPC
	case 19:
		name, mapped = "operation would block", syscall.EWOULDBLOCK
	case 20:
		name, mapped = "I/O error", syscall.EIO
	case 21:
		name, mapped = "operation interrupted", syscall.EINTR
	case 22:
		name, mapped = "operation in progress", syscall.EINPROGRESS
	case 23:
		name = "internal error"
	case 30:
		name, mapped = "mux error", ErrConnectionLost
	case 31:
		name, mapped = "no memory", syscall.ENOMEM
	case 32:
		name, mapped = "not enough data", io.ErrUnexpectedEOF
	case 33:
		name, mapped = "directory not empty", syscall.ENOTEMPTY
	case 34:
		name, mapped = "TLS error", ErrConnectionLost
	}
	return &StatusError{Code: code, Name: name, err: mapped}
}
