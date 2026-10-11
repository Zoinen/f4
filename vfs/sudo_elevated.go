package vfs

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"time"
)

// An elevated dispatcher on Windows (f4#1768) cannot tell who is calling from
// the socket the way the Unix one can: the file system permission that keeps
// other users off a root socket does not keep another program of the same
// user off an administrator's. UAC asked this user for consent once, for f4;
// a program that merely finds the socket must not inherit it. So the dispatcher
// is started with a one-time token that only the f4 that launched it knows, and
// answers nothing until a client has presented it.

const (
	sudoTokenBytes   = 32
	sudoHelloTimeout = 5 * time.Second
)

// ErrSudoAccessDenied is what a dispatcher answers to a wrong token.
var ErrSudoAccessDenied = errors.New("access denied: wrong elevation token")

// NewSudoToken returns a fresh random token for one dispatcher.
func NewSudoToken() (string, error) {
	var raw [sudoTokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate elevation token: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

// DialElevated connects to the dispatcher listening on sockPath and presents
// token. The connection is returned only once the dispatcher has accepted it.
func DialElevated(sockPath, token string, timeout time.Duration) (*net.UnixConn, error) {
	addr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, err
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	if err := sendMsg(conn, SudoRequest{Cmd: CmdHello, Path: token}, -1); err != nil {
		_ = conn.Close()
		return nil, err
	}
	var resp SudoResponse
	if _, err := recvMsg(conn, &resp); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if resp.Error != "" {
		_ = conn.Close()
		if resp.Error == ErrSudoAccessDenied.Error() {
			return nil, ErrSudoAccessDenied
		}
		return nil, errors.New(resp.Error)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

// ServeElevated accepts clients on l, one at a time, and serves the first
// that presents token; it returns when that client goes away. A client with the
// wrong token, or one that says nothing within sudoHelloTimeout, is dropped and
// the next is accepted. After the hello, CmdPing is answered here and every
// other request is passed to handle.
func ServeElevated(l *net.UnixListener, token string, handle func(SudoRequest) SudoResponse) error {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			return err
		}
		if !acceptSudoHello(conn, token) {
			_ = conn.Close()
			continue
		}
		serveSudoSession(conn, handle)
		_ = conn.Close()
		return nil
	}
}

func acceptSudoHello(conn *net.UnixConn, token string) bool {
	_ = conn.SetReadDeadline(time.Now().Add(sudoHelloTimeout))
	var req SudoRequest
	if _, err := recvMsg(conn, &req); err != nil {
		return false
	}
	_ = conn.SetReadDeadline(time.Time{})
	if req.Cmd != CmdHello || subtle.ConstantTimeCompare([]byte(req.Path), []byte(token)) != 1 {
		_ = sendMsg(conn, SudoResponse{Error: ErrSudoAccessDenied.Error()}, -1)
		return false
	}
	return sendMsg(conn, SudoResponse{}, -1) == nil
}

func serveSudoSession(conn *net.UnixConn, handle func(SudoRequest) SudoResponse) {
	for {
		var req SudoRequest
		if _, err := recvMsg(conn, &req); err != nil {
			return
		}
		var resp SudoResponse
		switch {
		case req.Cmd == CmdPing:
		case req.Cmd == CmdHello:
			resp.Error = "already authenticated"
		case handle == nil:
			resp.Error = "operation not supported by this dispatcher"
		default:
			resp = handle(req)
		}
		if err := sendMsg(conn, resp, -1); err != nil {
			return
		}
	}
}
