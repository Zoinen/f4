//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
)

// MockSolarisStreams реализует SolarisStreamsAPI для тестирования на Linux.
// Он имитирует поведение ядра Illumos при работе с /dev/ptmx и /dev/pts/N.
type MockSolarisStreams struct {
	nextMinor  int
	openFiles  map[string]bool
	pushedMods map[string][]string // ключ: путь, значение: список STREAMS модулей
	lastCols   int
	lastRows   int
	busyState  bool
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
		m.openFiles[path] = true
		m.pushedMods[path] = []string{}
		f, err := os.CreateTemp("", "mock_pts_*")
		if err == nil {
			os.Remove(f.Name()) // Unlink immediately to prevent leaks
		}
		return f, err
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
