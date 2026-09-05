//go:build !windows

package vfs

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// mockSudoDispatcher имитирует поведение привилегированного демона
func mockSudoDispatcher(t *testing.T, l *net.UnixListener, stop chan struct{}) {
	conn, err := l.AcceptUnix()
	if err != nil {
		return
	}
	defer func() {
		_ = conn.Close() // connection cleanup errors are uninteresting
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}

		var req SudoRequest
		_, err := recvMsg(conn, &req)
		if err != nil {
			if err == io.EOF {
				return
			}
			t.Logf("mockSudoDispatcher recvMsg error: %v", err)
			return
		}

		resp := SudoResponse{}
		fdToSend := -1
		var tmpFileToClean *os.File

		switch req.Cmd {
		case CmdPing:
			// Успешный пустой ответ

		case CmdStat:
			if req.Path == "/protected/file.txt" {
				resp.Item = VFSItem{
					Name: "file.txt",
					Size: 1234,
				}
			} else {
				resp.Error = os.ErrNotExist.Error()
			}

		case CmdReadDir:
			if req.Path == "/protected" {
				resp.Items = []VFSItem{
					{Name: "file.txt", Size: 1234},
				}
			} else {
				resp.Error = os.ErrPermission.Error()
			}

		case CmdOpen:
			if req.Path == "/protected/file.txt" {
				tmp, err := os.CreateTemp("", "sudo-test-open-*")
				if err == nil {
					tmpFileToClean = tmp
					if _, err := tmp.Write([]byte("elevated content")); err != nil {
						t.Errorf("write temporary response file: %v", err)
						resp.Error = err.Error()
						break
					}
					if _, err := tmp.Seek(0, 0); err != nil {
						t.Errorf("rewind temporary response file: %v", err)
						resp.Error = err.Error()
						break
					}
					fdToSend = int(tmp.Fd())
				} else {
					resp.Error = err.Error()
				}
			} else {
				resp.Error = os.ErrPermission.Error()
			}

		case CmdMkDir:
			if req.Path == "/protected/newdir" {
				// Успешная имитация создания
			} else {
				resp.Error = os.ErrPermission.Error()
			}

		case CmdRemove:
			if req.Path == "/protected/del" {
				// Успешная имитация удаления
			} else {
				resp.Error = os.ErrPermission.Error()
			}

		case CmdRename:
			if req.Path == "/protected/old" && req.Path2 == "/protected/new" {
				// Успешная имитация переименования
			} else {
				resp.Error = os.ErrPermission.Error()
			}

		case CmdSetAttributes:
			if req.Path == "/protected/attr" && req.Item.UnixMode == 0600 {
				// Успешная имитация изменения атрибутов
			} else {
				resp.Error = os.ErrPermission.Error()
			}
		}

		if err := sendMsg(conn, resp, fdToSend); err != nil {
			t.Logf("mockSudoDispatcher sendMsg error: %v", err)
			if tmpFileToClean != nil {
				if err := tmpFileToClean.Close(); err != nil {
					t.Errorf("close temporary response file: %v", err)
				}
				if err := os.Remove(tmpFileToClean.Name()); err != nil {
					t.Errorf("remove temporary response file: %v", err)
				}
			}
			return
		}
		if tmpFileToClean != nil {
			if err := tmpFileToClean.Close(); err != nil {
				t.Errorf("close temporary response file: %v", err)
			}
			if err := os.Remove(tmpFileToClean.Name()); err != nil {
				t.Errorf("remove temporary response file: %v", err)
			}
		}
	}
}

func TestSudoClient_IPCProtocol(t *testing.T) {
	// Not t.TempDir(): on macOS its path is long enough to overflow
	// sun_path (~104 bytes) and bind fails with EINVAL.
	tmpDir := shortSocketDir(t)
	sockPath := filepath.Join(tmpDir, "sudo-test.sock")

	addr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		t.Fatalf("ResolveUnixAddr failed: %v", err)
	}

	l := listenUnixForTest(t, addr)
	defer func() {
		_ = l.Close() // listener cleanup errors are uninteresting
	}()

	stopChan := make(chan struct{})
	defer close(stopChan)

	go mockSudoDispatcher(t, l, stopChan)

	// Создаем SudoClient и вручную подключаем его к нашему сокету, минуя запуск sudo
	client := &SudoClient{
		sockPath: sockPath,
	}

	dialAddr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		t.Fatalf("ResolveUnixAddr for dial failed: %v", err)
	}
	conn, err := net.DialUnix("unix", nil, dialAddr)
	if err != nil {
		t.Fatalf("Failed to connect to mock dispatcher: %v", err)
	}
	client.conn = conn
	defer func() {
		_ = client.conn.Close() // connection cleanup errors are uninteresting
	}()

	// 1. Тест Stat
	t.Run("Stat Success", func(t *testing.T) {
		item, err := client.Stat("/protected/file.txt")
		if err != nil {
			t.Fatalf("Stat failed: %v", err)
		}
		if item.Name != "file.txt" || item.Size != 1234 {
			t.Errorf("Unexpected stat item: %+v", item)
		}
	})

	t.Run("Stat Error", func(t *testing.T) {
		_, err := client.Stat("/missing/path")
		if err == nil {
			t.Error("Expected error for missing path, got nil")
		}
	})

	// 2. Тест ReadDir
	t.Run("ReadDir Success", func(t *testing.T) {
		items, err := client.ReadDir("/protected")
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(items) != 1 || items[0].Name != "file.txt" {
			t.Errorf("Unexpected ReadDir result: %v", items)
		}
	})

	// 3. Тест Open (Передача файлового дескриптора)
	t.Run("Open and Transfer FD", func(t *testing.T) {
		f, err := client.Open("/protected/file.txt", os.O_RDONLY, 0)
		if err != nil {
			t.Fatalf("Open failed: %v", err)
		}
		defer func() { _ = f.Close() }()

		content, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("Failed to read from transferred file descriptor: %v", err)
		}

		if string(content) != "elevated content" {
			t.Errorf("Expected 'elevated content', got %q", string(content))
		}
	})

	// 4. Тест MkDir
	t.Run("MkDir", func(t *testing.T) {
		err := client.MkDir("/protected/newdir", 0755)
		if err != nil {
			t.Errorf("MkDir failed: %v", err)
		}
	})

	// 5. Тест Remove
	t.Run("Remove", func(t *testing.T) {
		err := client.Remove("/protected/del")
		if err != nil {
			t.Errorf("Remove failed: %v", err)
		}
	})

	// 6. Тест Rename
	t.Run("Rename", func(t *testing.T) {
		err := client.Rename("/protected/old", "/protected/new")
		if err != nil {
			t.Errorf("Rename failed: %v", err)
		}
	})

	// 7. Тест SetAttributes
	t.Run("SetAttributes", func(t *testing.T) {
		item := VFSItem{UnixMode: 0600}
		err := client.SetAttributes("/protected/attr", item)
		if err != nil {
			t.Errorf("SetAttributes failed: %v", err)
		}
	})
}

func TestSudoClient_DisconnectRecovery(t *testing.T) {
	tmpDir := shortSocketDir(t)
	sockPath := filepath.Join(tmpDir, "sudo-recovery.sock")

	addr, _ := net.ResolveUnixAddr("unix", sockPath)
	l := listenUnixForTest(t, addr)
	defer func() {
		_ = l.Close() // listener cleanup errors are uninteresting
	}()

	stopChan := make(chan struct{})
	defer close(stopChan)
	go mockSudoDispatcher(t, l, stopChan)

	client := &SudoClient{
		sockPath: sockPath,
	}

	// Успешно подключаемся к сокету
	dialAddr, err := net.ResolveUnixAddr("unix", sockPath)
	if err != nil {
		t.Fatalf("ResolveUnixAddr failed: %v", err)
	}
	conn, err := net.DialUnix("unix", nil, dialAddr)
	if err != nil {
		t.Fatalf("Failed to connect to dispatcher: %v", err)
	}
	client.conn = conn

	// Закрываем соединение, имитируя неожиданный обрыв связи
	if err := client.conn.Close(); err != nil {
		t.Fatalf("Close connection failed: %v", err)
	}

	// Первый вызов на закрытом сокете должен завершиться ошибкой и сбросить c.conn
	_, _, err = client.SendRequest(SudoRequest{Cmd: CmdPing})
	if err == nil {
		t.Fatal("Expected error on first write to a closed connection")
	}

	// Проверяем, что после ошибки соединение было сброшено в nil
	if client.conn != nil {
		t.Error("Expected client.conn to be set to nil after sendMsg failure")
	}
}

// shortSocketDir returns a freshly created directory with a path short
// enough for a unix socket: t.TempDir() on macOS easily exceeds the
// ~104-byte sun_path limit and bind(2) fails with EINVAL.
func shortSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "f4sock")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove short socket directory: %v", err)
		}
	})
	return dir
}

func listenUnixForTest(t *testing.T, addr *net.UnixAddr) *net.UnixListener {
	t.Helper()
	l, err := net.ListenUnix("unix", addr)
	if errors.Is(err, syscall.EPERM) {
		t.Skipf("sandbox does not permit Unix socket bind: %v", err)
	}
	if err != nil {
		t.Fatalf("ListenUnix failed: %v", err)
	}
	return l
}
