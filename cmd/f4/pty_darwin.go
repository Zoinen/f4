//go:build darwin

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
)

// PTY handles pseudo-terminal allocation and process execution.
type PTY struct {
	Master    *os.File
	Slave     *os.File
	Cmd       *exec.Cmd
	closed    bool
	closeOnce sync.Once
	shellPgrp int
}

func NewPTY() (*PTY, error) {
	// The shell must never inherit the PTY master. If it does, an abnormal
	// parent exit cannot close the last master descriptor, so the shell never
	// receives a hangup and permanently consumes one of macOS's finite PTYs.
	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	// Put the master fd in non-blocking mode *before* wrapping it in
	// os.NewFile, same as every other platform; see pty_unix.go. Go only
	// registers a descriptor with the runtime poller when it is already
	// non-blocking at NewFile time. Setting it afterwards leaves os.File
	// in raw-syscall mode reading a non-blocking fd: every Read returns
	// EAGAIN immediately, and the terminal read loop treats that as fatal,
	// so the console goes permanently dark. The TIOCPTY* ioctls below take
	// the raw masterFd, so the ordering does not affect them.
	if err := unix.SetNonblock(masterFd, true); err != nil {
		unix.Close(masterFd)
		return nil, err
	}
	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")

	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCPTYGRANT, 0); e != 0 {
		master.Close()
		return nil, e
	}

	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCPTYUNLK, 0); e != 0 {
		master.Close()
		return nil, e
	}

	ptyName := make([]byte, 128)
	// #nosec G103 -- ioctl writes at most the 128-byte ptyName buffer during this synchronous syscall.
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCPTYGNAME, uintptr(unsafe.Pointer(&ptyName[0]))); e != 0 {
		master.Close()
		return nil, e
	}

	nameLen := bytes.IndexByte(ptyName, 0)
	if nameLen == -1 {
		nameLen = len(ptyName)
	}
	slaveName := string(ptyName[:nameLen])

	slaveFd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		master.Close()
		return nil, err
	}
	slave := os.NewFile(uintptr(slaveFd), slaveName)

	p := &PTY{
		Master: master,
		Slave:  slave,
	}
	registerPTYOpened()
	return p, nil
}

func (p *PTY) Write(b []byte) (int, error) {
	return p.Master.Write(b)
}

func (p *PTY) Read(b []byte) (int, error) {
	return p.Master.Read(b)
}

func (p *PTY) Close() error {
	var err error
	p.closeOnce.Do(func() {
		vtui.DebugLog("PTY: Closing PTY and killing child process group")
		if p.Cmd != nil && p.Cmd.Process != nil {
			_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
			p.Cmd.Process.Kill()
		}
		if p.Master != nil {
			err = p.Master.Close()
		}
		if p.Slave != nil {
			p.Slave.Close()
		}
		p.closed = true
		registerPTYClosed()
	})
	return err
}

func (p *PTY) Wait() error {
	return p.Cmd.Wait()
}

func (p *PTY) Run(name string, args ...string) error {
	p.Cmd = exec.Command(name, args...)
	p.Cmd.Stdin = p.Slave
	p.Cmd.Stdout = p.Slave
	p.Cmd.Stderr = p.Slave
	p.Cmd.Env = terminalChildEnv()
	p.Cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	// Set initial size
	p.SetSize(80, 24)

	err := p.Cmd.Start()
	if err == nil {
		// The child has duplicated the slave onto stdin/stdout/stderr. Keeping a
		// second copy in the parent delays hangup and needlessly retains the PTY.
		_ = p.Slave.Close()
		p.Slave = nil
		p.shellPgrp, _ = syscall.Getpgid(p.Cmd.Process.Pid)
	}
	return err
}

func (p *PTY) IsBusy() bool {
	if p.Master == nil {
		return false
	}
	var pgrp int32
	// #nosec G103 -- ioctl writes one int32 into this live stack variable during the synchronous syscall.
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, p.Master.Fd(), unix.TIOCGPGRP, uintptr(unsafe.Pointer(&pgrp)))
	if err != 0 {
		return false
	}
	return int(pgrp) != p.shellPgrp
}

func (p *PTY) SetSize(cols, rows int) {
	p.SetSizePixels(cols, rows, 0, 0)
}

// SetSizePixels also reports the size of the window in pixels, which is how
// a program in the terminal learns the shape of a character cell.
func (p *PTY) SetSizePixels(cols, rows, xpixel, ypixel int) {
	size := struct {
		Row, Col, Xpixel, Ypixel uint16
	}{
		Row: ptyPixels(rows), Col: ptyPixels(cols), Xpixel: ptyPixels(xpixel), Ypixel: ptyPixels(ypixel),
	}
	// #nosec G103 -- ioctl reads this fixed-size winsize-compatible stack struct only during the synchronous syscall.
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, p.Master.Fd(), unix.TIOCSWINSZ, uintptr(unsafe.Pointer(&size)))
}

func GetSystemShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	base := filepath.Base(shell)
	if base == "fish" || base == "csh" || base == "tcsh" {
		return "bash"
	}
	return shell
}
