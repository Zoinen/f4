package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/update"
	"github.com/unxed/vtui"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdater_ShouldCheck(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()

	now := time.Now().Unix()

	config.App.UpdateInterval = 0
	if ShouldCheck() {
		t.Error("Should not check if interval is 0")
	}

	config.App.UpdateInterval = 1
	config.App.LastUpdateCheck = now
	if !ShouldCheck() {
		t.Error("Should check every start")
	}

	config.App.UpdateInterval = 2
	config.App.LastUpdateCheck = now
	if ShouldCheck() {
		t.Error("Should not check daily if just checked")
	}
	config.App.LastUpdateCheck = now - 25*3600
	if !ShouldCheck() {
		t.Error("Should check daily if > 24h passed")
	}

	config.App.UpdateInterval = 3
	config.App.LastUpdateCheck = now - 2*24*3600
	if ShouldCheck() {
		t.Error("Should not check weekly if < 7 days passed")
	}
	config.App.LastUpdateCheck = now - 8*24*3600
	if !ShouldCheck() {
		t.Error("Should check weekly if > 7 days passed")
	}
}

func TestUpdater_CheckForUpdates_API(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := config.App
	defer func() { config.App = oldCfg }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/unxed/f4/releases/latest" {
			resp := update.Release{
				TagName: "v9.9.9",
				Assets: []update.Asset{
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

	origAPIURL := update.APIURL
	origOS := update.CurrentOS
	origArch := update.CurrentArch
	update.APIURL = ts.URL + "/repos/unxed/f4/releases"
	update.CurrentOS = "linux"
	update.CurrentArch = "amd64"

	defer func() {
		update.APIURL = origAPIURL
		update.CurrentOS = origOS
		update.CurrentArch = origArch
	}()

	config.App.UpdateChannel = 0
	config.App.UpdateInterval = 0

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

	api := &CoreAPI{}
	realVer := api.GetVersion()
	parts := strings.Split(realVer, "-")
	expected := realVer
	if len(parts) >= 3 {
		expected = strings.Join(parts[:len(parts)-1], "-")
	}

	if got := GetCurrentVersion(); got != expected {
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

	origAPIURL := update.APIURL
	defer func() { update.APIURL = origAPIURL }()

	// Test 1: 500
	update.APIURL = ts500.URL
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
	update.APIURL = tsBadJSON.URL
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
//   - config.App.LastUpdateVersion must stay untouched (that field is the
//     "we already installed this version" marker and would suppress the
//     prompt on every subsequent restart, which is the reported bug).
//   - sessionDismissedUpdateKey must be set, so a follow-up automatic
//     check within the same run does not re-prompt for the same version.
func TestUpdater_UserDeclinesUpdate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	oldDismissed := SessionDismissedUpdateKey
	defer func() { SessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := update.CurrentOS, update.CurrentArch
	update.CurrentOS, update.CurrentArch = "linux", "amd64"
	defer func() { update.CurrentOS, update.CurrentArch = origOS, origArch }()
	SessionDismissedUpdateKey = ""

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := update.Release{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []update.Asset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := update.APIURL
	update.APIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { update.APIURL = origAPIURL }()

	config.App.UpdateChannel = 0 // Stable
	config.App.LastUpdateVersion = ""

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

	if config.App.LastUpdateVersion != "" {
		t.Errorf("declining the prompt must NOT persist across restarts (see #374); LastUpdateVersion=%q, want empty",
			config.App.LastUpdateVersion)
	}
	if SessionDismissedUpdateKey != "v100.0.0" {
		t.Errorf("declining the prompt must arm the session-level dismiss; sessionDismissedUpdateKey=%q, want %q",
			SessionDismissedUpdateKey, "v100.0.0")
	}
}

// TestUpdater_ManualCheckIgnoresSessionDismiss guards the second half
// of #374: after the user declined once, an explicit "Check for
// updates" from the settings dialog must still offer the update.
// The session-level dismissal only silences the automatic prompt.
func TestUpdater_ManualCheckIgnoresSessionDismiss(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	oldDismissed := SessionDismissedUpdateKey
	defer func() { SessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := update.CurrentOS, update.CurrentArch
	update.CurrentOS, update.CurrentArch = "linux", "amd64"
	defer func() { update.CurrentOS, update.CurrentArch = origOS, origArch }()
	// Simulate the user having declined the same release earlier.
	SessionDismissedUpdateKey = "v100.0.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := update.Release{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []update.Asset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := update.APIURL
	update.APIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { update.APIURL = origAPIURL }()

	config.App.UpdateChannel = 0
	config.App.LastUpdateVersion = ""

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
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	oldDismissed := SessionDismissedUpdateKey
	defer func() { SessionDismissedUpdateKey = oldDismissed }()
	// The stub release only carries linux/windows assets; without pinning
	// the platform the updater finds nothing on darwin and never prompts.
	origOS, origArch := update.CurrentOS, update.CurrentArch
	update.CurrentOS, update.CurrentArch = "linux", "amd64"
	defer func() { update.CurrentOS, update.CurrentArch = origOS, origArch }()
	SessionDismissedUpdateKey = "v100.0.0"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := update.Release{
			TagName:     "v100.0.0",
			PublishedAt: "2030-01-01T00:00:00Z",
			Assets: []update.Asset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "http://mock"},
				{Name: "f4-windows-amd64.zip", BrowserDownloadURL: "http://mock"},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode release response: %v", err)
		}
	}))
	defer ts.Close()

	origAPIURL := update.APIURL
	update.APIURL = ts.URL + "/repos/unxed/f4/releases"
	defer func() { update.APIURL = origAPIURL }()

	config.App.UpdateChannel = 0
	config.App.LastUpdateVersion = ""
	// Force shouldCheck() to allow the auto path to reach the dismiss guard.
	config.App.UpdateInterval = 1
	config.App.LastUpdateCheck = 0

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

func TestUpdater_PerformUpdate(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	mockExe := filepath.Join(tmpDir, "f4_mock_exe")
	if err := os.WriteFile(mockExe, []byte("old_binary"), 0600); err != nil {
		t.Fatal(err)
	}

	origExeFunc := update.Executable
	update.Executable = func() (string, error) {
		return mockExe, nil
	}
	defer func() { update.Executable = origExeFunc }()

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

	pf := panel.NewPanelsFrame()
	defer pf.Close()

	PerformUpdate(pf, update.Candidate{
		DownloadURL: ts.URL,
		ArchiveKind: "targz",
		UpdateKey:   "v9.9.9",
		NeedsUpdate: true,
	})

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
