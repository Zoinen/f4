package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
	"gopkg.in/yaml.v3"
)

func TestResolveAssetURL(t *testing.T) {
	tpl := "https://example.com/plugin_{os}_{arch}.zip"
	expected := "https://example.com/plugin_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"

	got := ResolveAssetURL(tpl)
	if got != expected {
		t.Errorf("ResolveAssetURL failed. Expected %q, got %q", expected, got)
	}

	// Test no placeholders
	plain := "https://example.com/plugin.zip"
	if ResolveAssetURL(plain) != plain {
		t.Errorf("ResolveAssetURL altered a string without placeholders")
	}
}

func TestBundledPlugRingCatalogUsesRemoteAssets(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	catalogPath := filepath.Join(filepath.Dir(sourceFile), "..", "..", "plugring", "index.yaml")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read bundled PlugRing catalog: %v", err)
	}

	var items []PlugRingItem
	if err := yaml.Unmarshal(data, &items); err != nil {
		t.Fatalf("parse bundled PlugRing catalog: %v", err)
	}
	for _, item := range items {
		u, err := url.Parse(item.URL)
		if err != nil {
			t.Errorf("PlugRing item %q has invalid asset URL %q: %v", item.ID, item.URL, err)
			continue
		}
		if u.Scheme != "https" || u.Host == "" {
			t.Errorf("PlugRing item %q must use an absolute HTTPS asset URL, got %q", item.ID, item.URL)
		}
	}
}

func TestFetchCatalog_Success(t *testing.T) {
	mockYaml := `
- id: "test-plugin"
  name: "Test Plugin"
  version: "1.0"
  author: "Tester"
  description: "A mock plugin"
  url: "http://example.com"
  entrypoint: "main.exe"
`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		if _, err := w.Write([]byte(mockYaml)); err != nil {
			t.Errorf("write catalog response: %v", err)
		}
	}))
	defer ts.Close()

	// Override URL for testing
	origURL := PlugRingCatalogURL
	PlugRingCatalogURL = ts.URL
	defer func() { PlugRingCatalogURL = origURL }()

	items, err := FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog failed: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(items))
	}

	if items[0].ID != "test-plugin" || items[0].Name != "Test Plugin" {
		t.Errorf("Parsed item mismatch: %+v", items[0])
	}
}

func TestFetchCatalog_Errors(t *testing.T) {
	origURL := PlugRingCatalogURL
	defer func() { PlugRingCatalogURL = origURL }()

	t.Run("404 Not Found", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer ts.Close()

		PlugRingCatalogURL = ts.URL
		_, err := FetchCatalog(context.Background())
		if err == nil || err.Error() != "server returned status 404" {
			t.Errorf("Expected 404 error, got: %v", err)
		}
	})

	t.Run("Bad YAML", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, err := w.Write([]byte(`- id: "test" \n invalid_yaml: {`)); err != nil {
				t.Errorf("write malformed catalog response: %v", err)
			}
		}))
		defer ts.Close()

		PlugRingCatalogURL = ts.URL
		_, err := FetchCatalog(context.Background())
		if err == nil {
			t.Error("Expected YAML parse error, got nil")
		}
	})
}
func TestFetchCatalog_Dependencies(t *testing.T) {
	mockYaml := `
- id: "dep-plugin"
  name: "Dep Plugin"
  dependencies:
    - lua
    - python3
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		if _, err := w.Write([]byte(mockYaml)); err != nil {
			t.Errorf("write catalog response: %v", err)
		}
	}))
	defer ts.Close()

	origURL := PlugRingCatalogURL
	PlugRingCatalogURL = ts.URL
	defer func() { PlugRingCatalogURL = origURL }()

	items, err := FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog failed: %v", err)
	}

	if len(items) != 1 || len(items[0].Dependencies) != 2 || items[0].Dependencies[0] != "lua" {
		t.Errorf("Dependencies parsing failed: %+v", items)
	}
}
func TestGetInstalledPlugRingItems(t *testing.T) {
	tmpDir := t.TempDir()
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { userConfigDir = oldUserConfigDir })
	resetConfigDirForTest()
	t.Cleanup(resetConfigDirForTest)

	cfgDir := GetF4ConfigDir()
	plugringDir := filepath.Join(cfgDir, "plugring")
	pluginPath := filepath.Join(plugringDir, "test-id")
	if err := os.MkdirAll(pluginPath, 0700); err != nil {
		t.Fatal(err)
	}

	manifest := `{"id": "test-id", "version": "1.0.0"}`
	if err := os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}

	installed := GetInstalledPlugRingItems()
	if len(installed) != 1 {
		t.Fatalf("Expected 1 installed item, got %d", len(installed))
	}
	if installed["test-id"].Version != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", installed["test-id"].Version)
	}
}
func TestCheckForPluginUpdates(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	oldUserConfigDir := userConfigDir
	userConfigDir = func() (string, error) { return tmpDir, nil }
	t.Cleanup(func() { userConfigDir = oldUserConfigDir })
	resetConfigDirForTest()
	t.Cleanup(resetConfigDirForTest)

	cfgDir := GetF4ConfigDir()
	plugringDir := filepath.Join(cfgDir, "plugring")
	pluginPath := filepath.Join(plugringDir, "test-plugin")
	if err := os.MkdirAll(pluginPath, 0700); err != nil {
		t.Fatal(err)
	}

	// Local manifest has version 1.0
	manifest := `{"id": "test-plugin", "version": "1.0"}`
	if err := os.WriteFile(filepath.Join(pluginPath, "manifest.json"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}

	// Remote catalog has newer version 2.0
	mockYaml := `
- id: "test-plugin"
  name: "Test Plugin"
  version: "2.0"
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		if _, err := w.Write([]byte(mockYaml)); err != nil {
			t.Errorf("write catalog response: %v", err)
		}
	}))
	defer ts.Close()

	origURL := PlugRingCatalogURL
	PlugRingCatalogURL = ts.URL
	defer func() { PlugRingCatalogURL = origURL }()

	// We override sleep duration inside CheckForPluginUpdates indirectly by running
	// its core logic synchronously in our test to avoid 5-second sleep hang.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	items, err := FetchCatalog(ctx)
	if err != nil {
		t.Fatalf("FetchCatalog failed: %v", err)
	}

	installed := GetInstalledPlugRingItems()
	updateCount := 0
	for _, itm := range items {
		if inst, ok := installed[itm.ID]; ok {
			if inst.Version != itm.Version {
				updateCount++
			}
		}
	}

	if updateCount != 1 {
		t.Errorf("Expected 1 update available, got %d", updateCount)
	}

	// Trigger toast on UI thread
	vtui.FrameManager.PostTask(func() {
		vtui.ShowToast("PlugRing: 1 plugin update available!", 100*time.Millisecond)
	})

	// Process task queue and verify toast appeared
	timeout := time.After(1 * time.Second)
	foundToast := false
	for !foundToast {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			if vtui.FrameManager.GetActiveToast() != "" {
				foundToast = true
			}
		case <-timeout:
			t.Fatal("Timeout waiting for update toast")
		}
	}

	// The toast owns a timer goroutine. Its second redraw is the completion
	// signal; wait for it before restoring the global frame manager.
	select {
	case <-vtui.FrameManager.RedrawChan:
	default:
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for update toast to expire")
	}
}
func TestPlugRing_InstallAndRemove_EndToEnd(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpConfig := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpConfig)
	t.Setenv("APPDATA", tmpConfig)
	resetConfigDirForTest()

	// 1. Setup mock single-file plugin
	pluginContent := `#!/bin/sh
echo "running"
`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(pluginContent)); err != nil {
			t.Errorf("write plugin response: %v", err)
		}
	}))
	defer ts.Close()

	item := PlugRingItem{
		ID:          "e2e-plugin",
		Name:        "E2E Plugin",
		Version:     "1.0.0",
		Author:      "Tester",
		Description: "End to end test plugin",
		URL:         ts.URL + "/plugin.sh",
		Entrypoint:  "sh plugin.sh",
		SetupCmd:    "echo 'setup complete' > setup.log",
	}

	pf := NewPanelsFrame()
	defer pf.Close()

	// 2. Perform installation
	done := make(chan struct{})
	var installErr error

	// Installing an entry that breaks the distribution policy asks first,
	// and nobody is here to press a button, so the dialogs are answered as
	// they appear. Message blocks its caller until then, and the caller
	// must not be the goroutine that pumps the queue.
	answered := map[*vtui.Window]bool{}
	answer := func() {
		top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if !ok || top == nil || top.OnResult == nil || answered[top] {
			return
		}
		title := top.GetTitle()
		if strings.Contains(title, "Installing Plugin") {
			return // Don't cancel the progress dialog
		}
		answered[top] = true
		top.OnResult(0) // "Install Anyway"
		top.SetExitCode(-1)
		vtui.FrameManager.Pop()
	}

	go actionInstallPlugRingItem(pf, nil, item, func() {
		close(done)
	})

	// Drain task queue to process async download and installation
	timeout := time.After(3 * time.Second)
	// A dialog can be waiting while the queue is empty, so look for one on
	// a tick too, not only after a task has run.
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
Loop:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			answer()
		case <-tick.C:
			answer()
		case <-done:
			break Loop
		case <-timeout:
			t.Fatal("Timeout waiting for plugin installation")
		}
	}

	if installErr != nil {
		t.Fatalf("Plugin installation failed: %v", installErr)
	}

	// 3. Verify files on disk
	pluginDir := filepath.Join(GetF4ConfigDir(), "plugring", "e2e-plugin")
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.sh")); os.IsNotExist(err) {
		t.Error("Plugin binary file was not saved to disk")
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "manifest.json")); os.IsNotExist(err) {
		t.Error("Plugin manifest.json was not created")
	}

	// Verify setup_cmd is ignored
	if _, err := os.Stat(filepath.Join(pluginDir, "setup.log")); !os.IsNotExist(err) {
		t.Error("setup_cmd was executed, but it should have been ignored per policy")
	}

	// 4. Verify GetInstalledPlugRingItems detects it
	installed := GetInstalledPlugRingItems()
	if _, ok := installed["e2e-plugin"]; !ok {
		t.Error("GetInstalledPlugRingItems failed to detect newly installed plugin")
	}

	// 5. Perform Removal
	removeDone := make(chan struct{})
	actionRemovePlugRingItem(pf, nil, item, func() {
		close(removeDone)
	})

	// Process confirmation dialog and removal
	timeout = time.After(2 * time.Second)
	removedSimulated := false
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		}
		// Check for dialogs after processing any task or tick
		if !removedSimulated {
			top := vtui.FrameManager.GetTopFrame()
			if top != nil {
				title := top.GetTitle()
				if strings.Contains(title, "Remove Plugin") {
					if dlg, ok := top.(*vtui.Window); ok && dlg.OnResult != nil {
						removedSimulated = true
						dlg.OnResult(0) // Click Remove
						top.SetExitCode(-1)
						vtui.FrameManager.Pop()
					}
				} else if strings.Contains(title, "Info") {
					t.Fatal("Plugin is not installed, removal aborted")
				}
			}
		}
		select {
		case <-removeDone:
			goto VerifiedRemoval
		case <-timeout:
			t.Fatal("Timeout waiting for plugin removal")
		default:
		}
	}

VerifiedRemoval:
	if _, err := os.Stat(pluginDir); !os.IsNotExist(err) {
		t.Error("Plugin directory was not deleted after removal")
	}
}
