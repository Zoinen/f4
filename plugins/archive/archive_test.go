package archive

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mholt/archives"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"github.com/unxed/zip"
)

func TestActionExtractArchive_Integrity(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	srcZip := filepath.Join(tmpDir, "source.zip")

	f, _ := os.Create(srcZip)
	zw := zip.NewWriter(f)
	fw, _ := zw.Create("extracted.txt")
	if _, err := fw.Write([]byte("content to extract")); err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Create("empty_dir/"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "output")
	if err := os.Mkdir(destDir, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestActionExtractArchive_Encrypted7zPromptsForPassword(t *testing.T) {
	sevenZip, err := exec.LookPath("7z")
	if err != nil {
		t.Skip("7z command is not installed")
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(tmpDir, "secret.7z")
	// Keep headers visible: this is the real-world case where a wrong password
	// can be accepted while listing the archive and rejected only by payload
	// integrity validation during extraction.
	cmd := exec.Command(sevenZip, "a", "-t7z", "-pCorrect", "-bd", archivePath, "secret.txt")
	cmd.Dir = tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create encrypted 7z: %v: %s", err, output)
	}

	destDir := filepath.Join(tmpDir, "output")
	if err := os.Mkdir(destDir, 0700); err != nil {
		t.Fatal(err)
	}

	prompts := 0
	previousPrompt := archivePasswordPrompt
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		prompts++
		if prompts == 1 {
			return "Wrong", nil
		}
		return "Correct", nil
	}
	t.Cleanup(func() { archivePasswordPrompt = previousPrompt })

	app := &mockAppForProgress{
		t:          t,
		activeVfs:  vfs.NewOSVFS(tmpDir),
		passiveVfs: vfs.NewOSVFS(destDir),
		names:      []string{filepath.Base(archivePath)},
		done:       make(chan struct{}),
	}
	actionExtractArchive(app)
	<-app.done

	if prompts != 2 {
		t.Fatalf("password prompt count = %d, want 2", prompts)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret data" {
		t.Fatalf("extracted content = %q, want %q", data, "secret data")
	}
}

func TestZipCompression_Deflate(t *testing.T) {
	tmpDir := t.TempDir()
	arcPath := filepath.Join(tmpDir, "test.zip")

	data := []byte(strings.Repeat("A", 1000))
	filePath := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		t.Fatal(err)
	}

	out, err := os.Create(arcPath)
	if err != nil {
		t.Fatal(err)
	}

	z := archives.Zip{
		Compression: zip.Deflate,
	}

	files, err := archives.FilesFromDisk(context.Background(), nil, map[string]string{filePath: "data.txt"})
	if err != nil {
		t.Fatal(err)
	}

	err = z.Archive(context.Background(), out, files)
	if closeErr := out.Close(); closeErr != nil {
		t.Fatalf("close output archive: %v", closeErr)
	}

	if err != nil {
		t.Fatalf("Archiving failed: %v", err)
	}

	r, err := zip.OpenReader(arcPath)
	if err != nil {
		t.Fatalf("Failed to open resulting zip: %v", err)
	}
	defer func() { _ = r.Close() }()

	if len(r.File) == 0 {
		t.Fatal("Zip is empty")
	}

	if r.File[0].Method != zip.Deflate {
		t.Errorf("Compression method mismatch. Got %d, want %d (Deflate)", r.File[0].Method, zip.Deflate)
	}
}

type mockAppForProgress struct {
	t           *testing.T
	activeVfs   vfs.VFS
	passiveVfs  vfs.VFS
	names       []string
	progressPct []int
	progressMsg []string
	done        chan struct{}
	mu          sync.Mutex
}

func (m *mockAppForProgress) GetActivePanelVFS() vfs.VFS      { return m.activeVfs }
func (m *mockAppForProgress) GetPassivePanelVFS() vfs.VFS     { return m.passiveVfs }
func (m *mockAppForProgress) GetSelectedNames() []string      { return m.names }
func (m *mockAppForProgress) GetSelectedName() string         { return m.names[0] }
func (m *mockAppForProgress) RefreshAll()                     {}
func (m *mockAppForProgress) SetPendingSelection(name string) {}
func (m *mockAppForProgress) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	update := func(msg string, percent int) {
		m.mu.Lock()
		m.progressPct = append(m.progressPct, percent)
		m.progressMsg = append(m.progressMsg, msg)
		m.mu.Unlock()
	}

	err := worker(ctx, update)
	onComplete(err)
	close(m.done)
}

type mockReporter struct {
	m *mockAppForProgress
}

func (r *mockReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *mockReporter) IsCancelled() bool                                { return false }
func (r *mockReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	r.m.mu.Lock()
	r.m.progressPct = append(r.m.progressPct, totalPct)
	r.m.progressMsg = append(r.m.progressMsg, fmt.Sprintf("%s: %s | %s | %s", action, filename, totalText, speedText))
	r.m.mu.Unlock()
}

func (m *mockAppForProgress) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	reporter := &mockReporter{m: m}

	err := worker(ctx, reporter)
	onComplete(err)
	close(m.done)
}

func (m *mockAppForProgress) Message(title, msg string, buttons []string) int { return 0 }
func (m *mockAppForProgress) InputBox(title, prompt, defaultText string, callback func(string)) {
	callback(defaultText)
}
func (m *mockAppForProgress) Menu(title string, items []string, callback func(int)) {}

func TestActionExtractArchive_ProgressUpdates(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	srcZip := filepath.Join(tmpDir, "test_progress.zip")

	f, _ := os.Create(srcZip)
	zw := zip.NewWriter(f)
	fw, _ := zw.Create("file.txt")
	if _, err := fw.Write([]byte("some data")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "output")
	if err := os.Mkdir(destDir, 0700); err != nil {
		t.Fatal(err)
	}

	activeVfs := vfs.NewOSVFS(tmpDir)
	passiveVfs := vfs.NewOSVFS(destDir)

	app := &mockAppForProgress{
		t:          t,
		activeVfs:  activeVfs,
		passiveVfs: passiveVfs,
		names:      []string{"test_progress.zip"},
		done:       make(chan struct{}),
	}

	actionExtractArchive(app)
	<-app.done

	app.mu.Lock()
	defer app.mu.Unlock()

	if len(app.progressPct) == 0 {
		t.Error("Extraction progress percentage was never updated")
	}

	hasSpeedInfo := false
	for _, msg := range app.progressMsg {
		if strings.Contains(msg, "/s") && strings.Contains(msg, "files") && strings.Contains(msg, "Extracting") {
			hasSpeedInfo = true
			break
		}
	}
	if !hasSpeedInfo {
		t.Errorf("Expected extraction status message to contain real progress (speed and files), got: %v", app.progressMsg)
	}
}

func TestActionAddArchive_ProgressUpdates(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	activeVfs := vfs.NewOSVFS(tmpDir)

	app := &mockAppForProgress{
		t:          t,
		activeVfs:  activeVfs,
		passiveVfs: activeVfs,
		names:      []string{"file1.txt"},
		done:       make(chan struct{}),
	}

	actionAddArchive(app)
	<-app.done

	app.mu.Lock()
	defer app.mu.Unlock()

	if len(app.progressPct) == 0 {
		t.Error("Archiving progress percentage was never updated")
	}

	hasSpeedInfo := false
	for _, msg := range app.progressMsg {
		if strings.Contains(msg, "/s") && strings.Contains(msg, "files") && strings.Contains(msg, "Archiving") {
			hasSpeedInfo = true
			break
		}
	}
	if !hasSpeedInfo {
		t.Errorf("Expected archiving status message to contain real progress (speed and files), got: %v", app.progressMsg)
	}
}

type mockCancelApp struct {
	t          *testing.T
	activeVfs  vfs.VFS
	passiveVfs vfs.VFS
	names      []string
	done       chan struct{}
	err        error
}

func (m *mockCancelApp) GetActivePanelVFS() vfs.VFS      { return m.activeVfs }
func (m *mockCancelApp) GetPassivePanelVFS() vfs.VFS     { return m.passiveVfs }
func (m *mockCancelApp) GetSelectedNames() []string      { return m.names }
func (m *mockCancelApp) GetSelectedName() string         { return m.names[0] }
func (m *mockCancelApp) RefreshAll()                     {}
func (m *mockCancelApp) SetPendingSelection(name string) {}
func (m *mockCancelApp) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
	ctx, cancel := context.WithCancel(context.Background())
	update := func(msg string, percent int) {
		cancel()
	}
	m.err = worker(ctx, update)
	onComplete(m.err)
	close(m.done)
}

type mockCancelReporter struct {
	m      *mockCancelApp
	cancel context.CancelFunc
}

func (r *mockCancelReporter) UpdateScan(currentPath string, files, dirs int64) {}
func (r *mockCancelReporter) IsCancelled() bool                                { return false }
func (r *mockCancelReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	r.cancel()
}

func (m *mockCancelApp) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
	ctx, cancel := context.WithCancel(context.Background())
	reporter := &mockCancelReporter{m: m, cancel: cancel}
	m.err = worker(ctx, reporter)
	onComplete(m.err)
	close(m.done)
}

func (m *mockCancelApp) Message(title, msg string, buttons []string) int { return 0 }
func (m *mockCancelApp) InputBox(title, prompt, defaultText string, callback func(string)) {
	callback(defaultText)
}
func (m *mockCancelApp) Menu(title string, items []string, callback func(int)) {}

func TestActionExtractArchive_Cancellation(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	srcZip := filepath.Join(tmpDir, "test_cancel.zip")

	f, _ := os.Create(srcZip)
	zw := zip.NewWriter(f)
	for i := 0; i < 100; i++ {
		fw, _ := zw.Create(fmt.Sprintf("file_%d.txt", i))
		if _, err := fw.Write([]byte("some data to simulate work")); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "output_cancel")
	if err := os.Mkdir(destDir, 0700); err != nil {
		t.Fatal(err)
	}

	activeVfs := vfs.NewOSVFS(tmpDir)
	passiveVfs := vfs.NewOSVFS(destDir)

	app := &mockCancelApp{
		t:          t,
		activeVfs:  activeVfs,
		passiveVfs: passiveVfs,
		names:      []string{"test_cancel.zip"},
		done:       make(chan struct{}),
	}

	actionExtractArchive(app)
	<-app.done

	if app.err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", app.err)
	}
}
