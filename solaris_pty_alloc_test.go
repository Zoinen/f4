//go:build !windows

package main

import (
	"testing"
)

func TestOpenSolarisPTY_AllocationSequence(t *testing.T) {
	mock := NewMockSolarisStreams()

	// Запуск выделения PTY через наш оркестратор
	pty, err := OpenSolarisPTY(mock)
	if err != nil {
		t.Fatalf("OpenSolarisPTY failed: %v", err)
	}
	defer pty.Close()

	// 1. Проверяем правильность полученного имени слейва
	if pty.Name != "/dev/pts/42" {
		t.Errorf("Expected slave name '/dev/pts/42', got %q", pty.Name)
	}

	// 2. Проверяем, что файлы действительно открыты и сохранены в структуре
	if pty.Master == nil || pty.Slave == nil {
		t.Fatal("Master or Slave file object is nil")
	}

	// 3. Проверяем точную последовательность и полноту пуша модулей STREAMS
	// Ожидается ровно 3 модуля: ptem -> ldterm -> ttcompat
	expectedModules := []string{"ptem", "ldterm", "ttcompat"}
	pushed := mock.pushedMods["last_slave"]

	if len(pushed) != len(expectedModules) {
		t.Fatalf("Expected %d pushed modules, got %d: %v", len(expectedModules), len(pushed), pushed)
	}

	for i, expected := range expectedModules {
		if pushed[i] != expected {
			t.Errorf("Module mismatch at position %d: expected %q, got %q", i, expected, pushed[i])
		}
	}
}
