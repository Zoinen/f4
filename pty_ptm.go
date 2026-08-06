//go:build openbsd || netbsd

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/unxed/vtui"
	"golang.org/x/sys/unix"
)

type PTY struct {
	Master    *os.File
	Slave     *os.File
	Cmd       *exec.Cmd
	shellPgrp int
}

func NewPTY() (*PTY, error) {
	ptmFd, err := unix.Open("/dev/ptm", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(ptmFd)

	type ptmget struct {
		Cfd int32
		Sfd int32
		Cn  [16]byte
		Sn  [16]byte
	}

	var pg ptmget
	const PTMGET = 0x40287401

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(ptmFd), PTMGET, uintptr(unsafe.Pointer(&pg)))
	if errno != 0 {
		return nil, errno
	}

	master := os.NewFile(uintptr(pg.Cfd), "/dev/ptmx")
	slave := os.NewFile(uintptr(pg.Sfd), "slave")

	return &PTY{
		Master: master,
		Slave:  slave,
	}, nil
}

func (p *PTY) Write(b []byte) (int, error) {
	return p.Master.Write(b)
}

func (p *PTY) Read(b []byte) (int, error) {
	return p.Master.Read(b)
}

func (p *PTY) Close() error {
	vtui.DebugLog("PTY: Closing PTY and killing child process group")
	if p.Cmd != nil && p.Cmd.Process != nil {
		_ = syscall.Kill(-p.Cmd.Process.Pid, syscall.SIGKILL)
		p.Cmd.Process.Kill()
	}
	var err error
	if p.Master != nil {
		err = p.Master.Close()
	}
	if p.Slave != nil {
		p.Slave.Close()
	}
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

	p.SetSize(80, 24)

	err := p.Cmd.Start()
	if err == nil {
		p.shellPgrp, _ = syscall.Getpgid(p.Cmd.Process.Pid)
	}
	return err
}

func (p *PTY) IsBusy() bool {
	if p.Master == nil {
		return false
	}
	var pgrp int32
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
		Row: uint16(rows), Col: uint16(cols), Xpixel: ptyPixels(xpixel), Ypixel: ptyPixels(ypixel),
	}
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
