package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveSettingsGroupsKeepUnselectedValues(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "settings.ini")
	sessionPath := filepath.Join(tmpDir, "session.ini")

	oldConfig := config.App
	oldUserPath := config.GetUserConfigIniPath
	oldConfigPaths := config.GetConfigIniPaths
	oldSessionPath := GetSessionIniPath
	t.Cleanup(paneltest.SwapFrameManager(t))
	defer func() {
		config.App = oldConfig
		config.GetUserConfigIniPath = oldUserPath
		config.GetConfigIniPaths = oldConfigPaths
		GetSessionIniPath = oldSessionPath
	}()

	config.GetUserConfigIniPath = func() string { return settingsPath }
	config.GetConfigIniPaths = func() []string { return []string{settingsPath} }
	GetSessionIniPath = func() string { return sessionPath }

	config.App.ColorStyle = "Persisted"
	config.App.GuiCols = 80
	config.App.GuiRows = 25
	config.SaveConfig()

	config.App.ColorStyle = "Pending"
	config.App.GuiCols = 120
	config.App.GuiRows = 40
	saveSettingsGroups(true, false, false)
	saved := ini.Load(settingsPath)
	if got := saved.GetString("Interface", "ColorStyle", ""); got != "Pending" {
		t.Fatalf("general settings were not saved: ColorStyle = %q", got)
	}
	if got := saved.GetString("Appearance", "GuiCols", ""); got != "80" {
		t.Fatalf("unselected GUI width changed: %q", got)
	}
	if got := saved.GetString("Appearance", "GuiRows", ""); got != "25" {
		t.Fatalf("unselected GUI height changed: %q", got)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("\n[ThirdParty]\nKeep=1\n")...)
	if err := os.WriteFile(settingsPath, data, 0600); err != nil { // #nosec G703 -- settingsPath is a fixed fixture name beneath t.TempDir().
		t.Fatal(err)
	}
	config.App.ColorStyle = "Not persisted"
	config.App.GuiCols = 140
	config.App.GuiRows = 50
	saveSettingsGroups(false, false, true)
	saved = ini.Load(settingsPath)
	if got := saved.GetString("Interface", "ColorStyle", ""); got != "Pending" {
		t.Fatalf("window-only save changed general settings: %q", got)
	}
	if got := saved.GetString("Appearance", "GuiCols", ""); got != "140" {
		t.Fatalf("window-only save did not save width: %q", got)
	}
	if got := saved.GetString("Appearance", "GuiRows", ""); got != "50" {
		t.Fatalf("window-only save did not save height: %q", got)
	}
	if got := saved.GetString("ThirdParty", "Keep", ""); got != "1" {
		t.Fatalf("window-only save discarded unknown settings: %q", got)
	}

	freshSettingsPath := filepath.Join(tmpDir, "fresh-settings.ini")
	config.GetUserConfigIniPath = func() string { return freshSettingsPath }
	config.GetConfigIniPaths = func() []string { return []string{freshSettingsPath} }
	config.App.GuiCols = 90
	config.App.GuiRows = 30
	saveSettingsGroups(false, false, true)
	fresh, err := os.ReadFile(freshSettingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fresh), "[Interface]") {
		t.Fatalf("window-only save created unrelated settings on a fresh profile:\n%s", fresh)
	}
}

func TestSaveSessionDisabled(t *testing.T) {
	oldConfig := config.App
	oldSessionPath := GetSessionIniPath
	defer func() {
		config.App = oldConfig
		GetSessionIniPath = oldSessionPath
	}()

	path := filepath.Join(t.TempDir(), "session.ini")
	GetSessionIniPath = func() string { return path }
	config.App.AutoSaveSettings = false
	SaveSession()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("disabled automatic session save created %s, err=%v", path, err)
	}
}

// TestRequestSaveConfigPostsToTheFrameManagerItWasArmedWith is the regression
// for the second race of this shape the race job reported.
//
// The debounced save reads vtui.FrameManager half a second after it is armed,
// from the timer's own goroutine. Nothing keeps a test alive that long, so the
// read lands in whichever test is running by then, and the write it races is
// that test's paneltest.SwapFrameManager -- which is why the report names a test that has
// nothing to do with saving settings.
func TestRequestSaveConfigPostsToTheFrameManagerItWasArmedWith(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	arming := vtui.FrameManager

	oldConfig := config.App
	t.Cleanup(func() { config.App = oldConfig })
	config.App.AutoSaveSettings = true
	config.App.AutoSaveDialogSettings = true

	config.RequestSaveConfig()
	t.Cleanup(func() {
		config.SaveConfigTimerMu.Lock()
		if config.SaveConfigTimer != nil {
			config.SaveConfigTimer.Stop()
		}
		config.SaveConfigTimerMu.Unlock()
	})

	// What the next test does while the timer is still counting down.
	replacement := vtui.NewFrameManager()
	vtui.FrameManager = replacement
	t.Cleanup(func() {
		testutil.CloseFrameManagerFrames(replacement)
		replacement.Shutdown()
		vtui.FrameManager = arming
	})

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	select {
	case task := <-arming.TaskChan:
		// Not run: config.SaveConfig would write the real configuration file. That it
		// arrived here rather than on the replacement is the whole point.
		_ = task
	case <-replacement.TaskChan:
		t.Fatal("the debounced save posted to the frame manager that replaced it")
	case <-deadline.C:
		t.Fatal("the debounced save never reached the frame manager it was armed with")
	}
}
