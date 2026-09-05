package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/fusefs"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestAllDialogs_LayoutValidation(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	skipIfNoRelevantChanges(t, "layouts",
		"lang/*.lng",
		"lang/*.txt",
		"file_ops.go",
		"dialog_button_layout.go",
		"*_dialog*.go",
		"*_ui*.go",
		"*_settings*.go",
		"actions*.go",
		"dialog_layouts_test.go",
		"go.mod",
	)
	vtui.SetDefaultPalette()

	// 1. Temporary redirect of the config paths to prevent writing/reading from the user's home directory.
	tmpDir := t.TempDir()

	oldGetConfig := getUserConfigIniPath
	oldConfig := AppConfig
	defer func() {
		getUserConfigIniPath = oldGetConfig
		AppConfig = oldConfig
	}()
	getUserConfigIniPath = func() string {
		return filepath.Join(tmpDir, "settings.ini")
	}

	// Actions that are destructive, async, or mutate global state without a dialog.
	skipActions := map[string]bool{
		"app.quit":                         true,
		"panel.systemexplorer":             true,
		"app.togglewindowsize":             true,
		"panel.rescan":                     true, // no dialog
		"panel.swap":                       true, // no dialog
		"panel.toggle":                     true, // no dialog
		"panel.toggleleftpanel":            true, // no dialog
		"panel.togglerightpanel":           true, // no dialog
		"panel.togglepassivepanel":         true, // no dialog
		"panel.splitleft":                  true, // no dialog
		"panel.splitright":                 true, // no dialog
		"panel.splitup":                    true, // no dialog
		"panel.splitdown":                  true, // no dialog
		"panel.splitactiveup":              true, // no dialog
		"panel.splitactivedown":            true, // no dialog
		"panel.splitreset":                 true, // no dialog
		"panel.viewbrief":                  true, // no dialog
		"panel.viewmedium":                 true, // no dialog
		"panel.viewdetailed":               true, // no dialog
		"panel.viewwide":                   true, // no dialog
		"panel.sortbyname":                 true, // no dialog
		"panel.sortbyext":                  true, // no dialog
		"panel.sortbytime":                 true,
		"panel.sortbysize":                 true,
		"panel.sortunsorted":               true,
		"panel.togglekeybar":               true,
		"panel.toggleinfobytes":            true,
		"panel.togglehidden":               true,
		"panel.historyback":                true,
		"panel.historyforward":             true,
		"file.view":                        true, // async launch
		"file.edit":                        true, // async launch
		"file.new":                         true, // async launch
		"file.attributes":                  true, // async launch
		"file.findduplicates":              true, // async launch
		"terminal.viewlog":                 true, // async launch
		"terminal.editlog":                 true, // async launch
		"editor.switchtoviewer":            true, // async launch
		"viewer.switchtoeditor":            true, // async launch
		"editor.codepagenext":              true, // modifies config
		"viewer.codepagenext":              true, // modifies config
		"editor.save":                      true, // no dialog
		"editor.undo":                      true, // no dialog
		"editor.redo":                      true, // no dialog
		"editor.copy":                      true, // no dialog
		"editor.cut":                       true, // no dialog
		"editor.paste":                     true, // no dialog
		"editor.selectall":                 true, // no dialog
		"editor.deleteline":                true, // no dialog
		"editor.toggleovertype":            true, // no dialog
		"editor.searchnext":                true, // no dialog
		"editor.wordwrap":                  true, // no dialog
		"editor.showwhitespaces":           true, // no dialog
		"editor.insertleftpanelpath":       true, // no dialog
		"editor.insertrightpanelpath":      true, // no dialog
		"editor.insertactivepanelfilename": true, // no dialog
		"editor.deletespacersforward":      true, // no dialog
		"viewer.wrapmode":                  true, // no dialog
		"viewer.hexmode":                   true, // no dialog
		"panel.copypath":                   true, // no dialog
		"panel.copyname":                   true, // no dialog
		"panel.copyselectednames":          true, // no dialog
		"panel.copyselectedpaths":          true, // no dialog
		"panel.copyselectedrealpaths":      true, // no dialog
		"panel.invertselection":            true, // no dialog
		"panel.restoreselection":           true, // no dialog
		"app.screengrab":                   true, // full screen raw frame
		"app.plugring":                     true, // async fetch
		"panel.leftdrivemenu":              true, // relies on active pty/panels
		"panel.rightdrivemenu":             true, // relies on active pty/panels
		"panel.enterdirectory":             true, // no dialog
		"panel.insertfilename":             true, // no dialog
		"panel.insertleftpath":             true, // no dialog
		"panel.insertrightpath":            true, // no dialog
		"debug.dummyoperation":             true, // async queue
		"macro.reload":                     true, // no dialog; owns an asynchronous toast
		"panel.infopanel":                  true, // no dialog
		"panel.quickview":                  true, // no dialog
	}

	// 3. Create a dummy file in the temp directory so file operations (Copy, Edit, etc.)
	// have a valid target and will naturally display their progress/confirmation dialogs.
	srcFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(srcFile, []byte("dummy content"), 0600); err != nil {
		t.Fatal(err)
	}

	oldHotkeys := GlobalHotkeysMgr
	oldMacro := MacroMgr
	defer func() {
		GlobalHotkeysMgr = oldHotkeys
		MacroMgr = oldMacro
	}()
	GlobalHotkeysMgr = NewHotkeyManager(filepath.Join(tmpDir, "hotkeys.ini"))
	MacroMgr = NewMacroManager(filepath.Join(tmpDir, "key_macros.ini"))

	// Load all language packs so the validator can assert layout against all translations dynamically
	packs := LoadAllLanguagePacks()
	if len(packs) == 0 {
		packs = []vtui.LanguagePack{{Name: "current"}}
	}

	oldMountTaskRunner := runPanelMountTask
	runPanelMountTask = func(pf *PanelsFrame, label string, readOnly bool, _ func(context.Context) (*fusefs.Mount, error)) {
		reportMount(pf, label, &fusefs.Mount{
			MountPoint: filepath.Join(tmpDir, "layout-validation-mount"),
			ReadOnly:   readOnly,
		}, nil, readOnly)
	}
	t.Cleanup(func() { runPanelMountTask = oldMountTaskRunner })

	rig := newDialogLayoutRig(t, tmpDir)
	defer rig.close(t)

	// Complex-script widths are handled by vtui's grapheme-cell shaping.
	for _, act := range GetActions() {
		name := act.Name
		if skipActions[strings.ToLower(name)] {
			continue
		}

		t.Run(name, func(t *testing.T) {
			rules := vtui.DefaultLayoutRules
			rules.MaxWidth = 120 // Allow configurator/large dialogs to exceed default 78 columns
			baseStrings := vtui.SnapshotStrings()
			defer vtui.ReplaceStrings(baseStrings)

			var msgs []string
			for _, pack := range packs {
				vtui.ReplaceStrings(baseStrings)
				if len(pack.Strings) > 0 {
					vtui.AddStrings(pack.Strings)
				}

				var errs []error
				if dialogLayoutActionNeedsFreshRig(name) {
					errs = func() []error {
						rig.detach(t)
						defer rig.attach()

						fresh := newDialogLayoutRig(t, tmpDir)
						defer fresh.close(t)
						return fresh.validateAction(t, act, name, srcFile, rules)
					}()
				} else {
					errs = func() []error {
						defer rig.reset(t)
						return rig.validateAction(t, act, name, srcFile, rules)
					}()
				}

				packName := pack.Name
				if packName == "" {
					packName = "?"
				}
				for _, err := range errs {
					msgs = append(msgs, fmt.Sprintf("[lang:%s] %s", packName, err))
				}
			}

			if len(msgs) > 0 {
				t.Errorf("Layout validation failed for Action %s:\n%s", name, strings.Join(msgs, "\n"))
			}
		})
	}
}

type dialogLayoutRig struct {
	manager    *vtui.FrameManagerType
	screen     *vtui.ScreenBuf
	baseScreen *vtui.AppScreen
	panels     *PanelsFrame
	localVFS   vfs.VFS
}

func newDialogLayoutRig(t *testing.T, dir string) *dialogLayoutRig {
	t.Helper()
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 60)
	manager := vtui.FrameManager
	manager.Init(screen)

	localVFS := vfs.NewOSVFS(dir)
	if err := localVFS.SetPath(dir); err != nil {
		t.Fatal(err)
	}
	panels := NewPanelsFrame()
	left := NewFileSystemPanel(0, 0, 40, 20, localVFS)
	right := NewFileSystemPanel(40, 0, 40, 20, localVFS.Clone())
	waitForLoad(t, left)
	waitForLoad(t, right)
	panels.panels[0] = left
	panels.panels[1] = right
	panels.ResizeConsole(120, 60)
	waitForLoad(t, panels.panels[0].(*FileSystemPanel))
	waitForLoad(t, panels.panels[1].(*FileSystemPanel))
	manager.Push(panels)

	return &dialogLayoutRig{
		manager:    manager,
		screen:     screen,
		baseScreen: manager.Screens[manager.ActiveIdx],
		panels:     panels,
		localVFS:   localVFS,
	}
}

func (rig *dialogLayoutRig) validateAction(t *testing.T, act Action, name, srcFile string, rules vtui.LayoutRules) []error {
	t.Helper()
	if strings.HasPrefix(name, "Editor.") {
		showEditor(rig.panels, rig.localVFS, srcFile, &vfs.MemoryReadAtCloser{Data: []byte("dummy")})
	} else if strings.HasPrefix(name, "Viewer.") {
		viewer, err := NewViewerView(context.Background(), rig.localVFS, srcFile)
		if err == nil {
			showViewer(rig.panels, viewer, srcFile)
		}
	}

	initialCount := len(rig.manager.Screens[rig.manager.ActiveIdx].Frames)
	act.Handler()
	waitForLoad(t, rig.panels.panels[0].(*FileSystemPanel))
	waitForLoad(t, rig.panels.panels[1].(*FileSystemPanel))
	if rig.manager.GetActiveToast() != "" {
		waitForToastExpiry(t, 6*time.Second)
	}

	frames := rig.manager.Screens[rig.manager.ActiveIdx].Frames
	if len(frames) <= initialCount {
		// Many actions are silent and do not open a dialog (e.g. Editor.Save).
		// This is completely expected, so skip validation for this pass.
		return nil
	}

	// Check if the top-most frame is a container. If not (e.g. raw drawing view), skip it safely.
	topFrame := frames[len(frames)-1]
	container, ok := topFrame.(vtui.Container)
	if !ok {
		return nil
	}
	return vtui.ValidateLayoutWithRules(container, rules)
}

func (rig *dialogLayoutRig) reset(t *testing.T) {
	t.Helper()
	waitForDirectoryLoads(t)

	baseIdx := -1
	for i, screen := range rig.manager.Screens {
		if screen == rig.baseScreen {
			baseIdx = i
			continue
		}
		closeFrameManagerScreens([]*vtui.AppScreen{screen})
	}
	if baseIdx < 0 {
		t.Fatal("layout validation action removed the shared panels workspace")
	}

	// Drop any editor/viewer or action-created workspaces without rebuilding
	// the shared panel fixture. Their frames have already been closed above.
	for i := len(rig.manager.Screens) - 1; i >= 0; i-- {
		if rig.manager.Screens[i] == rig.baseScreen {
			continue
		}
		before := len(rig.manager.Screens)
		rig.manager.CloseScreen(i)
		if len(rig.manager.Screens) != before-1 {
			t.Fatal("layout validation action left an extra workspace that could not be closed")
		}
	}

	for i, screen := range rig.manager.Screens {
		if screen == rig.baseScreen {
			baseIdx = i
			break
		}
	}
	rig.manager.SwitchScreen(baseIdx)
	frames := append([]vtui.Frame(nil), rig.baseScreen.Frames...)
	foundPanels := false
	for i := len(frames) - 1; i >= 0; i-- {
		frame := frames[i]
		if frame == rig.panels {
			foundPanels = true
			continue
		}
		frame.Close()
		rig.manager.RemoveFrame(frame)
	}
	if !foundPanels || rig.panels.IsDone() {
		t.Fatal("layout validation action mutated the shared panels frame")
	}
}

func (rig *dialogLayoutRig) detach(t *testing.T) {
	t.Helper()
	// The next rig reinitializes the global manager. Clipboard workers read
	// that global asynchronously, so join them at this lifecycle boundary.
	waitForAsyncClipboard()
	rig.reset(t)
}

func (rig *dialogLayoutRig) attach() {
	rig.manager.Init(rig.screen)
	rig.manager.Push(rig.panels)
	rig.baseScreen = rig.manager.Screens[rig.manager.ActiveIdx]
}

func (rig *dialogLayoutRig) close(t *testing.T) {
	t.Helper()
	waitForAsyncClipboard()
	waitForDirectoryLoads(t)
	closeFrameManagerFrames(rig.manager)
}

// These handlers change the reusable PanelsFrame itself (or stop/close its
// manager). Keeping their old per-combination freshness avoids making a later
// action or translation depend on that mutation; ordinary actions share the
// expensive VFS and panels fixture above.
func dialogLayoutActionNeedsFreshRig(name string) bool {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "panel.left.") || strings.HasPrefix(name, "panel.right.") {
		return true
	}
	switch name {
	case "ai.togglepanel",
		"app.background",
		"panel.goparent",
		"panel.goroot",
		"panel.insertpath",
		"panel.selectnavigation",
		"panel.togglecommandlinefocus",
		"workspace.close":
		return true
	default:
		return false
	}
}
