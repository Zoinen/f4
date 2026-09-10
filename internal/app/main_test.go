package app

import (
	"errors"
	"github.com/unxed/f4/internal/panel"
	"os"
	"testing"
	"time"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/internal/update"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// pressKey is testutil.PressKey with this package's macro filter, which is
// where action hotkeys are dispatched. The managers are created on demand
// because most tests never touch them.
func pressKey(f vtui.Frame, e *vtinput.InputEvent) bool {
	if keymap.GlobalHotkeysMgr == nil {
		keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	}
	if macro.MacroMgr == nil {
		macro.MacroMgr = macro.NewMacroManager("")
	}
	return testutil.PressKey(f, e, func(e *vtinput.InputEvent) bool {
		return macroFilter(macro.MacroMgr, e)
	})
}

// preserveActionRegistry keeps tests that register synthetic actions from
// leaking them into later tests or the next -count iteration. The copy itself
// lives in internal/action, which is the only package that can reach the maps.
func preserveActionRegistry(t *testing.T) {
	t.Helper()
	t.Cleanup(action.Snapshot())
}

func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, installTestSeams, unmountTestFilesystems))
}

// installTestSeams points this package's escape hatches somewhere harmless for
// the duration of the run.
func installTestSeams() {
	// f4 extends vtui's palette past its last index, and any widget drawn with
	// one of the extra colours indexes past the end of the default one. Which
	// test draws first depends on the shuffle seed, so the palette is sized
	// here rather than left to whichever test happens to grow it.
	theme.SetDefaultF4Palette()

	vfs.InitSudoClient("/usr/bin/f4", "")

	// main() wires this before it reads the config directory; without it here
	// the tests would exercise the "resolver not wired" fallback instead of the
	// path a running f4 takes.
	config.Executable = update.Executable
	keymap.Suspended = KeyRemapSuspended

	// SetupUI installs this in production; the test binary never runs it, and
	// without it every action label falls back to its English spelling.
	action.Localize = i18n.Msg
	// internal/editor declares what it needs from above; this is the root
	// filling it in. Each is one call site inside the editor.
	editor.RunAction = RunAction
	editor.LookupHotkey = func(e *vtinput.InputEvent) bool { return macroLookupHotkey(macro.MacroMgr, e) }
	editor.MenuBarItems = BuildMenuBarItems
	editor.CrossAttrs = editor.EditorCrossAttrs
	editor.KeyBarLabels = keymap.KeyBarLabelsForArea
	editor.HotkeyAction = func(area, key string) string {
		if keymap.GlobalHotkeysMgr == nil {
			return ""
		}
		return keymap.GlobalHotkeysMgr.GetAction(area, key)
	}
	editor.RememberEdited = func(v vfs.VFS, path string) { RememberViewerEditorHistory(v, path, HistoryModeEdit) }
	editor.SaveSession = SaveSession
	editor.HandleWorkspaceFork = HandleWorkspaceForkCommand
	editor.SwitchToViewer = ActionSwitchEditorToViewer
	// internal/panel declares what it needs from the application above it; this
	// is the root filling it in. Every default is inert, so an unwired panel
	// declines the command rather than doing the wrong thing.
	panel.AppCommand = handlePanelsAppCommand
	panel.RunAction = RunAction
	panel.BuildMenuBarItems = BuildMenuBarItems
	panel.SaveSession = SaveSession
	panel.OpenEditor = ActionOpenEditor
	panel.OpenViewer = ActionOpenViewer
	panel.OpenViewerInternal = OpenViewerInternal
	panel.OpenEditFileIn = OpenEditFileIn
	panel.ShowViewer = ShowViewer
	panel.ShowEditor = ShowEditor
	panel.FindOpenedEditor = FindOpenedEditor
	panel.Execute = ActionExecute
	panel.SortMenuForPanel = ActionSortMenuForPanel
	panel.WorkspaceClose = ActionWorkspaceClose
	panel.Arkanoid = ActionArkanoid
	panel.CurrentArea = macroCurrentArea
	panel.MacroHotkey = func(e *vtinput.InputEvent) bool { return macroLookupHotkey(macro.MacroMgr, e) }
	panel.KeyFilter = func(e *vtinput.InputEvent) bool { return macroFilter(macro.MacroMgr, e) }
	panel.AISetViewMode = aiSetViewMode

	// Unit tests must never hand control to the user's desktop. Individual
	// tests that exercise these routes install per-dialog/per-frame recorders.
	panel.DefaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	dialog.DefaultNativePropertiesOpener = func(string) error { return nil }

	// Frames must not fork the user's shell during unit tests; the few
	// tests that exercise the term.PTY path construct one explicitly.
	panel.SpawnLocalShellPTY = false

	// Toast behavior is still exercised through vtui's real asynchronous
	// setup and expiry paths, but unit tests do not need production-length
	// display times. Keep a small observable window: tests may observe another
	// effect of the same UI task (for example, clipboard contents) before they
	// pump the nested toast task, and a 1 ms toast can expire in that gap.
	toast.DurationOverride = func(time.Duration) time.Duration {
		const minimumObservableToastDuration = 100 * time.Millisecond
		return minimumObservableToastDuration
	}
	fileops.QueueShowToast = func(string, time.Duration) {}

	// os.UserConfigDir ignores XDG_CONFIG_HOME and APPDATA on darwin, so the
	// seam is what actually isolates the suite from the developer's profile.
	if dir := testutil.ConfigDir(); dir != "" {
		config.UserConfigDir = func() (string, error) { return dir, nil }
		config.ResetConfigDirForTest()
	}
}

func unmountTestFilesystems() error {
	return errors.Join(fusefs.UnmountAll()...)
}
