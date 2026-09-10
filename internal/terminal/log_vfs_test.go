package terminal

import (
	"context"
	"github.com/unxed/vtui"
	"io"
	"strings"
	"testing"
)

func TestTerminalLogVFS_VFSInterface(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()

	// Наполняем лог терминала символами
	for _, r := range "line1\nline2\nline3" {
		tv.PutChar(r, DefaultTermAttr)
	}
	tv.FlushLog()

	v := &TerminalLogVFS{tv: tv}

	// 1. Тест IsAtRoot
	if !v.IsAtRoot() {
		t.Error("TerminalLogVFS should always be at root")
	}

	// 2. Тест GetPath
	if v.GetPath() != "term://" {
		t.Errorf("Unexpected path: %q", v.GetPath())
	}

	// 3. Тест IsAbs
	if !v.IsAbs("term://log") {
		t.Error("Paths starting with term:// should be treated as absolute")
	}
	if v.IsAbs("relative") {
		t.Error("Relative paths should not be treated as absolute")
	}

	// 4. Тест Stat
	stat, err := v.Stat(context.Background(), "Terminal Log")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if stat.IsDir {
		t.Error("Terminal Log should not be a directory")
	}
	if stat.Name != "Terminal Log" {
		t.Errorf("Expected name 'Terminal Log', got %q", stat.Name)
	}

	// 5. Тест Mutex/Capabilities
	caps := v.GetCapabilities()
	if !caps.HasRandomAccess {
		t.Error("TerminalLogVFS must support random access")
	}

	// 6. Мутационные методы (должны возвращать ошибки доступа)
	if err := v.MkDir(context.Background(), "newdir"); err == nil {
		t.Error("MkDir should return error")
	}
	if err := v.Remove(context.Background(), "log"); err == nil {
		t.Error("Remove should return error")
	}
	if err := v.Rename(context.Background(), "old", "new"); err == nil {
		t.Error("Rename should return error")
	}
	if _, err := v.Create(context.Background(), "newfile"); err == nil {
		t.Error("Create should return error")
	}
}

func TestTerminalLogVFS_ReadOperations(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()

	expectedContent := "terminal output data\nsecond line"
	for _, r := range expectedContent {
		tv.PutChar(r, DefaultTermAttr)
	}
	tv.FlushLog()

	v := &TerminalLogVFS{tv: tv}

	rc, err := v.Open(context.Background(), "Terminal Log")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer func() { _ = rc.Close() }()

	// 1. Проверяем размер лога
	if rc.Size() != int64(len(expectedContent)) {
		t.Errorf("Size mismatch: expected %d, got %d", len(expectedContent), rc.Size())
	}

	// 2. Чтение ReadAt (произвольный доступ)
	buf := make([]byte, 6)
	n, err := rc.ReadAt(context.Background(), buf, 9)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 6 || string(buf) != "output" {
		t.Errorf("Expected 'output', got %q", string(buf[:n]))
	}

	// 3. Чтение ReadAt на границе файла
	bufEnd := make([]byte, 20)
	nEnd, errEnd := rc.ReadAt(context.Background(), bufEnd, int64(len(expectedContent)-4))
	if errEnd != io.EOF {
		t.Errorf("Expected io.EOF on boundary read, got %v", errEnd)
	}
	if nEnd != 4 || string(bufEnd[:nEnd]) != "line" {
		t.Errorf("Expected 'line', got %q", string(bufEnd[:nEnd]))
	}

	// 4. Потоковое последовательное чтение через Read должно возвращать EOF сразу (или не поддерживаться)
	bufStream := make([]byte, 10)
	_, errStream := rc.Read(context.Background(), bufStream)
	if errStream != io.EOF {
		t.Errorf("Standard stream Read should immediately return io.EOF (random-access wrapper), got %v", errStream)
	}
}

func TestTerminalLogSnapshotPreservesSemanticViewportOffset(t *testing.T) {
	tv := seededTerminalSemanticHistory(t, 40, 8, 300)
	defer tv.Close()
	target := vtui.SemanticID(tv)
	tv.HandleSemanticAction(map[string]any{
		"target": target, "action": "terminal.viewport", "rows": 6,
	})
	tv.HandleSemanticAction(map[string]any{
		"target": target, "action": "terminal.scroll",
		"visualRow": 173, "followTail": false, "generation": uint64(1),
	})

	v := NewTerminalLogVFS(tv, nil)
	offset, ok := v.InitialOffset()
	if !ok {
		t.Fatal("terminal snapshot did not expose its initial offset")
	}
	if offset < 0 || offset >= int64(len(v.data)) {
		t.Fatalf("initial offset %d is outside %d-byte snapshot", offset, len(v.data))
	}
	if got := string(v.data[offset:]); !strings.HasPrefix(got, "row-000173 payload") {
		t.Fatalf("snapshot at offset %d starts %q, want row 173",
			offset, truncateForTest(got, 40))
	}

}

func truncateForTest(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
