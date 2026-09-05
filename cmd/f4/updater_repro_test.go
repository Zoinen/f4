package main

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

// TestUpdateFailureMessageRepro simulates the user scenario where the f4 binary is locked
// by another process during an update.
func TestUpdateFailureMessageRepro(t *testing.T) {
	// 1. Setup a dummy "executable" in a temp directory
	tmpDir := t.TempDir()

	exeName := "f4"
	if runtime.GOOS == "windows" {
		exeName = "f4.exe"
	}
	exePath := filepath.Join(tmpDir, exeName)
	if err := os.WriteFile(exePath, []byte("original binary content"), 0755); err != nil { // #nosec G306 -- the updater fixture represents an executable binary.
		t.Fatal(err)
	}

	// 2. Lock the file
	// On Windows, opening with O_RDWR and not sharing delete/write access usually locks it.
	// To make this test cross-platform reliable for our purpose (triggering an error in writeFileSafe),
	// we'll either lock it or make the dir non-writable.
	if runtime.GOOS == "windows" {
		// os.OpenFile always shares read/write/delete, which does not stop
		// the updater's rename-aside at all; only a no-sharing CreateFile
		// actually blocks the install.
		unlock, err := lockFileExclusively(exePath)
		if err != nil {
			t.Fatal("Failed to create a lock on the file:", err)
		}
		defer unlock()
	} else {
		// On Unix, removing write permission from the directory prevents renaming/creating files.
		// We also make the file itself read-only, because if rename fails, writeFileSafe
		// will fall back to truncating the existing file, which would otherwise succeed
		// if the file itself is writable.
		if err := os.Chmod(exePath, 0444); err != nil { // #nosec G302 -- the read-only executable is required to exercise update failure.
			t.Fatal(err)
		}
		if err := os.Chmod(tmpDir, 0555); err != nil { // #nosec G302 -- the non-writable directory is required to exercise update failure.
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(tmpDir, 0755); err != nil { // #nosec G302 -- cleanup must restore access to the deliberately locked directory.
				t.Errorf("restore temporary directory permissions: %v", err)
			}
			if err := os.Chmod(exePath, 0755); err != nil { // #nosec G302 -- cleanup restores the fixture's executable mode.
				t.Errorf("restore executable permissions: %v", err)
			}
		})
	}

	// 3. Setup a mock update server
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	f, err := zw.Create(exeName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new binary content")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		if _, err := w.Write(zipBuf.Bytes()); err != nil {
			t.Errorf("write update response: %v", err)
		}
	}))
	defer ts.Close()

	// 4. Mock globals
	oldExe := osExecutable
	osExecutable = func() (string, error) { return exePath, nil }
	defer func() { osExecutable = oldExe }()

	// 5. Initialize headless UI environment
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	// 6. Run update logic
	// We call performUpdate directly as it's the one handling the installation.
	// Since performUpdate is internal and runs in a goroutine via RunProgressTask,
	// we need a PanelsFrame to host it.
	pf := NewPanelsFrame()
	pf.ResizeConsole(80, 25)

	performUpdate(pf, ts.URL, "zip", "v9.9.9", "2026-08-05T12:00:00Z")

	// 7. Wait for the background task to hit the error and show the dialog by pumping TaskChan
	var capturedError string
	// A download, an extract and a failed spawn all happen before the
	// dialog shows; loaded CI runners need far more headroom than 5s.
	timeout := time.After(30 * time.Second)
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			top := vtui.FrameManager.GetTopFrame()
			if top != nil && strings.Contains(top.GetTitle(), "Update Failed") {
				if win, ok := top.(*vtui.Window); ok {
					capturedError = collectUIText(win.GetChildren())
					top.SetExitCode(-1)
					vtui.FrameManager.Pop()
					break Loop
				}
			}
		case <-timeout:
			break Loop
		}
	}

	if capturedError == "" {
		t.Fatal("Update did not fail as expected, or error dialog was not found.")
	}

	// 8. Verify the presence of the new advice
	expectedAdvice := "check Task Manager for ghost f4 processes"
	if !strings.Contains(capturedError, expectedAdvice) {
		t.Errorf("Error message did not contain expected advice.\nGot: %s\nWant to contain: %s", capturedError, expectedAdvice)
	}

	t.Logf("Captured expected error message: %s", capturedError)
}
func collectUIText(elements []vtui.UIElement) string {
	var res string
	for _, el := range elements {
		if txt, ok := el.(*vtui.Text); ok {
			res += txt.GetText() + " "
		}
		if container, ok := el.(vtui.Container); ok {
			res += collectUIText(container.GetChildren())
		}
	}
	return res
}
