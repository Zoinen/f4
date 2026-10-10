//go:build windows

package vfs

import (
	"errors"
	"net"
	"os"
)

// ErrSudoNotSupported is what the Unix-only operations of the dispatcher
// (descriptor passing) answer on Windows.
var ErrSudoNotSupported = errors.New("sudo privilege elevation is not yet supported on Windows")

// sendMsg writes msg to the elevated peer. Windows has no descriptor passing,
// so a message that asks for one is refused instead of being sent without it.
func sendMsg(conn *net.UnixConn, msg any, fd int) error {
	if fd >= 0 {
		return ErrSudoNotSupported
	}
	return writeSudoFrame(conn, msg)
}

// recvMsg reads one message from the elevated peer; there is never a file.
func recvMsg(conn *net.UnixConn, msg any) (*os.File, error) {
	return nil, readSudoFrame(conn, msg)
}
