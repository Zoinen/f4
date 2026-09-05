package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

func TestRPCVFS_ReadDir(t *testing.T) {
	clientSess, serverSess := setupTestSessions(t)

	// Мокаем ответ от плагина
	serverSess.Register("VFS.ReadDir", func(data msgpack.RawMessage) (any, error) {
		var req map[string]string
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}

		// Проверяем, что запрос пришел в правильный драйв и путь
		if req["Drive"] == "dummy_drive" && req["Path"] == "/test" {
			return []vfs.VFSItem{
				{Name: "virtual_file.txt", Size: 4096, IsDir: false},
			}, nil
		}
		return []vfs.VFSItem{}, nil
	})

	// Инициализируем VFS-адаптер на стороне ядра
	v := NewRPCVFS(clientSess, "dummy_drive")

	var received []vfs.VFSItem
	err := v.ReadDir(context.Background(), "/test", func(chunk []vfs.VFSItem) {
		received = append(received, chunk...)
	})

	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}

	if len(received) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(received))
	}
	if received[0].Name != "virtual_file.txt" || received[0].Size != 4096 {
		t.Errorf("Unexpected item data: %+v", received[0])
	}
}

func TestRPCVFS_PathResolution(t *testing.T) {
	// Проверяем корректность работы встроенных методов (без RPC)
	v := NewRPCVFS(nil, "dummy")

	if !v.IsAtRoot() {
		t.Error("Should be at root initially")
	}

	if err := v.SetPath("/folder/sub"); err != nil {
		t.Fatal(err)
	}

	if v.IsAtRoot() {
		t.Error("Should not be at root after SetPath")
	}

	expectedPath := filepath.FromSlash("/folder/sub")
	if v.GetPath() != expectedPath {
		t.Errorf("Expected path %q, got %q", expectedPath, v.GetPath())
	}

	abs, _ := v.Abs("file.txt")
	expectedAbs := filepath.FromSlash("/folder/sub/file.txt")
	if abs != expectedAbs {
		t.Errorf("Abs failed: expected %q, got %q", expectedAbs, abs)
	}

	if v.Base("/folder/sub/file.txt") != "file.txt" {
		t.Errorf("Base failed: expected 'file.txt', got %q", v.Base("/folder/sub/file.txt"))
	}
}

func TestRPCVFS_NativeSlashes(t *testing.T) {
	v := NewRPCVFS(nil, "dummy")
	if err := v.SetPath("/linux/style/path"); err != nil {
		t.Fatal(err)
	}

	path := v.GetPath()
	expected := filepath.FromSlash("/linux/style/path")

	if path != expected {
		t.Errorf("GetPath failed to return native slashes. Got %q, expected %q", path, expected)
	}

	abs, _ := v.Abs("file.txt")
	expectedAbs := filepath.Join(expected, "file.txt")
	if abs != expectedAbs {
		t.Errorf("Abs failed to return native slashes. Got %q, expected %q", abs, expectedAbs)
	}
}
