//go:build !windows

package vtui

import (
	"os"
	"strings"
	"time"
)

// da1Sixel queries the primary device attributes and reports whether the
// terminal declares sixel (parameter 4). Best effort: a terminal that never
// answers yields false after a short budget, leaving detection to the
// environment result.
func da1Sixel() bool {
	in, out := os.Stdin, os.Stdout
	// Prefer the controlling terminal: stdin/stdout may be pipes even with a
	// tty attached (a daemonised server, a redirect). In a daemon there is no
	// controlling terminal, so this fails and the received PTY descriptors
	// are used instead.
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		in, out = tty, tty
	}
	_ = in.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	defer in.SetReadDeadline(time.Time{})
	_, _ = out.WriteString("\x1b[c")

	var sb strings.Builder
	buf := make([]byte, 128)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			if da1ResponseComplete(sb.String()) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return parseDA1Sixel(sb.String())
}

// QueryCellSize asks the terminal (CSI 16 t) for the pixel size of one cell.
func QueryCellSize() (cw, ch int, ok bool) {
	in, out := os.Stdin, os.Stdout
	if tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		defer tty.Close()
		in, out = tty, tty
	}
	_ = in.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	defer in.SetReadDeadline(time.Time{})
	_, _ = out.WriteString("\x1b[16t")

	var sb strings.Builder
	buf := make([]byte, 128)
	for {
		n, err := in.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
			if cellSizeResponseComplete(sb.String()) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return parseCellSizeResponse(sb.String())
}
