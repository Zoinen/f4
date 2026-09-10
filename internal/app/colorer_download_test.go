package app

import (
	"archive/zip"
	"bytes"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

// The colorer downloader takes a vfs.App for its progress task, and the panels
// frame is the only one there is. The rest of the colorer tests moved to
// internal/editor with their subject; this one stayed with the frame.

func TestColorer_DownloadColorerSchemas(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("far2l-v_2.8.0/colorer/configs/base/catalog.xml")
	if err != nil {
		t.Fatalf("Failed to create zip entry: %v", err)
	}
	if _, err := f.Write([]byte("<catalog></catalog>")); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	// Preserve an initialized config cache when this test temporarily swaps
	// the directory below. Otherwise cleanup can restore an empty cache while
	// config.ConfigDirOnce remains consumed, making later shuffled tests resolve
	// relative paths.
	_ = config.GetF4ConfigDir()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		if _, err := w.Write(buf.Bytes()); err != nil {
			t.Errorf("write schema archive response: %v", err)
		}
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	oldConfigDirFunc := config.GetUserConfigIniPath
	config.GetUserConfigIniPath = func() string { return filepath.Join(tmpDir, "settings.ini") }
	origPathsFunc := config.GetConfigIniPaths
	config.GetConfigIniPaths = func() []string { return []string{filepath.Join(tmpDir, "settings.ini")} }
	oldConfigDir := config.CachedF4ConfigDir
	config.CachedF4ConfigDir = tmpDir

	defer func() {
		config.GetUserConfigIniPath = oldConfigDirFunc
		config.GetConfigIniPaths = origPathsFunc
		config.CachedF4ConfigDir = oldConfigDir
	}()

	oldURL := editor.ColorerDownloadURL
	editor.ColorerDownloadURL = ts.URL
	defer func() { editor.ColorerDownloadURL = oldURL }()

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	done := make(chan bool)
	editor.DownloadColorerSchemas(pf, func(success bool) {
		if !success {
			t.Error("Expected successful download and extraction")
		}
		close(done)
	})

	timeout := time.After(3 * time.Second)
Loop:
	for {
		select {
		case <-done:
			break Loop
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for downloader")
		}
	}

	if !editor.SchemasExist() {
		t.Error("SchemasExist returned false after successful extraction")
	}
}
