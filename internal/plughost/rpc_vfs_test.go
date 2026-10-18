package plughost

import (
	"bytes"
	"context"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	msgpack "github.com/vmihailenco/msgpack/v5"
	"path/filepath"
	"testing"
)

func TestRPCVFS_ReadDir(t *testing.T) {
	clientSess, serverSess := testutil.RPCSessionPair(t)

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

func TestRPCVFS_CreateBuffersSmallWrites(t *testing.T) {
	clientSess, serverSess := testutil.RPCSessionPair(t)
	var got []byte
	writes, closes := 0, 0
	serverSess.Register("VFS.Create", func(data msgpack.RawMessage) (any, error) {
		return OpenRes{ID: 7}, nil
	})
	serverSess.Register("VFS.Write", func(data msgpack.RawMessage) (any, error) {
		var req WriteReq
		if err := msgpack.Unmarshal(data, &req); err != nil {
			return nil, err
		}
		writes++
		got = append(got, req.Data...)
		return nil, nil
	})
	serverSess.Register("VFS.CloseFile", func(data msgpack.RawMessage) (any, error) {
		closes++
		return nil, nil
	})

	w, err := NewRPCVFS(clientSess, "dummy").Create(context.Background(), "/file")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"first ", "second ", "third"} {
		if n, err := w.Write([]byte(part)); err != nil || n != len(part) {
			t.Fatalf("Write() = (%d, %v), want (%d, nil)", n, err, len(part))
		}
	}
	if writes != 0 {
		t.Fatalf("small writes made %d RPC calls before Close, want 0", writes)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if writes != 1 {
		t.Errorf("Close flushed with %d RPC writes, want 1", writes)
	}
	if closes != 1 {
		t.Errorf("Close made %d remote close calls, want 1", closes)
	}
	if want := []byte("first second third"); !bytes.Equal(got, want) {
		t.Errorf("remote data = %q, want %q", got, want)
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
