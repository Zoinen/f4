package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func TestIssue635NetworkDropWhileProgressScreenIsBackground(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldTimeout := updateDownloadIdleTimeout
	updateDownloadIdleTimeout = 50 * time.Millisecond
	defer func() { updateDownloadIdleTimeout = oldTimeout }()
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "f4")
	if err := os.WriteFile(exePath, []byte("old"), 0755); err != nil { // #nosec G306 -- the updater fixture represents an executable binary.
		t.Fatal(err)
	}
	oldExe := osExecutable
	osExecutable = func() (string, error) { return exePath, nil }
	defer func() { osExecutable = oldExe }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer ts.Close()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	performUpdate(pf, ts.URL, "zip", "v9.9.9", "2026-08-22")

	backgrounded := false
	deadline := time.After(5 * time.Second)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if !backgrounded && len(vtui.FrameManager.Screens) > 1 {
				vtui.FrameManager.SwitchScreen(0)
				backgrounded = true
			}
			top := vtui.FrameManager.GetTopFrame()
			if backgrounded && top != nil && top.GetTitle() == " Update Failed " {
				return
			}
		case <-deadline:
			t.Fatalf("update failure message was not shown after backgrounding progress screen; backgrounded=%v active=%d screens=%d", backgrounded, vtui.FrameManager.ActiveIdx, len(vtui.FrameManager.Screens))
		}
	}
}
