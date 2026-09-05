package main

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/unxed/f4/fusefs"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// pressKey dispatches a key through the production input path: the
// macro/hotkey filter first (action hotkeys are dispatched there), then
// the frame's own ProcessKey for widget-level keys. It ensures the
// global managers exist and the frame is the top frame.
func pressKey(f vtui.Frame, e *vtinput.InputEvent) bool {
	if vtui.FrameManager == nil || len(vtui.FrameManager.Screens) == 0 {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	}
	if GlobalHotkeysMgr == nil {
		GlobalHotkeysMgr = NewHotkeyManager("")
	}
	if MacroMgr == nil {
		MacroMgr = NewMacroManager("")
	}
	inStack := false
	for _, s := range vtui.FrameManager.Screens {
		for _, fr := range s.Frames {
			if fr == f {
				inStack = true
				break
			}
		}
	}
	if !inStack {
		vtui.FrameManager.Push(f)
	}
	if MacroMgr.Filter(e) {
		return true
	}
	return f.ProcessKey(e)
}

// preserveActionRegistry keeps tests that register synthetic actions from
// leaking them into later tests or the next -count iteration.
func preserveActionRegistry(t *testing.T) {
	t.Helper()
	oldRegistry := actionRegistry
	oldOrder := actionOrder
	actionRegistry = make(map[string]Action, len(oldRegistry))
	for key, action := range oldRegistry {
		actionRegistry[key] = action
	}
	actionOrder = append([]string(nil), oldOrder...)
	t.Cleanup(func() {
		actionRegistry = oldRegistry
		actionOrder = oldOrder
	})
}

func checkedMouseCoordinate(t *testing.T, value int) int16 {
	t.Helper()
	if value < -32768 || value > 32767 {
		t.Fatalf("mouse coordinate %d does not fit in int16", value)
	}
	return int16(value) // #nosec G115 -- range checked immediately above.
}

func TestMain(m *testing.M) {
	setExtUiPanelCatalogMetadataEnabled(true)
	newLocalPTY = func() (PtyBackend, error) {
		return nil, errors.New("local PTY disabled for unit-test panel frames")
	}
	baseFrameManager := vtui.FrameManager
	baseFrameManager.Init(vtui.NewSilentScreenBuf())
	vfs.InitSudoClient("/usr/bin/f4", "")

	// Unit tests must never hand control to the user's desktop. Individual
	// tests that exercise these routes install per-dialog/per-frame recorders.
	defaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	defaultNativePropertiesOpener = func(string) error { return nil }

	// Frames must not fork the user's shell during unit tests; the few
	// tests that exercise the PTY path construct one explicitly.
	spawnLocalShellPTY = false

	// Toast behavior is still exercised through vtui's real asynchronous
	// setup and expiry paths, but unit tests do not need production-length
	// display times. Keep a small observable window: tests may observe another
	// effect of the same UI task (for example, clipboard contents) before they
	// pump the nested toast task, and a 1 ms toast can expire in that gap.
	toastDurationOverride = func(time.Duration) time.Duration {
		const minimumObservableToastDuration = 100 * time.Millisecond
		return minimumObservableToastDuration
	}

	// The machine's clipboard is global, slow to reach (pbcopy/xclip) and
	// shared with whatever else the CI runner is doing; tests keep clipboard
	// traffic in vtui's process-local buffer instead, and skip the OSC 52
	// stdout fallback that used to spray base64 into the test logs. A test
	// that genuinely targets the OS clipboard switches the knob back off
	// for its own scope.
	vtui.SkipOSClipboard(true)
	vtui.DisableTerminalClipboard()
	queueShowToast = func(string, time.Duration) {}

	tmpDir, err := os.MkdirTemp("", "f4-test-config-*")
	if err == nil {
		// XDG_CONFIG_HOME/APPDATA cover Linux and Windows; os.UserConfigDir
		// ignores both on darwin, so the seam is what actually isolates the
		// suite from the developer's real profile there.
		if setErr := os.Setenv("XDG_CONFIG_HOME", tmpDir); setErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "set XDG_CONFIG_HOME: %v\n", setErr)
			_ = os.RemoveAll(tmpDir) // Process exit makes cleanup failure uninteresting.
			os.Exit(1)
		}
		if setErr := os.Setenv("APPDATA", tmpDir); setErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "set APPDATA: %v\n", setErr)
			_ = os.RemoveAll(tmpDir) // Process exit makes cleanup failure uninteresting.
			os.Exit(1)
		}
		userConfigDir = func() (string, error) { return tmpDir, nil }
		resetConfigDirForTest()
	}

	result := m.Run()

	for _, unmountErr := range fusefs.UnmountAll() {
		_, _ = fmt.Fprintf(os.Stderr, "unmount test FUSE filesystem: %v\n", unmountErr)
		result = 1
	}

	globalFrameManager := vtui.FrameManager
	if globalFrameManager != nil {
		closeFrameManagerFrames(globalFrameManager)
		globalFrameManager.Shutdown()
	}
	if baseFrameManager != globalFrameManager {
		_, _ = fmt.Fprintln(os.Stderr, "vtui.FrameManager was not restored to the TestMain manager")
		closeFrameManagerFrames(baseFrameManager)
		baseFrameManager.Shutdown()
		result = 1
	}

	taskPumps, goroutineProfile, profileErr := taskPumpGoroutineProfile()
	if profileErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "capture goroutine profile after vtui shutdown: %v\n", profileErr)
		result = 1
	} else if taskPumps > 0 {
		_, _ = fmt.Fprintf(os.Stderr,
			"vtui task-pump goroutine leak: %d startTaskPump goroutine(s) remain after test teardown; want 0\n%s",
			taskPumps, goroutineProfile)
		result = 1
	}

	if err == nil {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "remove test config directory: %v\n", removeErr)
			result = 1
		}
	}

	os.Exit(result)
}
