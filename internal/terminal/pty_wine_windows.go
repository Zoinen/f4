//go:build windows

package terminal

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/unxed/f4/vfs/hostmode"
	winescape "github.com/unxed/libwinescape/go"
	"github.com/unxed/vtui"
)

// The native terminal under Wine (WINE.md §18.3): the built-in terminal runs the
// host's own shell on a real pseudo-terminal, made and driven with libwinescape,
// where ConPTY is not available at all.
//
// It follows the UseWinescape setting, and follows it in one place: every
// decision to use it goes through hostmode.Posix(), which is false when the
// setting is off, when this is not Wine, and when the raw-syscall path does not
// work. cmd/f4/winescape_gate_test.go keeps it that way for any file that
// imports libwinescape.

// winePTYProbe records whether a pseudo-terminal can really be allocated on this
// host, once. Being under Wine with the library available is not the same
// thing: it is the allocation that proves /dev/ptmx and the ioctls work.
var winePTYProbe struct {
	once sync.Once
	ok   bool
}

func winePTYUsable() bool {
	if !hostmode.Posix() {
		return false
	}
	winePTYProbe.once.Do(func() {
		master, _, err := winescape.OpenPTY()
		if err != nil {
			vtui.DebugLog("PTY_WINE: native pty unavailable: %v", err)
			return
		}
		_ = winescape.Close(master)
		winePTYProbe.ok = true
		vtui.DebugLog("PTY_WINE: native host pty is available")
	})
	return winePTYProbe.ok
}

// nativeShellActive reports the POSIX shell personality, not PTY availability.
// Simple modes use the same answer when they have to run without a PTY.
func nativeShellActive() bool { return hostmode.Posix() }

// nativeSystemShell is GetSystemShell's answer in the POSIX Wine personality:
// the host's $SHELL rather than cmd.exe. PTY availability is a transport
// choice; it must not change the shell language used by simple modes.
func nativeSystemShell() (string, bool) {
	if !hostmode.Posix() {
		return "", false
	}
	return nativeShellName(winescape.HostGetenv("SHELL")), true
}

func isHostExecutable(path string) bool {
	const xOK = 1
	return winescape.Access(path, xOK) == nil
}

// Idle waits, in the polling loops below. The master is never read or written
// with a blocking call: a blocking raw syscall holds its OS thread and its P for
// as long as the child is quiet, and cannot be interrupted by Close. Polling
// with no timeout and sleeping in Go instead costs a few wake-ups a second for
// an idle terminal, and an echo latency of a few milliseconds.
const (
	winePTYIdleMin = 1 * time.Millisecond
	winePTYIdleMax = 8 * time.Millisecond
	winePTYChunk   = 1024
)

func nextIdle(d time.Duration) time.Duration {
	if d < winePTYIdleMin {
		return winePTYIdleMin
	}
	d *= 2
	if d > winePTYIdleMax {
		return winePTYIdleMax
	}
	return d
}

// winePTY is a PtyBackend on a host pseudo-terminal.
type winePTY struct {
	// mu guards every field and, more to the point, the master descriptor: a
	// step that uses it holds the read lock, and Close holds the write lock while
	// it releases it, so a descriptor number is never used after it was closed
	// and handed to something else.
	mu      sync.RWMutex
	master  int
	pid     int
	started bool
	closed  bool

	cols, rows, xpixel, ypixel int

	closeOnce sync.Once
}

func newWinePTY() (PtyBackend, bool, error) {
	if !winePTYUsable() {
		return nil, false, nil
	}
	registerPTYOpened()
	return &winePTY{master: -1, cols: 80, rows: 24}, true, nil
}

// PreservesLogicalLines: a kernel pty passes long lines through unwrapped, which
// is what lets the view reflow them.
func (p *winePTY) PreservesLogicalLines() bool { return true }

func (p *winePTY) winsize() winescape.Winsize {
	return winescape.Winsize{
		Row:    ptyPixels(p.rows),
		Col:    ptyPixels(p.cols),
		Xpixel: ptyPixels(p.xpixel),
		Ypixel: ptyPixels(p.ypixel),
	}
}

// nativeChildEnv is TerminalChildEnv on the host's environment: what Wine gives
// this process describes a Windows session (drive-letter PATH, and so on).
func nativeChildEnv(hostEnv []string) []string {
	if currentHostShellMode() == ShellModeHost {
		return withDefaultTerm(BuildChildEnv(hostEnv, false, false))
	}
	graphics := terminalShowsImages()
	return withDefaultTerm(BuildChildEnv(hostEnv, graphics, graphics && announceKittyTerm()))
}

func (p *winePTY) Run(name string, args ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return io.ErrClosedPipe
	}
	if p.started {
		return errors.New("native terminal: shell already started")
	}

	hostEnv, err := winescape.HostEnviron()
	if err != nil {
		return fmt.Errorf("native terminal: reading the host environment: %w", err)
	}
	exe, ok := resolveHostExecutable(name, hostEnvValue(hostEnv, "PATH"), isHostExecutable)
	if !ok {
		return fmt.Errorf("native terminal: cannot find %q on the host", name)
	}
	argv := append([]string{name}, args...)

	master, pid, err := winescape.StartPTY(exe, argv, nativeChildEnv(hostEnv), "", p.winsize())
	if err != nil {
		return fmt.Errorf("native terminal: starting %s: %w", exe, err)
	}
	p.master, p.pid, p.started = master, pid, true
	vtui.DebugLog("PTY_WINE: started %s as pid %d on host pty", exe, pid)
	return nil
}

// Read waits for output without ever blocking in the kernel; see the constants
// above. It returns io.EOF once the child has hung up or the terminal is closed.
func (p *winePTY) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	var idle time.Duration
	for {
		n, done, err := p.readStep(b)
		if done {
			return n, err
		}
		idle = nextIdle(idle)
		time.Sleep(idle)
	}
}

// readStep looks once: done is false when nothing was ready.
func (p *winePTY) readStep(b []byte) (n int, done bool, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed || p.master < 0 {
		return 0, true, io.EOF
	}
	fds := []winescape.PollFd{{Fd: int32(p.master), Events: winescape.POLLIN}}
	ready, perr := winescape.Poll(fds, 0)
	if perr != nil {
		return 0, true, perr
	}
	if ready == 0 {
		return 0, false, nil
	}
	if fds[0].Revents&winescape.POLLIN != 0 {
		m, rerr := winescape.Read(p.master, b)
		if m > 0 {
			return m, true, nil
		}
		if rerr != nil {
			if errors.Is(rerr, winescape.EIO) {
				return 0, true, io.EOF // the child closed its side
			}
			return 0, true, rerr
		}
		return 0, true, io.EOF
	}
	// Hang-up or error with nothing left to read.
	return 0, true, io.EOF
}

func (p *winePTY) Write(b []byte) (int, error) {
	written := 0
	var idle time.Duration
	for written < len(b) {
		chunk := b[written:]
		if len(chunk) > winePTYChunk {
			chunk = chunk[:winePTYChunk]
		}
		m, done, err := p.writeStep(chunk)
		written += m
		if err != nil {
			return written, err
		}
		if done {
			idle = 0
			continue
		}
		idle = nextIdle(idle)
		time.Sleep(idle)
	}
	return written, nil
}

// writeStep writes one chunk if the master will take it right now.
func (p *winePTY) writeStep(chunk []byte) (n int, done bool, err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed || p.master < 0 {
		return 0, false, io.ErrClosedPipe
	}
	fds := []winescape.PollFd{{Fd: int32(p.master), Events: winescape.POLLOUT}}
	ready, perr := winescape.Poll(fds, 0)
	if perr != nil {
		return 0, false, perr
	}
	if ready == 0 || fds[0].Revents&winescape.POLLOUT == 0 {
		return 0, false, nil
	}
	m, werr := winescape.Write(p.master, chunk)
	if werr != nil {
		if errors.Is(werr, winescape.EAGAIN) {
			return 0, false, nil
		}
		return 0, false, werr
	}
	return m, true, nil
}

func (p *winePTY) SetSize(cols, rows int) { p.SetSizePixels(cols, rows, 0, 0) }

// SetSizePixels implements PtyPixelSizer. Setting the size of the master
// delivers SIGWINCH to the foreground process group.
func (p *winePTY) SetSizePixels(cols, rows, xpixel, ypixel int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cols, p.rows, p.xpixel, p.ypixel = cols, rows, xpixel, ypixel
	if p.closed || p.master < 0 {
		return
	}
	ws := p.winsize()
	if err := winescape.SetWinsize(p.master, &ws); err != nil {
		vtui.DebugLog("PTY_WINE: resize to %dx%d failed: %v", cols, rows, err)
	}
}

// IsBusy reports whether something other than the shell is in the foreground of
// the terminal. The shell leads its own session, so its process group is its pid.
func (p *winePTY) IsBusy() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.started || p.closed || p.master < 0 {
		return false
	}
	pgrp, err := winescape.Tcgetpgrp(p.master)
	return err == nil && pgrp != p.pid
}

// Wait returns when the shell has exited. It reaps by polling with WNOHANG for
// the same reason Read polls.
func (p *winePTY) Wait() error {
	p.mu.RLock()
	pid, started := p.pid, p.started
	p.mu.RUnlock()
	if !started {
		return errors.New("native terminal: shell was not started")
	}
	var idle time.Duration
	for {
		var status int32
		got, err := winescape.Wait4(pid, &status, 1 /* WNOHANG */, nil)
		if err != nil {
			if errors.Is(err, winescape.ECHILD) {
				return nil // already reaped by Close
			}
			return err
		}
		if got == pid {
			ws := winescape.WaitStatus(status)
			switch {
			case ws.Exited() && ws.ExitStatus() == 0:
				return nil
			case ws.Exited():
				return fmt.Errorf("exit status %d", ws.ExitStatus())
			case ws.Signaled():
				return fmt.Errorf("signal %d", ws.Signal())
			}
			return nil
		}
		idle = nextIdle(idle) * 4 // the shell's exit is not latency-sensitive
		time.Sleep(idle)
	}
}

// Close hangs the shell up: it kills the whole process group, releases the
// master, and reaps the child in the background.
func (p *winePTY) Close() error {
	p.closeOnce.Do(func() {
		p.mu.RLock()
		pid, started := p.pid, p.started
		p.mu.RUnlock()
		if started {
			_ = winescape.Kill(-pid, winescape.SIGKILL)
			_ = winescape.Kill(pid, winescape.SIGKILL)
		}

		p.mu.Lock()
		p.closed = true
		master := p.master
		p.master = -1
		p.mu.Unlock()
		if master >= 0 {
			_ = winescape.Close(master)
		}
		registerPTYClosed()

		if started {
			go reapWinePTYChild(pid)
		}
	})
	return nil
}

// reapWinePTYChild collects the killed shell so it does not linger as a zombie.
func reapWinePTYChild(pid int) {
	for i := 0; i < 100; i++ {
		var status int32
		got, err := winescape.Wait4(pid, &status, 1 /* WNOHANG */, nil)
		if err != nil || got == pid {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
