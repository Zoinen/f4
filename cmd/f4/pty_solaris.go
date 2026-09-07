//go:build solaris || illumos

package main

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"unsafe"

	"golang.org/x/sys/unix"
)

// NativeSolarisStreams реализует интерфейс SolarisStreamsAPI
// с помощью стабильных библиотечных оберток golang.org/x/sys/unix.
type NativeSolarisStreams struct{}

func NewNativeSolarisStreams() *NativeSolarisStreams {
	return &NativeSolarisStreams{}
}

func (n *NativeSolarisStreams) Open(path string, flag int, perm os.FileMode) (*os.File, error) {
	fd, err := unix.Open(path, flag|unix.O_CLOEXEC, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// I_PUSH в System V STREAMS равен (int('S')<<8) | 2 -> 0x5302
const I_PUSH = 0x5302

// Команды драйвера ptm, uts/common/sys/ptms.h. Передаются вниз по потоку
// через I_STR, а не обычным ioctl.
const (
	unlkpt  = ('P' << 8) | 2 // снять блокировку с пары master/subsidiary
	ownerpt = ('P' << 8) | 5 // назначить владельца узлу subsidiary
)

// ptOwn повторяет pt_own_t из uts/common/sys/ptms.h: uid_t и gid_t на
// illumos — 32-битные.
type ptOwn struct {
	Ruid uint32
	Rgid uint32
}

// defaultTTYGroup — группа, которую grantpt(3C) назначает подчинённому
// устройству, см. DEFAULT_TTY_GROUP в lib/libc/port/gen/pt.c.
const defaultTTYGroup = "tty"

func (n *NativeSolarisStreams) IoctlPush(f *os.File, module string) error {
	return unix.IoctlSetString(int(f.Fd()), I_PUSH, module)
}

// GrantPt повторяет grantpt(3C): назначает узлу /dev/pts/N владельцем
// вызывающего пользователя и группу tty. Без этого узел остаётся root:sys с
// правами 0620 (DEVPTS_DEVMODE_DEFAULT в fs/dev/sdev_ptsops.c), и open()
// подчинённого устройства возвращает EACCES обычному пользователю — проверка
// прав в VFS срабатывает раньше, чем ptsopen().
func (n *NativeSolarisStreams) GrantPt(master *os.File) error {
	owner := ptOwn{
		Ruid: uint32(os.Getuid()),
		Rgid: uint32(ttyGroupID()),
	}

	request := unix.Strioctl{
		Cmd: ownerpt,
		Len: int32(unsafe.Sizeof(owner)),
		Dp:  (*int8)(unsafe.Pointer(&owner)),
	}
	_, err := unix.IoctlSetStrioctlRetInt(int(master.Fd()), unix.I_STR, &request)
	runtime.KeepAlive(&owner)
	return err
}

// UnlockPt повторяет unlockpt(3C). ptmopen() выставляет PTMOPEN|PTLOCK, а
// ptsopen() (uts/common/io/pts.c) отказывает с EAGAIN, пока PTLOCK стоит.
func (n *NativeSolarisStreams) UnlockPt(master *os.File) error {
	request := unix.Strioctl{Cmd: unlkpt}
	_, err := unix.IoctlSetStrioctlRetInt(int(master.Fd()), unix.I_STR, &request)
	return err
}

// ttyGroupID повторяет выбор группы из grantpt(3C): группа tty, если она
// есть, иначе реальная группа процесса.
func ttyGroupID() int {
	if group, err := user.LookupGroup(defaultTTYGroup); err == nil {
		if gid, err := strconv.Atoi(group.Gid); err == nil {
			return gid
		}
	}
	return os.Getgid()
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
