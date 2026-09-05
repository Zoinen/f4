package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/sevenzip"
	"github.com/unxed/vtui"
)

type memoryWriteSeeker struct {
	data []byte
	off  int64
}

func TestUpdater_ParseUpdateHelperArgs(t *testing.T) {
	archive, kind, found, err := parseUpdateHelperArgs([]string{updateHelperFlag, `C:\Users\Test User\f4-update.archive`, "zip"})
	if err != nil || !found {
		t.Fatalf("parseUpdateHelperArgs() failed: found=%v err=%v", found, err)
	}
	if archive != `C:\Users\Test User\f4-update.archive` || kind != "zip" {
		t.Fatalf("parseUpdateHelperArgs() = %q, %q; want archive path and zip", archive, kind)
	}

	if _, _, found, err := parseUpdateHelperArgs([]string{updateHelperFlag, "archive.zip"}); !found || err == nil {
		t.Fatalf("malformed helper invocation: found=%v err=%v", found, err)
	}
	if _, _, found, err := parseUpdateHelperArgs([]string{"--gui=win32"}); found || err != nil {
		t.Fatalf("normal invocation parsed as update helper: found=%v err=%v", found, err)
	}
}

func TestUpdater_ManualBuildVersionUsesBuildTimestamp(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.LastUpdateVersion = ""

	buildTime := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	newerLocalBuild := githubRelease{TagName: "v0.2.0-beta", PublishedAt: "2026-08-20T12:00:00Z"}
	if stableReleaseNeedsUpdate(newerLocalBuild, "manual-build-sha", buildTime) {
		t.Fatal("a manual build newer than the release must not request a downgrade")
	}

	newerRelease := githubRelease{TagName: "v0.2.0-beta", PublishedAt: "2026-08-22T12:00:00Z"}
	if !stableReleaseNeedsUpdate(newerRelease, "manual-build-sha", buildTime) {
		t.Fatal("a release newer than a manual build must be offered")
	}

	if stableReleaseNeedsUpdate(newerRelease, "v0.2.0-beta", buildTime) {
		t.Fatal("an exact release build must not request an update to itself")
	}
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

func TestUpdater_ShouldCheck(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	now := time.Now().Unix()

	AppConfig.UpdateInterval = 0
	if shouldCheck() {
		t.Error("Should not check if interval is 0")
	}

	AppConfig.UpdateInterval = 1
	AppConfig.LastUpdateCheck = now
	if !shouldCheck() {
		t.Error("Should check every start")
	}

	AppConfig.UpdateInterval = 2
	AppConfig.LastUpdateCheck = now
	if shouldCheck() {
		t.Error("Should not check daily if just checked")
	}
	AppConfig.LastUpdateCheck = now - 25*3600
	if !shouldCheck() {
		t.Error("Should check daily if > 24h passed")
	}

	AppConfig.UpdateInterval = 3
	AppConfig.LastUpdateCheck = now - 2*24*3600
	if shouldCheck() {
		t.Error("Should not check weekly if < 7 days passed")
	}
	AppConfig.LastUpdateCheck = now - 8*24*3600
	if !shouldCheck() {
		t.Error("Should check weekly if > 7 days passed")
	}
}

func TestUpdater_CheckForUpdates_API(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/unxed/f4/releases/latest" {
			resp := githubRelease{
				TagName: "v9.9.9",
				Assets: []githubAsset{
					{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock/download"},
				},
			}
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("encode release response: %v", err)
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ts.Close()

	origAPIURL := githubAPIURL
	origOS := currentOS
	origArch := currentArch
	githubAPIURL = ts.URL + "/repos/unxed/f4/releases"
	currentOS = "linux"
	currentArch = "amd64"

	defer func() {
		githubAPIURL = origAPIURL
		currentOS = origOS
		currentArch = origArch
	}()

	AppConfig.UpdateChannel = 0
	AppConfig.UpdateInterval = 0

	CheckForUpdates(nil, true)

	foundDialog := false
	timeout := time.After(1 * time.Second)
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Auto Update " {
				foundDialog = true
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if !foundDialog {
		t.Error("Update dialog did not appear when a newer version was available")
	}
}

func TestUpdater_Extractors(t *testing.T) {
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
	err = extractZipToDir(zipBuf.Bytes(), destZip)
	if err != nil {
		t.Fatalf("extractZipToDir failed: %v", err)
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
	err = extractTarGzToDir(tgzBuf.Bytes(), destTar)
	if err != nil {
		t.Fatalf("extractTarGzToDir failed: %v", err)
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
	err = extract7zToDir(sevenData, dest7z)
	if err != nil {
		t.Fatalf("extract7zToDir failed: %v", err)
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

func TestSanitizeExtractPathRejectsPlatformSeparators(t *testing.T) {
	for _, name := range []string{`..\\outside`, `folder\\..\\outside`, "nul\x00name"} {
		if _, err := sanitizeExtractPath(name, t.TempDir()); err == nil {
			t.Errorf("sanitizeExtractPath(%q) accepted an unsafe archive name", name)
		}
	}
}

func TestUpdater_GetCurrentVersion(t *testing.T) {
	/*
		tests := []struct {
			input string
			want  string
		}{
			{"v0.1.1-alpha-a1b2c3d", "v0.1.1-alpha"},
			{"v1.0.0-beta", "v1.0.0-beta"}, // no hash
			{"v2.0.0", "v2.0.0"},
		}
	*/

	// We can't easily mock api.GetVersion() without interface refactoring,
	// but we can test the splitting logic directly if we just mock the logic.
	// Since getCurrentVersion internally calls api.GetVersion(), we can check
	// the actual returned value of the core.

	api := &coreAPI{}
	realVer := api.GetVersion()
	parts := strings.Split(realVer, "-")
	expected := realVer
	if len(parts) >= 3 {
		expected = strings.Join(parts[:len(parts)-1], "-")
	}

	if got := getCurrentVersion(); got != expected {
		t.Errorf("getCurrentVersion() = %q, want %q", got, expected)
	}
}

func TestUpdater_NetworkErrors(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Test 500 error
	ts500 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts500.Close()

	// Test bad JSON
	tsBadJSON := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`{bad json`)); err != nil {
			t.Errorf("write malformed response: %v", err)
		}
	}))
	defer tsBadJSON.Close()

	origAPIURL := githubAPIURL
	defer func() { githubAPIURL = origAPIURL }()

	// Test 1: 500
	githubAPIURL = ts500.URL
	CheckForUpdates(nil, true)

	timeout := time.After(2 * time.Second)
	foundError := false
Loop500:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Update Error " {
				foundError = true
				top.SetExitCode(-1)
				vtui.FrameManager.Pop()
				break Loop500
			}
		case <-timeout:
			break Loop500
		}
	}
	if !foundError {
		t.Error("Did not show error dialog for 500 status")
	}

	// Test 2: Bad JSON
	githubAPIURL = tsBadJSON.URL
	CheckForUpdates(nil, true)

	timeout = time.After(2 * time.Second)
	foundError = false
LoopJSON:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Update Error " {
				foundError = true
				top.SetExitCode(-1)
				vtui.FrameManager.Pop()
				break LoopJSON
			}
		case <-timeout:
			break LoopJSON
		}
	}
	if !foundError {
		t.Error("Did not show error dialog for bad JSON")
	}
}

// TestUpdater_UserDeclinesUpdate pins the fix for #374: declining an
// update must NOT persist across sessions. Concretely:
//   - AppConfig.LastUpdateVersion must stay untouched (that field is the
//     "we already installed this version" marker and would suppress the
//     prompt on every subsequent restart, which is the reported bug).
//   - sessionDismissedUpdateKey must be set, so a follow-up automatic
//     check within the same run does not re-prompt for the same version.
func TestUpdater_UserDeclinesUpdate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	oldDismissed := sessionDismissedUpdateKey
	defer func() { sessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := currentOS, currentArch
	currentOS, currentArch = "linux", "amd64"
	defer func() { currentOS, currentArch = origOS, origArch }()
	sessionDismissedUpdateKey = ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := githubRelease{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []githubAsset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := githubAPIURL
	githubAPIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { githubAPIURL = origAPIURL }()

	AppConfig.UpdateChannel = 0 // Stable
	AppConfig.LastUpdateVersion = ""

	CheckForUpdates(nil, true)

	timeout := time.After(2 * time.Second)
	dialogHandled := false
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Auto Update " {
				if dlg, ok := top.(*vtui.Window); ok && dlg.OnResult != nil {
					// Simulate clicking "No" (button index 1)
					dlg.OnResult(1)
					top.SetExitCode(-1)
					vtui.FrameManager.Pop()
					dialogHandled = true
					break Loop
				}
			}
		case <-timeout:
			break Loop
		}
	}

	if !dialogHandled {
		t.Fatal("Update prompt dialog not found")
	}

	if AppConfig.LastUpdateVersion != "" {
		t.Errorf("declining the prompt must NOT persist across restarts (see #374); LastUpdateVersion=%q, want empty",
			AppConfig.LastUpdateVersion)
	}
	if sessionDismissedUpdateKey != "v100.0.0" {
		t.Errorf("declining the prompt must arm the session-level dismiss; sessionDismissedUpdateKey=%q, want %q",
			sessionDismissedUpdateKey, "v100.0.0")
	}
}

// TestUpdater_ManualCheckIgnoresSessionDismiss guards the second half
// of #374: after the user declined once, an explicit "Check for
// updates" from the settings dialog must still offer the update.
// The session-level dismissal only silences the automatic prompt.
func TestUpdater_ManualCheckIgnoresSessionDismiss(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	oldDismissed := sessionDismissedUpdateKey
	defer func() { sessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := currentOS, currentArch
	currentOS, currentArch = "linux", "amd64"
	defer func() { currentOS, currentArch = origOS, origArch }()
	// Simulate the user having declined the same release earlier.
	sessionDismissedUpdateKey = "v100.0.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := githubRelease{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []githubAsset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := githubAPIURL
	githubAPIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { githubAPIURL = origAPIURL }()

	AppConfig.UpdateChannel = 0
	AppConfig.LastUpdateVersion = ""

	CheckForUpdates(nil, true) // manual == true

	timeout := time.After(2 * time.Second)
	sawPrompt := false
	for !sawPrompt {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Auto Update " {
				sawPrompt = true
			}
		case <-timeout:
			t.Fatal("manual check must re-offer the update even after a session-level dismiss (see #374)")
		}
	}
}

// TestUpdater_AutoCheckSkipsSessionDismiss is the other side of the
// same coin: within one session, an interval-driven automatic check
// must NOT re-prompt for a version the user already declined this
// run. This keeps the manual override useful without introducing the
// #374 spam.
func TestUpdater_AutoCheckSkipsSessionDismiss(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	oldDismissed := sessionDismissedUpdateKey
	defer func() { sessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := currentOS, currentArch
	currentOS, currentArch = "linux", "amd64"
	defer func() { currentOS, currentArch = origOS, origArch }()
	sessionDismissedUpdateKey = "v100.0.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := githubRelease{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []githubAsset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := githubAPIURL
	githubAPIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { githubAPIURL = origAPIURL }()

	AppConfig.UpdateChannel = 0
	AppConfig.LastUpdateVersion = ""
	// Force shouldCheck() to allow the auto path to reach the dismiss guard.
	AppConfig.UpdateInterval = 1
	AppConfig.LastUpdateCheck = 0

	CheckForUpdates(nil, false) // manual == false

	// Give the goroutine a chance to reach the guard and return without
	// pushing a task; a leaking prompt would enqueue one within ~200ms.
	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Auto Update " {
				t.Fatal("auto check must respect the session-level dismiss (see #374)")
			}
		case <-timeout:
			return
		}
	}
}

func TestUpdater_WriteFileSafe_FallbackOldName(t *testing.T) {
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

	err := writeFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err != nil {
		t.Fatalf("writeFileSafe failed with fallback: %v", err)
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
func TestUpdater_WriteFileSafe(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "binary.exe")

	// 1. Initial write
	err := writeFileSafe(targetPath, strings.NewReader("v1"), 0755)
	if err != nil {
		t.Fatalf("writeFileSafe failed: %v", err)
	}

	b, _ := os.ReadFile(targetPath)
	if string(b) != "v1" {
		t.Errorf("Expected 'v1', got %q", string(b))
	}

	// 2. Overwrite existing file
	err = writeFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err != nil {
		t.Fatalf("writeFileSafe overwrite failed: %v", err)
	}

	b, _ = os.ReadFile(targetPath)
	if string(b) != "v2" {
		t.Errorf("Expected 'v2', got %q", string(b))
	}
}

func TestUpdater_PerformUpdate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	mockExe := filepath.Join(tmpDir, "f4_mock_exe")
	if err := os.WriteFile(mockExe, []byte("old_binary"), 0600); err != nil {
		t.Fatal(err)
	}

	origExeFunc := osExecutable
	osExecutable = func() (string, error) {
		return mockExe, nil
	}
	defer func() { osExecutable = origExeFunc }()

	var tgzBuf bytes.Buffer
	gw := gzip.NewWriter(&tgzBuf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: filepath.Base(mockExe), Size: int64(len("new_binary")), Mode: 0755}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("new_binary")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write(tgzBuf.Bytes()); err != nil {
			t.Errorf("write update archive: %v", err)
		}
	}))
	defer ts.Close()

	pf := NewPanelsFrame()
	defer pf.Close()

	performUpdate(pf, ts.URL, "targz", "v9.9.9", "2026-01-01")

	timeout := time.After(3 * time.Second)
	successDialogFound := false
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && top.GetTitle() == " Update Successful " {
				successDialogFound = true
				break Loop
			}
		case <-timeout:
			break Loop
		}
	}

	if !successDialogFound {
		t.Fatal("Success dialog never appeared. Update process likely failed.")
	}

	content, err := os.ReadFile(mockExe)
	if err != nil {
		t.Fatalf("Failed to read replaced executable: %v", err)
	}

	if string(content) != "new_binary" {
		t.Errorf("Executable replacement failed. Got %q, want 'new_binary'", string(content))
	}
}

func TestUpdater_WriteFileSafe_SudoElevationFallback(t *testing.T) {
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
	err = writeFileSafe(targetPath, strings.NewReader("v2"), 0755)
	if err == nil {
		t.Error("expected writeFileSafe to fail under restricted directory")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "elevated dispatcher") && !strings.Contains(errStr, "sudo process") {
		t.Errorf("expected error to originate from sudo elevation fallback, got: %q", errStr)
	}
}
