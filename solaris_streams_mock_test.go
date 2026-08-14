//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// MockSolarisStreams реализует SolarisStreamsAPI для тестирования на Linux.
// Он имитирует поведение ядра Illumos при работе с /dev/ptmx и /dev/pts/N.
//
// Слейв, которым GetPtsName/Open отвечают тестам, — настоящий tty, а не
// обычный файл (см. newMockTTYSlave). SolarisPTY.Run() выставляет
// SysProcAttr.Setctty, а это ioctl(TIOCSCTTY) поверх stdin запущенного
// процесса, и на обычном файле он падает с ENOTTY. Мастер и остальная
// бухгалтерия STREAMS остаются полностью замоканы; реален только сам
// файловый дескриптор слейва.
type MockSolarisStreams struct {
	nextMinor  int
	openFiles  map[string]bool
	pushedMods map[string][]string // ключ: путь, значение: список STREAMS модулей
	lastCols   int
	lastRows   int
	busyState  bool

	// granted и unlocked воспроизводят состояние, которое ядро держит для
	// пары master/subsidiary. Пока владелец узла не переназначен на
	// вызывающего пользователя, open() слейва отбивается проверкой прав в
	// VFS (EACCES); пока стоит PTLOCK, отказывает уже ptsopen() (EAGAIN).
	// Мок обязан отказывать так же, иначе он подтвердит любую, в том числе
	// неверную, последовательность вызовов — ровно это и пропустило в
	// релиз баг #444.
	granted  bool
	unlocked bool

	// refuseGrant и refuseUnlock позволяют тесту сымитировать пропуск
	// соответствующего шага, не переписывая боевой код.
	refuseGrant  bool
	refuseUnlock bool
}

func NewMockSolarisStreams() *MockSolarisStreams {
	return &MockSolarisStreams{
		nextMinor:  42, // Случайный начальный номер псевдотерминала
		openFiles:  make(map[string]bool),
		pushedMods: make(map[string][]string),
		busyState:  false,
	}
}

func (m *MockSolarisStreams) Open(path string, flag int, perm os.FileMode) (*os.File, error) {
	if path == "/dev/ptmx" {
		m.openFiles[path] = true
		// Создаем фиктивный файл в /tmp для эмуляции дескриптора
		f, err := os.CreateTemp("", "mock_ptmx_*")
		if err == nil {
			os.Remove(f.Name()) // Unlink immediately to prevent leaks
		}
		return f, err
	}

	// Эмуляция открытия слейва /dev/pts/N
	if len(path) > 9 && path[:9] == "/dev/pts/" {
		if !m.granted {
			return nil, unix.EACCES
		}
		if !m.unlocked {
			return nil, unix.EAGAIN
		}
		m.openFiles[path] = true
		m.pushedMods[path] = []string{}
		f, err := newMockTTYSlave()
		if err != nil {
			return nil, err
		}
		return f, nil
	}

	return nil, os.ErrNotExist
}

func (m *MockSolarisStreams) IoctlPush(f *os.File, module string) error {
	// В тестах мы будем проверять f.Name(), но поскольку f - это TempFile,
	// мы опираемся на факт, что Ioctl применяется к слейву.
	// Для упрощения мока сохраняем модуль в глобальный список "последних пушей".
	m.pushedMods["last_slave"] = append(m.pushedMods["last_slave"], module)
	return nil
}

func (m *MockSolarisStreams) GrantPt(master *os.File) error {
	if master == nil {
		return errors.New("invalid master fd")
	}
	if m.refuseGrant {
		return nil // шаг "пропущен": состояние узла не меняется
	}
	m.granted = true
	return nil
}

func (m *MockSolarisStreams) UnlockPt(master *os.File) error {
	if master == nil {
		return errors.New("invalid master fd")
	}
	if m.refuseUnlock {
		return nil // шаг "пропущен": PTLOCK остается выставленным
	}
	m.unlocked = true
	return nil
}

func (m *MockSolarisStreams) GetPtsName(master *os.File) (string, error) {
	if master == nil {
		return "", errors.New("invalid master fd")
	}
	// Имитируем логику: ОС назначает следующий доступный minor number
	name := fmt.Sprintf("/dev/pts/%d", m.nextMinor)
	m.nextMinor++
	return name, nil
}

func (m *MockSolarisStreams) SetSize(master *os.File, cols, rows int) error {
	m.lastCols = cols
	m.lastRows = rows
	return nil
}

func (m *MockSolarisStreams) IsBusy(master *os.File, shellPgrp int) bool {
	return m.busyState
}
