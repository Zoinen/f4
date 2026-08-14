//go:build !windows

package main

import (
	"os"
	"testing"
)

func TestMockSolarisStreams_Lifecycle(t *testing.T) {
	mock := NewMockSolarisStreams()

	// 1. Открываем мастер
	master, err := mock.Open("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("Failed to open mock master: %v", err)
	}
	defer master.Close()

	// 2. Передаем слейв во владение пользователю и снимаем блокировку.
	// Без этих двух шагов ядро не даст открыть /dev/pts/N.
	if err := mock.GrantPt(master); err != nil {
		t.Fatalf("GrantPt failed: %v", err)
	}
	if err := mock.UnlockPt(master); err != nil {
		t.Fatalf("UnlockPt failed: %v", err)
	}

	// 3. Получаем имя слейва
	slaveName, err := mock.GetPtsName(master)
	if err != nil {
		t.Fatalf("Failed to get slave name: %v", err)
	}
	if slaveName != "/dev/pts/42" {
		t.Errorf("Expected slave name '/dev/pts/42', got %q", slaveName)
	}

	// 4. Открываем слейв
	slave, err := mock.Open(slaveName, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("Failed to open mock slave: %v", err)
	}
	defer slave.Close()

	// 5. Пушим STREAMS модули (как это делает Illumos)
	err = mock.IoctlPush(slave, "ptem")
	if err != nil {
		t.Errorf("Failed to push ptem: %v", err)
	}
	err = mock.IoctlPush(slave, "ldterm")
	if err != nil {
		t.Errorf("Failed to push ldterm: %v", err)
	}

	// 6. Проверяем состояние мока
	mods := mock.pushedMods["last_slave"]
	if len(mods) != 2 || mods[0] != "ptem" || mods[1] != "ldterm" {
		t.Errorf("STREAMS modules pushed incorrectly: %v", mods)
	}
}
