//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
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
	shellPgrp int
}

func NewPTY() (*PTY, error) {
	masterFd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}

	master := os.NewFile(uintptr(masterFd), "/dev/ptmx")

	var res uintptr
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCGPTN, uintptr(unsafe.Pointer(&res))); err != 0 {
		master.Close()
		return nil, err
	}

	// TIOCSPTLCK is used on Linux to unlock the slave pty.
	// FreeBSD doesn't need/have it for /dev/ptmx.
	var unlock int
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(masterFd), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))

	slaveName := fmt.Sprintf("/dev/pts/%d", res)
	slaveFd, err := unix.Open(slaveName, unix.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}

	slave := os.NewFile(uintptr(slaveFd), slaveName)

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
		// Kill the whole process group because we used Setsid
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

	// Set initial size
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
		Row:    uint16(rows),
		Col:    uint16(cols),
		Xpixel: ptyPixels(xpixel),
		Ypixel: ptyPixels(ypixel),
	}
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, p.Master.Fd(), unix.TIOCSWINSZ, uintptr(unsafe.Pointer(&size)))
}

func GetSystemShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "/bin/sh"
	}
	return shell
}
