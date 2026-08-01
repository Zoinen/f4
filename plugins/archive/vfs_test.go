package archive

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/tar"
	"github.com/unxed/zip"
	"github.com/unxed/zipper/archive"
)

func TestArchiveVFS_PathSlashes(t *testing.T) {
	v := &ArchiveVFS{
		arcPath:   filepath.FromSlash("C:/path/to/archive.zip"),
		innerPath: "folder/file.txt",
	}

	path := v.GetPath()
	expected := filepath.Join(v.arcPath, filepath.FromSlash(v.innerPath))

	if path != expected {
		t.Errorf("ArchiveVFS.GetPath slashes mismatch.\nGot:      %q\nExpected: %q", path, expected)
	}
}

func TestArchiveVFS_Abs(t *testing.T) {
	arcPath := filepath.FromSlash("/tmp/test.zip")
	v := &ArchiveVFS{
		arcPath:   arcPath,
		innerPath: "folder",
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Relative path inside archive",
			input:    "file.txt",
			expected: "/tmp/test.zip/folder/file.txt",
		},
		{
			name:     "Absolute path (full path with archive)",
			input:    "/tmp/test.zip/other",
			expected: "/tmp/test.zip/other",
		},
		{
			name:     "Root-style path inside archive",
			input:    "/manual/root",
			expected: "/manual/root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exp := filepath.ToSlash(filepath.Clean(tt.expected))
			got, _ := v.Abs(tt.input)
			if got != exp {
				t.Errorf("ArchiveVFS.Abs(%q): expected %q, got %q", tt.input, exp, got)
			}
		})
	}
}

func TestArchiveVFS_AtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	arcPath := filepath.Join(tmp, "test.zip")

	os.WriteFile(arcPath, []byte("PK\x05\x06\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"), 0644)
	origInfo, _ := os.Stat(arcPath)

	v, err := NewArchiveVFS(&vfs.OSVFS{}, arcPath)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	wc, err := v.Create(context.Background(), v.Join(arcPath, "newfile.txt"))
	if err != nil {
		t.Fatal(err)
	}

	wc.Write([]byte("some data"))

	currentInfo, _ := os.Stat(arcPath)
	if currentInfo.Size() != origInfo.Size() {
		t.Error("Original archive size changed BEFORE Close() - not atomic!")
	}
}

func TestArchiveVFS_TempFileLeak(t *testing.T) {
	tmpDir := t.TempDir()

	zipPath := filepath.Join(tmpDir, "test_leak.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("file.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("hello world"))
	zw.Close()
	f.Close()

	vOuter, err := NewArchiveVFS(&vfs.OSVFS{}, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer vOuter.Close()

	rc, err := vOuter.Open(context.Background(), vOuter.Join(zipPath, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}

	// Trigger lazy extraction which creates the temp file
	rc.ReadAt(context.Background(), make([]byte, 1), 0)

	var tempFilePath string
	if wrapper, ok := rc.(*vfs.TempFileWrapper); ok {
		tempFilePath = wrapper.TempPath
	} else if wrapper, ok := rc.(interface{ TempPath() string }); ok {
		tempFilePath = wrapper.TempPath()
	} else {
		t.Fatalf("Expected wrapper with TempPath, got %T", rc)
	}

	if _, err := os.Stat(tempFilePath); os.IsNotExist(err) {
		t.Fatalf("Temp file was not created at expected path: %s", tempFilePath)
	}
	t.Logf("Temp file created successfully at: %s", tempFilePath)

	rc.Close()

	if _, err := os.Stat(tempFilePath); err == nil {
		os.Remove(tempFilePath)
		t.Fatalf("TEST FAILED: Temp file %s was not deleted after Close()! Leak detected.", tempFilePath)
	}

	t.Log("SUCCESS: Temp file was properly deleted.")
}

// TestArchiveVFS_DeferredClose verifies that closing the ArchiveVFS is deferred
// while there are active readers or writers (grace period of inactivity).
func TestArchiveVFS_DeferredClose(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")

	// Create a zip with multiple files
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)

	w1, err := zw.Create("file1.txt")
	if err != nil {
		t.Fatal(err)
	}
	w1.Write([]byte("content1"))

	w2, err := zw.Create("file2.txt")
	if err != nil {
		t.Fatal(err)
	}
	w2.Write([]byte("content2"))

	zw.Close()
	f.Close()

	// 1. Open the archive VFS
	vArc, err := NewArchiveVFS(&vfs.OSVFS{}, zipPath)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Open the first file (simulates beginning of copy/extraction)
	rc1, err := vArc.Open(context.Background(), vArc.Join(zipPath, "file1.txt"))
	if err != nil {
		vArc.Close()
		t.Fatal(err)
	}
	rc1.Close()

	// 3. Simulate exiting the panel (closing the VFS) while extraction is active
	errClose := vArc.Close()
	if errClose != nil {
		t.Logf("vArc.Close() returned error: %v", errClose)
	}

	// 4. Try to open the second file immediately. It should succeed (grace period active)
	rc2, errRead2 := vArc.Open(context.Background(), vArc.Join(zipPath, "file2.txt"))
	if errRead2 != nil {
		t.Fatalf("BUG: Open file2 failed after VFS Close: %v. Expected to succeed due to active copy grace period.", errRead2)
	}
	rc2.Close()

	// 5. Wait for the inactivity timer to expire and perform cleanup (2-second grace period)
	time.Sleep(2500 * time.Millisecond)

	// 6. Try to open the file again. It should fail now as the VFS has been fully cleaned up.
	_, errRead3 := vArc.Open(context.Background(), vArc.Join(zipPath, "file1.txt"))
	if errRead3 == nil {
		t.Error("VFS failed to perform cleanup after inactivity grace period")
	}
}

type dummyFileInfo struct {
	name string
	size int64
}

func (d dummyFileInfo) Name() string       { return d.name }
func (d dummyFileInfo) Size() int64        { return d.size }
func (d dummyFileInfo) Mode() fs.FileMode  { return 0644 }
func (d dummyFileInfo) ModTime() time.Time { return time.Now() }
func (d dummyFileInfo) IsDir() bool        { return false }
func (d dummyFileInfo) Sys() any           { return nil }

type mockSlowFile struct {
	readBlock chan struct{}
}

func (m *mockSlowFile) Read(p []byte) (int, error) {
	<-m.readBlock
	return 0, io.EOF
}

func (m *mockSlowFile) Stat() (fs.FileInfo, error) {
	return dummyFileInfo{name: "somefile.txt", size: 100}, nil
}

func (m *mockSlowFile) Close() error {
	return nil
}

func TestArchiveReadWrapper_CloseNonBlocking(t *testing.T) {
	readBlock := make(chan struct{})
	f := &mockSlowFile{readBlock: readBlock}
	v := &ArchiveVFS{activeCount: 1}
	w := &archiveReadWrapper{
		v: v,
		f: f,
	}

	go func() {
		buf := make([]byte, 10)
		_, _ = w.ReadAt(context.Background(), buf, 0)
	}()

	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		w.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("archiveReadWrapper.Close() blocked because of active extraction holding the mutex!")
	}

	close(readBlock)
}

type mockSlowArchiveFS struct {
	archive.FileSystem
	openBlock chan struct{}
	readBlock chan struct{}
}

func (m *mockSlowArchiveFS) Open(name string) (fs.File, error) {
	if m.openBlock != nil {
		<-m.openBlock
	}
	return &mockSlowFile{readBlock: m.readBlock}, nil
}

func TestArchiveVFS_OpenCloseNonBlocking(t *testing.T) {
	openBlock := make(chan struct{})
	readBlock := make(chan struct{})

	v := &ArchiveVFS{
		fsys: &mockSlowArchiveFS{openBlock: openBlock, readBlock: readBlock},
	}

	ctx := context.WithValue(context.Background(), vfs.ReporterKey, &mockVFSProgressReporter{})

	openDone := make(chan struct{})
	var openErr error
	go func() {
		_, openErr = v.Open(ctx, "somefile.txt")
		close(openDone)
	}()

	time.Sleep(50 * time.Millisecond)

	closeDone := make(chan struct{})
	go func() {
		v.Close()
		close(closeDone)
	}()

	select {
	case <-closeDone:
		// Success
	case <-time.After(500 * time.Millisecond):
		t.Fatal("BUG REPRODUCED: ArchiveVFS.Close() blocked because v.Open() holds the mutex during extraction/seeking!")
	}

	close(openBlock)
	close(readBlock)
	<-openDone
	_ = openErr
}

type mockSeekingReporter struct {
	lastAction    string
	lastFilename  string
	lastTotalText string
	lastSpeedText string
}

func (r *mockSeekingReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *mockSeekingReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	r.lastAction = action
	r.lastFilename = filename
	r.lastTotalText = totalText
	r.lastSpeedText = speedText
}
func (r *mockSeekingReporter) IsCancelled() bool { return false }

func TestArchiveVFS_Open_SeekingProgress(t *testing.T) {
	openBlock := make(chan struct{})
	readBlock := make(chan struct{})

	v := &ArchiveVFS{
		fsys: &mockSlowArchiveFS{openBlock: openBlock, readBlock: readBlock},
	}

	reporter := &mockSeekingReporter{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))

	openDone := make(chan struct{})
	var openErr error
	go func() {
		_, openErr = v.Open(ctx, "somefile.txt")
		close(openDone)
	}()

	// 1. Verify "Locating file..." status and elapsed time presence
	time.Sleep(400 * time.Millisecond)
	if !strings.HasPrefix(reporter.lastTotalText, "Locating file") {
		t.Errorf("Expected 'Locating file...' status while Open is blocked, got %q", reporter.lastTotalText)
	}
	if !strings.Contains(reporter.lastSpeedText, "Time:") {
		t.Errorf("Expected elapsed time in 'Locating' phase, got %q", reporter.lastSpeedText)
	}
	close(openBlock)

	// 2. Verify "Seeking/Decompressing..." status and elapsed time presence
	time.Sleep(400 * time.Millisecond)
	if !strings.HasPrefix(reporter.lastTotalText, "Seeking/Decompressing") {
		t.Errorf("Expected 'Seeking/Decompressing...' status while Read is blocked, got %q", reporter.lastTotalText)
	}
	if !strings.Contains(reporter.lastSpeedText, "Time:") {
		t.Errorf("Expected elapsed time in 'Seeking' phase, got %q", reporter.lastSpeedText)
	}

	close(readBlock)
	<-openDone
	_ = openErr
}

type mockVFSProgressReporter struct {
	called  bool
	lastPct int
}

func (r *mockVFSProgressReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *mockVFSProgressReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	r.called = true
	r.lastPct = currentPct
}
func (r *mockVFSProgressReporter) IsCancelled() bool { return false }

func TestArchiveVFS_Open_ProgressReporting(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "progress_test.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("test.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("Progress test data"))
	zw.Close()
	f.Close()

	vArc, err := NewArchiveVFS(&vfs.OSVFS{}, zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer vArc.Close()

	callbackCalled := false
	var lastCallbackPct int
	progressCB := func(msg string, percent int) {
		callbackCalled = true
		lastCallbackPct = percent
	}

	reporter := &mockVFSProgressReporter{}

	ctx := context.Background()
	ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(progressCB))
	ctx = context.WithValue(ctx, vfs.ReporterKey, vfs.TaskReporter(reporter))

	rc, err := vArc.Open(ctx, vArc.Join(zipPath, "test.txt"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer rc.Close()

	if !callbackCalled {
		t.Error("ProgressCallback was not invoked during Open")
	}
	if !reporter.called {
		t.Error("TaskReporter was not invoked during Open")
	}
	if lastCallbackPct != 100 || reporter.lastPct != 100 {
		t.Errorf("Expected final progress to be 100%%, got Callback=%d%%, Reporter=%d%%", lastCallbackPct, reporter.lastPct)
	}

	buf := make([]byte, 18)
	n, err := rc.ReadAt(context.Background(), buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if n != 18 || string(buf) != "Progress test data" {
		t.Errorf("Read data mismatch: got %q, want 'Progress test data'", string(buf[:n]))
	}
}

type dummyReporter struct{}

func (d *dummyReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (d *dummyReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
}
func (d *dummyReporter) IsCancelled() bool { return false }

func TestArchiveVFSCopyBulk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "f4-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "test.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}

	zw := zip.NewWriter(f)
	w1, err := zw.Create("file1.txt")
	if err != nil {
		t.Fatal(err)
	}
	w1.Write([]byte("content1"))

	w2, err := zw.Create("folder/file2.txt")
	if err != nil {
		t.Fatal(err)
	}
	w2.Write([]byte("content2"))

	zw.Close()
	f.Close()

	parentVFS := vfs.NewOSVFS(tmpDir)
	archiveVFS, err := NewArchiveVFS(parentVFS, "test.zip")
	if err != nil {
		t.Fatalf("failed to create ArchiveVFS: %v", err)
	}
	defer archiveVFS.Close()

	dstDir := filepath.Join(tmpDir, "extracted")
	dstVFS := vfs.NewOSVFS(dstDir)
	err = dstVFS.MkDir(context.Background(), dstDir)
	if err != nil {
		t.Fatal(err)
	}

	copier, ok := interface{}(archiveVFS).(vfs.BulkCopier)
	if !ok {
		t.Fatal("expected ArchiveVFS to implement BulkCopier")
	}

	err = copier.CopyBulk(context.Background(), []string{"file1.txt", "folder/file2.txt"}, dstVFS, dstDir, &dummyReporter{})
	if err != nil {
		t.Fatalf("Bulk copy failed: %v", err)
	}

	f1, err := dstVFS.Open(context.Background(), filepath.Join(dstDir, "file1.txt"))
	if err != nil {
		t.Fatal("file1.txt was not extracted")
	}
	defer f1.Close()
	data1, _ := io.ReadAll(ctxReader{r: f1, ctx: context.Background()})
	if string(data1) != "content1" {
		t.Errorf("expected content1, got %q", string(data1))
	}

	f2, err := dstVFS.Open(context.Background(), filepath.Join(dstDir, "folder/file2.txt"))
	if err != nil {
		t.Fatal("folder/file2.txt was not extracted")
	}
	defer f2.Close()
	data2, _ := io.ReadAll(ctxReader{r: f2, ctx: context.Background()})
	if string(data2) != "content2" {
		t.Errorf("expected content2, got %q", string(data2))
	}
}
func TestArchiveVFSCopyBulk_Tar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "f4-test-tar-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, "test.tar")
	f, err := os.Create(tarPath)
	if err != nil {
		t.Fatal(err)
	}

	tw := tar.NewWriter(f)
	hdr1 := &tar.Header{
		Name: "file1.txt",
		Mode: 0644,
		Size: 8,
	}
	if err := tw.WriteHeader(hdr1); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("content1"))

	hdr2 := &tar.Header{
		Name: "folder/file2.txt",
		Mode: 0644,
		Size: 8,
	}
	if err := tw.WriteHeader(hdr2); err != nil {
		t.Fatal(err)
	}
	tw.Write([]byte("content2"))

	tw.Close()
	f.Close()

	parentVFS := vfs.NewOSVFS(tmpDir)
	archiveVFS, err := NewArchiveVFS(parentVFS, "test.tar")
	if err != nil {
		t.Fatalf("failed to create ArchiveVFS: %v", err)
	}
	defer archiveVFS.Close()

	dstDir := filepath.Join(tmpDir, "extracted")
	dstVFS := vfs.NewOSVFS(dstDir)
	err = dstVFS.MkDir(context.Background(), dstDir)
	if err != nil {
		t.Fatal(err)
	}

	copier, ok := interface{}(archiveVFS).(vfs.BulkCopier)
	if !ok {
		t.Fatal("expected ArchiveVFS to implement BulkCopier")
	}

	err = copier.CopyBulk(context.Background(), []string{"file1.txt", "folder/file2.txt"}, dstVFS, dstDir, &dummyReporter{})
	if err != nil {
		t.Fatalf("Bulk copy failed: %v", err)
	}

	f1, err := dstVFS.Open(context.Background(), filepath.Join(dstDir, "file1.txt"))
	if err != nil {
		t.Fatal("file1.txt was not extracted")
	}
	defer f1.Close()
	data1, _ := io.ReadAll(ctxReader{r: f1, ctx: context.Background()})
	if string(data1) != "content1" {
		t.Errorf("expected content1, got %q", string(data1))
	}

	f2, err := dstVFS.Open(context.Background(), filepath.Join(dstDir, "folder/file2.txt"))
	if err != nil {
		t.Fatal("folder/file2.txt was not extracted")
	}
	defer f2.Close()
	data2, _ := io.ReadAll(ctxReader{r: f2, ctx: context.Background()})
	if string(data2) != "content2" {
		t.Errorf("expected content2, got %q", string(data2))
	}
}

func TestArchiveVFSCopyBulk_ConcurrentQueue(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "f4-test-queue-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	zipPath := filepath.Join(tmpDir, "test.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w1, _ := zw.Create("file1.txt")
	w1.Write([]byte("content1"))
	zw.Close()
	f.Close()

	parentVFS := vfs.NewOSVFS(tmpDir)
	archiveVFS, err := NewArchiveVFS(parentVFS, "test.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer archiveVFS.Close()

	absPath := archiveVFS.activePath()

	// 1. Manually lock the archive file path from the main thread
	if !vfs.GlobalArchiveLockManager.TryLock(absPath) {
		t.Fatal("expected to acquire lock")
	}

	dstDir := filepath.Join(tmpDir, "extracted")
	dstVFS := vfs.NewOSVFS(dstDir)
	dstVFS.MkDir(context.Background(), dstDir)

	copier := interface{}(archiveVFS).(vfs.BulkCopier)

	// 2. Start CopyBulk in a background goroutine.
	// It should block waiting for the lock.
	var wg sync.WaitGroup
	wg.Add(1)
	copyFinished := false

	go func() {
		defer wg.Done()
		// Inject AutoQueue into the context so the VFS doesn't block waiting for UI interaction
		ctx := context.WithValue(context.Background(), "AutoQueue", true)
		err := copier.CopyBulk(ctx, []string{"file1.txt"}, dstVFS, dstDir, &dummyReporter{})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		copyFinished = true
	}()

	// Sleep to let the goroutine block on Lock() inside CopyBulk
	time.Sleep(50 * time.Millisecond)
	if copyFinished {
		t.Fatal("expected CopyBulk to be blocked on lock")
	}

	// 3. Unlock the file from the main thread, which should wake up the CopyBulk
	vfs.GlobalArchiveLockManager.Unlock(absPath)
	wg.Wait()

	if !copyFinished {
		t.Fatal("expected CopyBulk to finish successfully after unlock")
	}

	// Verify file is extracted
	f1, err := dstVFS.Open(context.Background(), filepath.Join(dstDir, "file1.txt"))
	if err != nil {
		t.Fatal("file1.txt was not extracted")
	}
	defer f1.Close()
	data1, _ := io.ReadAll(ctxReader{r: f1, ctx: context.Background()})
	if string(data1) != "content1" {
		t.Errorf("expected content1, got %q", string(data1))
	}
}

type mockTickerReporter struct {
	calls int
}

func (m *mockTickerReporter) UpdateScan(string, int64, int64) {}
func (m *mockTickerReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	m.calls++
}
func (m *mockTickerReporter) IsCancelled() bool { return false }

// TestIssue149_TimeTicksDuringBlockingIO ensures that the progress dialog receives
// continuous time updates even when the underlying archive I/O is blocked.
// This addresses the "Time updates unevenly/jumps" user claim.
func TestIssue149_TimeTicksDuringBlockingIO(t *testing.T) {
	ctx := context.Background()
	done := make(chan struct{})
	rep := &mockTickerReporter{}

	getStatus := func() (string, string, int) {
		return "Extracting", "file.txt", 50
	}

	go runProgressTicker(ctx, done, rep, getStatus)

	// Simulate blocking I/O for 1.2 seconds
	time.Sleep(1200 * time.Millisecond)
	close(done)

	// The ticker runs every 250ms. In 1.2 seconds, it should tick at least 4 times.
	if rep.calls < 4 {
		t.Errorf("Expected progress ticker to fire at least 4 times during blocking I/O, but got %d", rep.calls)
	}
}
