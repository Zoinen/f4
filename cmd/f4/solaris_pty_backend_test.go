//go:build solaris || illumos

package main

import (
	"runtime"
	"testing"
)

func TestSolarisPTY_PtyBackend_Lifecycle(t *testing.T) {
	mock := NewMockSolarisStreams()

	// 1. Аллоцируем PTY через наш оркестратор
	pty, err := OpenSolarisPTY(mock)
	if err != nil {
		t.Fatalf("OpenSolarisPTY failed: %v", err)
	}

	// 2. Тестируем запуск дочернего процесса (PtyBackend.Run)
	// Запустим стандартную команду Unix "echo" для проверки потоков ввода-вывода.
	// Поскольку pty.Slave указывает на временный файл ОС, go-пакет os/exec
	// корректно свяжет дескрипторы для передачи текстового потока.
	err = pty.Run("echo", "hello-streams")
	if err != nil {
		t.Fatalf("PTY.Run failed: %v", err)
	}

	// 3. Тестируем ожидание завершения (PtyBackend.Wait)
	err = pty.Wait()
	if err != nil {
		t.Fatalf("PTY.Wait failed: %v", err)
	}

	// 4. Тестируем состояние занятости (PtyBackend.IsBusy)
	// После завершения процесса терминал должен быть свободен
	if pty.IsBusy() {
		t.Error("PTY should not be busy after process completes")
	}

	// 5. Тестируем закрытие (PtyBackend.Close)
	err = pty.Close()
	if err != nil {
		t.Fatalf("PTY.Close failed: %v", err)
	}
	runtime.KeepAlive(pty)
}

func TestSolarisPTY_IdleState_And_SetSize(t *testing.T) {
	mock := NewMockSolarisStreams()

	pty, err := OpenSolarisPTY(mock)
	if err != nil {
		t.Fatalf("OpenSolarisPTY failed: %v", err)
	}

	// 1. Тестируем вызов SetSize
	pty.SetSize(120, 43)
	if mock.lastCols != 120 || mock.lastRows != 43 {
		t.Errorf("SetSize failed to forward parameters. Got cols=%d, rows=%d", mock.lastCols, mock.lastRows)
	}

	// 2. Тестируем изменение состояния IsBusy
	mock.busyState = true
	if !pty.IsBusy() {
		t.Error("IsBusy should return true when mock state is true")
	}
	mock.busyState = false
	if pty.IsBusy() {
		t.Error("IsBusy should return false when mock state is false")
	}

	// 3. Тестируем вызов Wait на незапущенном PTY (должен завершиться без ошибок)
	err = pty.Wait()
	if err != nil {
		t.Errorf("Wait on idle PTY returned error: %v", err)
	}

	// 4. Тестируем закрытие незапущенного PTY (должно пройти успешно)
	err = pty.Close()
	if err != nil {
		t.Errorf("Close on idle PTY returned error: %v", err)
	}
}

func TestSolarisPTY_RawIO(t *testing.T) {
	mock := NewMockSolarisStreams()

	pty, err := OpenSolarisPTY(mock)
	if err != nil {
		t.Fatalf("OpenSolarisPTY failed: %v", err)
	}
	t.Cleanup(func() {
		if err := pty.Close(); err != nil {
			t.Errorf("close Solaris PTY: %v", err)
		}
	})

	// Запись в мастер-дескриптор
	inputData := []byte("test-data-io")
	written, err := pty.Write(inputData)
	if err != nil {
		t.Fatalf("PTY.Write failed: %v", err)
	}
	if written != len(inputData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(inputData), written)
	}

	// Чтение из мастер-дескриптора (поскольку это временный файл, читаем записанные данные)
	_, _ = pty.Master.Seek(0, 0)
	readBuf := make([]byte, len(inputData))
	read, err := pty.Read(readBuf)
	if err != nil {
		t.Fatalf("PTY.Read failed: %v", err)
	}

	if string(readBuf[:read]) != string(inputData) {
		t.Errorf("I/O data corruption: expected %q, got %q", string(inputData), string(readBuf[:read]))
	}
}
