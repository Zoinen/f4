package main

import (
	"errors"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"testing"
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

func TestMain(m *testing.M) {
	newLocalPTY = func() (PtyBackend, error) {
		return nil, errors.New("local PTY disabled for unit-test panel frames")
	}
	vfs.InitSudoClient("/usr/bin/f4", "")

	// Unit tests must never hand control to the user's desktop. Individual
	// tests that exercise these routes install per-dialog/per-frame recorders.
	defaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	defaultNativePropertiesOpener = func(string) error { return nil }

	tmpDir, err := os.MkdirTemp("", "f4-test-config-*")
	if err == nil {
		os.Setenv("XDG_CONFIG_HOME", tmpDir)
		os.Setenv("APPDATA", tmpDir)
		resetConfigDirForTest()
	}

	result := m.Run()

	if err == nil {
		os.RemoveAll(tmpDir)
	}

	if result != 0 {
		// disabled for now
		//vtui.DumpLogsToFile("_failed_tests_f4.log")
	}
	os.Exit(result)
}
