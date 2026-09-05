package archive

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/sevenzip"
	"github.com/unxed/zip"
	"github.com/unxed/zipper/archive"
)

type mockOverwriteApp struct {
	t              *testing.T
	v              vfs.VFS
	names          []string
	messageCalled  bool
	progressCalled bool
	done           chan struct{}
}

func (m *mockOverwriteApp) GetActivePanelVFS() vfs.VFS      { return m.v }
func (m *mockOverwriteApp) GetPassivePanelVFS() vfs.VFS     { return m.v }
func (m *mockOverwriteApp) GetSelectedNames() []string      { return m.names }
func (m *mockOverwriteApp) GetSelectedName() string         { return m.names[0] }
func (m *mockOverwriteApp) RefreshAll()                     {}
func (m *mockOverwriteApp) SetPendingSelection(name string) {}
func (m *mockOverwriteApp) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
	m.progressCalled = true
	close(m.done)
}
func (m *mockOverwriteApp) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
	m.progressCalled = true
	close(m.done)
}
func (m *mockOverwriteApp) Message(title, msg string, buttons []string) int {
	m.messageCalled = true
	m.t.Logf("mockApp.Message called: %q - %q", title, msg)
	return 0
}
func (m *mockOverwriteApp) InputBox(title, prompt, defaultText string, callback func(string)) {
	m.t.Logf("mockApp.InputBox called with defaultText: %q", defaultText)
	callback(defaultText)
}
func (m *mockOverwriteApp) Menu(title string, items []string, callback func(int)) {}

func TestHangReproduction_RootChroot(t *testing.T) {
	var testPath string
	var chroot string
	if runtime.GOOS == "windows" {
		chroot = "C:\\"
		testPath = "C:\\Windows"
	} else {
		chroot = "/"
		testPath = "/etc"
	}

	fi, err := os.Lstat(testPath)
	if err != nil {
		t.Skipf("Skipping test because test path %q is not accessible: %v", testPath, err)
	}

	fileMap := map[string]os.FileInfo{
		testPath: fi,
	}

	tempDir := t.TempDir()
	archivePath := filepath.Join(tempDir, "repro_archive.zip")

	t.Logf("Creating archiver with archivePath=%q, chroot=%q", archivePath, chroot)

	done := make(chan struct{})
	var archiverErr error

	go func() {
		defer close(done)
		a, err := archive.NewArchiver(archivePath, chroot, archive.Options{Xattrs: true})
		if err != nil {
			archiverErr = err
			return
		}
		archiverErr = a.Archive(context.Background(), fileMap)
		if closeErr := a.Close(); archiverErr == nil {
			archiverErr = closeErr
		}
	}()

	select {
	case <-done:
		if archiverErr != nil {
			t.Logf("Archiving completed with error: %v", archiverErr)
		} else {
			t.Log("Archiving completed successfully without hanging.")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("TEST FAILED: Archiver hung! Reproduction of Issue #132 detected.")
	}
}

func TestActionAddArchive_OverwriteWarning(t *testing.T) {
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)

	dummyFile := v.Join(tmpDir, "file_to_archive.txt")
	if err := os.WriteFile(dummyFile, []byte("some content"), 0600); err != nil {
		t.Fatal(err)
	}

	archiveName := v.Base(tmpDir) + ".zip"
	existingArchive := v.Join(tmpDir, archiveName)
	if err := os.WriteFile(existingArchive, []byte("existing zip content"), 0600); err != nil {
		t.Fatal(err)
	}

	app := &mockOverwriteApp{
		t:     t,
		v:     v,
		names: []string{"file_to_archive.txt"},
		done:  make(chan struct{}),
	}

	t.Log("Calling actionAddArchive...")
	actionAddArchive(app)

	<-app.done

	if !app.messageCalled {
		t.Fatal("TEST FAILED: actionAddArchive silently overwrote the archive! Overwrite warning dialog was NOT shown.")
	}
	t.Log("SUCCESS: Overwrite warning dialog was shown before archiving.")
}

func TestIssue137_ArchiveOpenIsLazyAndContextAware(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "large.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, _ := zw.Create("large.bin")
	chunk := []byte(strings.Repeat("A", 1024*1024))
	for i := 0; i < 5; i++ {
		if _, err := w.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	vArc, err := NewArchiveVFS(&vfs.OSVFS{}, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vArc.Close(); err != nil {
			t.Errorf("close archive VFS: %v", err)
		}
	})

	// Test 1: Open should be fast and not block
	start := time.Now()
	rc, errOpen := vArc.Open(context.Background(), vArc.Join(zipPath, "large.bin"))
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("BUG REPRODUCED: Open took too long, likely synchronously extracting!")
	}
	defer func() { _ = rc.Close() }()

	// Test 2: ReadAt respects cancellation
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	buf := make([]byte, 100)
	_, errRead := rc.ReadAt(ctx, buf, 0)
	if errRead != context.Canceled {
		t.Fatalf("Expected context.Canceled from ReadAt, got: %v", errRead)
	}

	// Test 3: Read respects cancellation
	_, errRead2 := rc.Read(ctx, buf)
	if errRead2 != context.Canceled {
		t.Fatalf("Expected context.Canceled from Read, got: %v", errRead2)
	}
}

func TestArchivePlugin_ConcurrentOperationWarning(t *testing.T) {
	tmpDir := t.TempDir()
	v := vfs.NewOSVFS(tmpDir)

	dummyFile := filepath.Join(tmpDir, "test.zip")
	if err := os.WriteFile(dummyFile, []byte("dummy zip content"), 0600); err != nil {
		t.Fatal(err)
	}

	// Simulate an active operation already running for this archive
	absPath, _ := v.Abs(dummyFile)
	activeOps.Store(absPath, true)
	defer activeOps.Delete(absPath)

	app := &mockOverwriteApp{
		t:     t,
		v:     v,
		names: []string{"test.zip"},
		done:  make(chan struct{}),
	}

	actionExtractArchive(app)

	// Since mock app.Message returns 0 (Yes), it should proceed to extraction task
	<-app.done

	if !app.messageCalled {
		t.Error("Expected concurrency warning dialog to be shown, but it was not")
	}
}
func TestIssue150_Concurrent7zReadDir(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test_concurrent.7z")

	// 1. Создаем тестовое дерево папок и файлов
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "dir1/dir2"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dir1/file1.txt"), []byte("data1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "dir1/dir2/file2.txt"), []byte("data2"), 0600); err != nil {
		t.Fatal(err)
	}

	fileMap := make(map[string]os.FileInfo)
	if err := filepath.Walk(srcDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && p != srcDir {
			fileMap[p] = fi
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Сжимаем с флагом Solid
	a, err := archive.NewArchiver(archivePath, srcDir, archive.Options{Solid: true})
	if err != nil {
		t.Fatal(err)
	}
	err = a.Archive(context.Background(), fileMap)
	if closeErr := a.Close(); closeErr != nil {
		t.Fatalf("close concurrent archive fixture: %v", closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}

	// 2. Инициализируем наш VFS архива
	v, err := NewArchiveVFS(&vfs.OSVFS{}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Errorf("close archive VFS: %v", err)
		}
	})

	// 3. Запускаем конкурентный обход дерева файлов
	var wg sync.WaitGroup
	workers := 16
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				errRead := v.ReadDir(context.Background(), v.Join(archivePath, "dir1"), func(items []vfs.VFSItem) {
					// Заглушка
				})
				if errRead != nil {
					t.Errorf("ReadDir failed: %v", errRead)
				}
			}
		}()
	}
	wg.Wait()
	t.Log("SUCCESS: Concurrent 7z ReadDir executed without hangs or concurrent map write panics.")
}

func TestIssue150_7zDirectoryStructureAndSolid(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test_structure.7z")

	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(filepath.Join(srcDir, "empty_dir"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("solid content 1"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("solid content 2"), 0600); err != nil {
		t.Fatal(err)
	}

	fileMap := make(map[string]os.FileInfo)
	if err := filepath.Walk(srcDir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && p != srcDir {
			fileMap[p] = fi
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Запаковываем в Solid-режиме
	a, err := archive.NewArchiver(archivePath, srcDir, archive.Options{Solid: true})
	if err != nil {
		t.Fatal(err)
	}
	err = a.Archive(context.Background(), fileMap)
	if closeErr := a.Close(); closeErr != nil {
		t.Fatalf("close solid archive fixture: %v", closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}

	// Открываем архив низкоуровневым ридером для точечной инспекции
	zr, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()

	// Проверяем, что директории помечены как IsDir() и имеют размер 0 (а не дублируются как файлы)
	foundDir := false
	var commonStreamID = -1

	for _, file := range zr.File {
		isDir := file.FileInfo().IsDir()
		if isDir {
			foundDir = true
			if file.UncompressedSize != 0 {
				t.Errorf("Expected directory %q to have size 0, got %d", file.Name, file.UncompressedSize)
			}
		} else {
			// Для регулярных файлов в Solid режиме проверяем, что они лежат в ОДНОМ Solid-потоке (Stream)
			if file.UncompressedSize > 0 {
				if commonStreamID == -1 {
					commonStreamID = file.Stream
				} else if file.Stream != commonStreamID {
					t.Errorf("Expected Solid compression (single stream), but file %s is in Stream %d, whereas previous was in %d", file.Name, file.Stream, commonStreamID)
				}
			}
		}
	}

	if !foundDir {
		t.Error("Expected empty_dir directory entry in 7z archive, but none was found")
	}
	if commonStreamID == -1 {
		t.Error("No regular files with data found to verify solid stream")
	}

	t.Logf("SUCCESS: Solid compression verified (Stream ID: %d). Directory structures correctly preserved.", commonStreamID)
}
