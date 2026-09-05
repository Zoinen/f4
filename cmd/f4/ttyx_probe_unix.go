//go:build !windows

package main

// Measuring the terminal, on the systems that have a terminal to measure.

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// hostPixelsFromIoctl reads the pixel size of the text area out of the window
// size ioctl, which carries it beside the size in characters. Nearly every
// terminal fills it in, it costs one system call, and unlike a question asked
// over the wire it cannot be lost and cannot swallow a keystroke.
func hostPixelsFromIoctl(f *os.File) (int, int, bool) {
	if f == nil {
		return 0, 0, false
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return 0, 0, false
	}
	var w, h int
	var ok bool
	// Control rather than Fd: Fd takes the descriptor away from the
	// runtime poller and puts it back in blocking mode, which would be a
	// rude thing to do to the descriptor the whole session reads from.
	cerr := rc.Control(func(fd uintptr) {
		ws, err := unix.IoctlGetWinsize(int(fd), unix.TIOCGWINSZ)
		if err != nil || ws == nil || ws.Xpixel == 0 || ws.Ypixel == 0 {
			return
		}
		w, h, ok = int(ws.Xpixel), int(ws.Ypixel), true
	})
	if cerr != nil {
		return 0, 0, false
	}
	return w, h, ok
}

// queryPixels asks one XTWINOPS question and reads the answer with the given
// prefix. Standard input and output are used rather than /dev/tty: where f4
// runs as a server the terminal is the pair of descriptors the client handed
// over, and /dev/tty in that process is either nothing or somebody else's.
func queryPixels(ask, prefix string) (int, int, bool) {
	in, out := os.Stdin, os.Stdout
	if _, err := out.WriteString(ask); err != nil {
		return 0, 0, false
	}

	const budget = 300 * time.Millisecond
	answer, ok := readAnswer(in, budget, prefix)
	if !ok {
		return 0, 0, false
	}
	return parseXTWinOps(answer, prefix)
}

// readAnswer collects bytes until the answer is complete or the budget runs
// out.
//
// A read deadline is tried first, and where the descriptor is one the runtime
// can poll that is all it takes. It is not always: a descriptor passed from
// another process — which is exactly what a session attached to a running
// server reads from — arrives in blocking mode, the runtime does not poll it,
// and SetReadDeadline refuses. Returning there was the whole of the bug: the
// question was never even asked, both answers came back "no" in the same
// millisecond, and the picture was placed by treating the window as the grid.
func readAnswer(in *os.File, budget time.Duration, prefix string) (string, bool) {
	if err := in.SetReadDeadline(time.Now().Add(budget)); err == nil {
		defer in.SetReadDeadline(time.Time{})
		var sb strings.Builder
		buf := make([]byte, 128)
		for {
			n, err := in.Read(buf)
			if n > 0 {
				sb.Write(buf[:n])
				if answerComplete(sb.String(), prefix) {
					return sb.String(), true
				}
			}
			if err != nil {
				return sb.String(), sb.Len() > 0
			}
		}
	}
	return pollAnswer(in, budget, prefix)
}

// pollAnswer waits on the descriptor by hand, for the descriptors the runtime
// will not wait on for us.
func pollAnswer(in *os.File, budget time.Duration, prefix string) (string, bool) {
	rc, err := in.SyscallConn()
	if err != nil {
		return "", false
	}
	var sb strings.Builder
	cerr := rc.Control(func(fd uintptr) {
		if fd > 1<<31-1 {
			return
		}
		deadline := time.Now().Add(budget)
		buf := make([]byte, 128)
		for {
			left := time.Until(deadline)
			if left <= 0 {
				return
			}
			// #nosec G115 -- descriptors larger than PollFd's int32 field are rejected above.
			pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			n, err := unix.Poll(pfd, int(left/time.Millisecond)+1)
			if err == unix.EINTR {
				continue
			}
			if err != nil || n == 0 {
				return
			}
			r, err := unix.Read(int(fd), buf)
			if r > 0 {
				sb.Write(buf[:r])
				if answerComplete(sb.String(), prefix) {
					return
				}
			}
			if err != nil || r <= 0 {
				return
			}
		}
	})
	if cerr != nil {
		return "", false
	}
	return sb.String(), answerComplete(sb.String(), prefix)
}
