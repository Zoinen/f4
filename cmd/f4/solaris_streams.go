//go:build !windows

package main

import "os"

// SolarisStreamsAPI абстрагирует системные вызовы, необходимые для
// работы с подсистемой STREAMS и выделения PTY в Solaris/Illumos.
type SolarisStreamsAPI interface {
	// Open открывает устройство и возвращает файловый объект
	Open(path string, flag int, perm os.FileMode) (*os.File, error)

	// IoctlPush выполняет команду I_PUSH для добавления STREAMS модуля в стек дескриптора
	IoctlPush(f *os.File, module string) error

	// GrantPt передаёт узел подчинённого устройства во владение вызывающему
	// пользователю (аналог grantpt(3C)). До этого вызова /dev/pts/N
	// принадлежит root:sys с правами 0620, и открыть его непривилегированный
	// процесс не может.
	GrantPt(master *os.File) error

	// UnlockPt снимает блокировку с пары master/subsidiary (аналог
	// unlockpt(3C)). Открытие мастера выставляет PTLOCK, и до его снятия
	// ptsopen() отказывает.
	UnlockPt(master *os.File) error

	// GetPtsName вычисляет имя подчиненного (slave) терминала по дескриптору мастера
	GetPtsName(master *os.File) (string, error)

	// SetSize устанавливает размеры экрана для мастера
	SetSize(master *os.File, cols, rows int) error

	// IsBusy проверяет, активен ли подчиненный процесс в терминале
	IsBusy(master *os.File, shellPgrp int) bool
}
