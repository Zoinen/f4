//go:build solaris || illumos

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// NativeSolarisStreams реализует интерфейс SolarisStreamsAPI
// с помощью стабильных библиотечных оберток golang.org/x/sys/unix.
type NativeSolarisStreams struct{}

func NewNativeSolarisStreams() *NativeSolarisStreams {
	return &NativeSolarisStreams{}
}

func (n *NativeSolarisStreams) Open(path string, flag int, perm os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, flag, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// I_PUSH в System V STREAMS равен (int('S')<<8) | 2 -> 0x5302
const I_PUSH = 0x5302

func (n *NativeSolarisStreams) IoctlPush(f *os.File, module string) error {
	return unix.IoctlSetString(int(f.Fd()), I_PUSH, module)
}

func (n *NativeSolarisStreams) GetPtsName(master *os.File) (string, error) {
	var stat unix.Stat_t
	err := unix.Fstat(int(master.Fd()), &stat)
	if err != nil {
		return "", err
	}
	// В Solaris минорный номер клонированного ptmx равен индексу pts-терминала
	minor := unix.Minor(stat.Rdev)
	return fmt.Sprintf("/dev/pts/%d", minor), nil
}

func (n *NativeSolarisStreams) SetSize(master *os.File, cols, rows int) error {
	ws := &unix.Winsize{Row: uint16(rows), Col: uint16(cols)}
	return unix.IoctlSetWinsize(int(master.Fd()), unix.TIOCSWINSZ, ws)
}

func (n *NativeSolarisStreams) IsBusy(master *os.File, shellPgrp int) bool {
	pgrp, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPGRP)
	if err != nil {
		return false
	}
	return pgrp != shellPgrp
}

// NewPTY инициализирует PTY-сессию на Solaris/Illumos через нативный STREAMS драйвер.
func NewPTY() (*SolarisPTY, error) {
	return OpenSolarisPTY(NewNativeSolarisStreams())
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
