package unpack

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/sevenzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type memoryWriteSeeker struct {
	data []byte
	off  int64
}

func (w *memoryWriteSeeker) Write(p []byte) (int, error) {
	end := w.off + int64(len(p))
	if w.off < 0 || end < w.off {
		return 0, errors.New("invalid memory write offset")
	}
	if end > int64(len(w.data)) {
		w.data = append(w.data, make([]byte, end-int64(len(w.data)))...)
	}
	copy(w.data[w.off:end], p)
	w.off = end
	return len(p), nil
}

func (w *memoryWriteSeeker) Seek(offset int64, whence int) (int64, error) {
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = w.off
	case io.SeekEnd:
		base = int64(len(w.data))
	default:
		return 0, errors.New("invalid seek origin")
	}
	next := base + offset
	if next < 0 {
		return 0, errors.New("negative seek offset")
	}
	w.off = next
	return next, nil
}

func TestExtractors(t *testing.T) {
	binaryContent := []byte("fake_executable_data")
	pluginContent := []byte("plugin_data")
	// Path Traversal items
	badAbsPath := "/etc/passwd"
	badRelPath := "../../windows/system32/cmd.exe"

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f1, err := zw.Create("f4.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f1.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	f2, err := zw.Create("plugins/dummy.dll")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.Write(pluginContent); err != nil {
		t.Fatal(err)
	}
	fBad1, err := zw.Create(badAbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fBad1.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	fBad2, err := zw.Create(badRelPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fBad2.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	destZip := t.TempDir()
	err = Zip(zipBuf.Bytes(), destZip)
	if err != nil {
		t.Fatalf("Zip failed: %v", err)
	}
	b1, _ := os.ReadFile(filepath.Join(destZip, "f4.exe"))
	b2, _ := os.ReadFile(filepath.Join(destZip, "plugins", "dummy.dll"))
	if string(b1) != "fake_executable_data" || string(b2) != "plugin_data" {
		t.Errorf("Zip extraction mismatch")
	}
	if _, err := os.Stat(filepath.Join(destZip, "etc", "passwd")); !os.IsNotExist(err) {
		t.Error("Zip Slip vulnerability detected (absolute path extracted)!")
	}

	var tgzBuf bytes.Buffer
	gw := gzip.NewWriter(&tgzBuf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: "f4", Size: int64(len(binaryContent)), Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "plugins/dummy.so", Size: int64(len(pluginContent)), Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(pluginContent); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: badAbsPath, Size: 6, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: badRelPath, Size: 6, Mode: 0644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	destTar := t.TempDir()
	err = TarGz(tgzBuf.Bytes(), destTar)
	if err != nil {
		t.Fatalf("TarGz failed: %v", err)
	}
	b1, _ = os.ReadFile(filepath.Join(destTar, "f4"))
	b2, _ = os.ReadFile(filepath.Join(destTar, "plugins", "dummy.so"))
	if string(b1) != "fake_executable_data" || string(b2) != "plugin_data" {
		t.Errorf("TarGz extraction mismatch")
	}

	var sevenBuf memoryWriteSeeker
	sw, err := sevenzip.NewWriter(&sevenBuf)
	if err != nil {
		t.Fatal(err)
	}
	sf1, err := sw.Create("f4.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf1.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	if err := sf1.Close(); err != nil {
		t.Fatal(err)
	}
	sf2, err := sw.Create("plugins/dummy.dll")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf2.Write(pluginContent); err != nil {
		t.Fatal(err)
	}
	if err := sf2.Close(); err != nil {
		t.Fatal(err)
	}
	sfBad, err := sw.Create(badAbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sfBad.Write([]byte("hacked")); err != nil {
		t.Fatal(err)
	}
	if err := sfBad.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sw.Close(); err != nil {
		t.Fatal(err)
	}
	sevenData := append([]byte(nil), sevenBuf.data...)
	dest7z := t.TempDir()
	err = SevenZip(sevenData, dest7z)
	if err != nil {
		t.Fatalf("SevenZip failed: %v", err)
	}
	b1, _ = os.ReadFile(filepath.Join(dest7z, "f4.exe"))
	b2, _ = os.ReadFile(filepath.Join(dest7z, "plugins", "dummy.dll"))
	if string(b1) != "fake_executable_data" || string(b2) != "plugin_data" {
		t.Errorf("7z extraction mismatch")
	}
	if _, err := os.Stat(filepath.Join(dest7z, "etc", "passwd")); !os.IsNotExist(err) {
		t.Error("7z Zip Slip vulnerability detected (absolute path extracted)!")
	}
	runtime.KeepAlive(sw)
}

func TestSanitizePathRejectsPlatformSeparators(t *testing.T) {
	for _, name := range []string{`..\\outside`, `folder\\..\\outside`, "nul\x00name"} {
		if _, err := SanitizePath(name, t.TempDir()); err == nil {
			t.Errorf("SanitizePath(%q) accepted an unsafe archive name", name)
		}
	}
}

func TestWriteFileSafeFallbackOldName(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "binary.exe")
	oldPath := targetPath + ".old"

	if err := os.WriteFile(targetPath, []byte("v1"), 0755); err != nil { // #nosec G306 -- the updater fixture represents an executable binary.
		t.Fatal(err)
	}

	// Make os.Remove(oldPath) fail by creating a non-empty directory
	if err := os.Mkdir(oldPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldPath, "lock"), []byte("lock"), 0600); err != nil {
		t.Fatal(err)
	}

	err := WriteFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err != nil {
		t.Fatalf("WriteFileSafe failed with fallback: %v", err)
	}

	b, _ := os.ReadFile(targetPath)
	if string(b) != "v2" {
		t.Errorf("Expected 'v2', got %q", string(b))
	}

	b, _ = os.ReadFile(oldPath + ".1")
	if string(b) != "v1" {
		t.Errorf("Expected old file to be renamed to .old.1, got %q", string(b))
	}
}

func TestWriteFileSafe(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "binary.exe")

	// 1. Initial write
	err := WriteFileSafe(targetPath, strings.NewReader("v1"), 0755)
	if err != nil {
		t.Fatalf("WriteFileSafe failed: %v", err)
	}

	b, _ := os.ReadFile(targetPath)
	if string(b) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(b))
	}

	// 2. Overwrite existing file
	err = WriteFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err != nil {
		t.Fatalf("WriteFileSafe overwrite failed: %v", err)
	}

	b, _ = os.ReadFile(targetPath)
	if string(b) != "v2" {
		t.Errorf("Expected 'v2', got %q", string(b))
	}
}

func TestWriteFileSafeSudoElevationFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific sudo elevation test on Windows")
	}

	tmpDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("failed to resolve temp dir: %v", err)
	}

	// Создаем директорию с ограниченными правами доступа (только чтение и выполнение)
	protectedDir := filepath.Join(tmpDir, "protected_dir")
	if err := os.Mkdir(protectedDir, 0555); err != nil { // #nosec G301 -- the read-only directory is the behavior under test.
		t.Fatalf("failed to create read-only dir: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(protectedDir, 0755); err != nil { // #nosec G302 -- cleanup must restore access to the deliberately locked directory.
			t.Errorf("restore protected directory permissions: %v", err)
		}
	})

	targetPath := filepath.Join(protectedDir, "binary.exe")

	// Проверяем, что система действительно запрещает запись под обычным пользователем
	_, errDirect := os.Create(targetPath)
	if errDirect == nil {
		t.Skip("System is running as root; skipping elevation test")
	}

	// Инициализируем глобальный SudoClient
	vfs.InitSudoClient("/nonexistent/f4", "")

	// Пытаемся записать файл. Операция должна пойти по пути эскалации и упасть
	// на попытке соединения с сокетом диспетчера (так как парольный диалог мы гасим),
	// что доказывает успешный переход управления в SudoClient!
	err = WriteFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err == nil {
		t.Error("expected WriteFileSafe to fail under restricted directory")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "elevated dispatcher") && !strings.Contains(errStr, "sudo process") {
		t.Errorf("expected error to originate from sudo elevation fallback, got: %q", errStr)
	}
}
