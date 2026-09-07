//go:build !windows

package main

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestOpenSolarisPTY_AllocationSequence(t *testing.T) {
	mock := NewMockSolarisStreams()

	// Запуск выделения PTY через наш оркестратор
	pty, err := OpenSolarisPTY(mock)
	if err != nil {
		t.Fatalf("OpenSolarisPTY failed: %v", err)
	}
	t.Cleanup(func() {
		if err := pty.Close(); err != nil {
			t.Errorf("close Solaris PTY: %v", err)
		}
	})

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

	// 4. Проверяем, что владение и блокировка сняты до открытия слейва.
	if !mock.granted {
		t.Error("GrantPt was never called: /dev/pts/N stays root:sys 0620 and open() fails with EACCES")
	}
	if !mock.unlocked {
		t.Error("UnlockPt was never called: PTLOCK stays set and ptsopen() fails with EAGAIN")
	}
}

// TestOpenSolarisPTY_RequiresGrant проверяет, что без передачи владения
// выделение падает, а не молча продолжается. Это регрессионный тест на
// первую половину issue #444: реальный EACCES с OpenIndiana приходил именно
// отсюда.
func TestOpenSolarisPTY_RequiresGrant(t *testing.T) {
	mock := NewMockSolarisStreams()
	mock.refuseGrant = true

	pty, err := OpenSolarisPTY(mock)
	if err == nil {
		_ = pty.Close() // Cleanup is secondary to the unexpected allocation success.
		t.Fatal("OpenSolarisPTY succeeded without grantpt; the kernel would have refused with EACCES")
	}
	if !errors.Is(err, unix.EACCES) {
		t.Errorf("Expected EACCES without grantpt, got %v", err)
	}
}

// TestOpenSolarisPTY_RequiresUnlock — то же для второго барьера. Он
// независим от первого: починка одних прав оставила бы EAGAIN.
func TestOpenSolarisPTY_RequiresUnlock(t *testing.T) {
	mock := NewMockSolarisStreams()
	mock.refuseUnlock = true

	pty, err := OpenSolarisPTY(mock)
	if err == nil {
		_ = pty.Close() // Cleanup is secondary to the unexpected allocation success.
		t.Fatal("OpenSolarisPTY succeeded without unlockpt; ptsopen() would have refused with EAGAIN")
	}
	if !errors.Is(err, unix.EAGAIN) {
		t.Errorf("Expected EAGAIN without unlockpt, got %v", err)
	}
}
