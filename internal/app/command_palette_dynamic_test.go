package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"sync"
)

type commandPaletteOtherFrame struct{ vtui.BaseFrame }

func (*commandPaletteOtherFrame) GetType() vtui.FrameType { return vtui.TypeUser + 7 }

func setCommandPaletteActivePanelsForTest(t *testing.T, pf *panel.PanelsFrame) {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	t.Cleanup(testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Number: 1, Frames: []vtui.Frame{pf}}}, 0))
}

// paletteMacroHost records what a macro would type. Everything else falls
// through to the real host: these tests list and select palette entries, and
// the only host call that reaches is the key injection they assert on.
type paletteMacroHost struct {
	F4MacroHost
	mu       sync.Mutex
	injected []*vtinput.InputEvent
}

func (h *paletteMacroHost) InjectKeys(keys []*vtinput.InputEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.injected = append(h.injected, keys...)
}

func (h *paletteMacroHost) injectedKeys() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	names := make([]string, 0, len(h.injected))
	for _, event := range h.injected {
		names = append(names, keymap.EventToFarString(event))
	}
	return names
}

// newTestMacroEngine builds a Lua engine over the real host: the palette lists
// macro bindings and never runs one, so nothing here reaches the UI.
func newTestMacroEngine(t *testing.T, host macro.MacroHost, source string) *macro.LuaMacroEngine {
	t.Helper()
	Engine, err := macro.NewLuaMacroEngine(host)
	if err != nil {
		t.Fatalf("NewLuaMacroEngine: %v", err)
	}
	t.Cleanup(func() {
		if err := Engine.Close(); err != nil {
			t.Errorf("close Lua macro engine: %v", err)
		}
	})
	if source != "" {
		if err := Engine.LoadString("test", source); err != nil {
			t.Fatalf("LoadString: %v", err)
		}
	}
	return Engine
}

func TestCommandPaletteIncludesRecordedAndLuaMacros(t *testing.T) {
	previous := macro.MacroMgr
	host := &paletteMacroHost{}
	Engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlL"; description = "Lint current item";
			action = function() Keys("F7") end }
	`)
	t.Cleanup(func() {
		if err := Engine.Close(); err != nil {
			t.Errorf("close Lua macro engine: %v", err)
		}
		macro.MacroMgr = previous
	})
	macro.MacroMgr = &macro.MacroManager{
		Macros: map[string]map[string][]*vtinput.InputEvent{
			"Shell":  {"CtrlR": {keymap.ParseFarKey("F5")}},
			"Common": {"AltR": {keymap.ParseFarKey("F6")}},
		},
		Lua: Engine,
	}

	entries := commandPaletteMacroEntries("Shell")
	byKey := make(map[string]commandPaletteEntry)
	for _, entry := range entries {
		byKey[entry.Key] = entry
	}
	for _, key := range []string{"macro:record-toggle", "recorded-macro:shell:ctrlr", "recorded-macro:common:altr", "lua-macro:shell:ctrll"} {
		if _, ok := byKey[key]; !ok {
			t.Fatalf("missing command-palette macro %q in %#v", key, byKey)
		}
	}
	results := rankCommandPaletteEntries(entries, "lint current", nil)
	if len(results) == 0 || results[0].Key != "lua-macro:shell:ctrll" {
		t.Fatalf("Lua description search = %#v", results)
	}
	if !executeCommandPaletteEntry(byKey["lua-macro:shell:ctrll"]) {
		t.Fatal("Lua macro did not start from the palette")
	}
	if !Engine.WaitIdle(time.Second) {
		t.Fatal("Lua macro did not finish")
	}
	if got := strings.Join(host.injectedKeys(), " "); got != "F7" {
		t.Fatalf("injected Lua keys = %q, want F7", got)
	}
}

func TestCommandPaletteLuaMacroStaleAreaBindingDoesNotFallBackToCommon(t *testing.T) {
	previous := macro.MacroMgr
	host := &paletteMacroHost{}
	Engine := newTestMacroEngine(t, host, `
		Macro { area = "Shell"; key = "CtrlX"; description = "Shell command";
			action = function() Keys("F5") end }
		Macro { area = "Common"; key = "CtrlX"; description = "Common command";
			action = function() Keys("F6") end }
	`)
	t.Cleanup(func() { macro.MacroMgr = previous })
	macro.MacroMgr = &macro.MacroManager{Lua: Engine}

	var shellEntry commandPaletteEntry
	for _, entry := range commandPaletteLuaMacroEntries("Shell", "Macros", nil) {
		if entry.Key == "lua-macro:shell:ctrlx" {
			shellEntry = entry
			break
		}
	}
	if shellEntry.Key == "" {
		t.Fatal("Shell Lua macro entry is missing")
	}
	if !Engine.Remove("Shell", "CtrlX") {
		t.Fatal("failed to remove Shell binding")
	}
	if executeCommandPaletteEntry(shellEntry) {
		t.Fatal("stale Shell entry fell back to a Common macro")
	}
	if !Engine.WaitIdle(time.Second) {
		t.Fatal("macro engine did not become idle")
	}
	if got := strings.Join(host.injectedKeys(), " "); got != "" {
		t.Fatalf("stale entry injected %q", got)
	}
}

func TestCommandPaletteCommandPrefixReResolvesBeforeInsertion(t *testing.T) {
	api := &CoreAPI{}
	registration, err := api.RegisterCommandPrefix("test.palette-prefix", "deploy", func(vfs.App, string) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	pf := &panel.PanelsFrame{CmdLine: cmdline.NewCommandLine("")}
	setCommandPaletteActivePanelsForTest(t, pf)
	entries := commandPalettePrefixEntries("Shell", pf)
	var entry commandPaletteEntry
	for _, candidate := range entries {
		if candidate.ID == "test.palette-prefix" {
			entry = candidate
			break
		}
	}
	if entry.ID == "" || entry.Label != "deploy:" {
		t.Fatalf("test prefix is missing from %#v", entries)
	}
	if !executeCommandPaletteEntry(entry) || pf.CmdLine.Edit.GetText() != "deploy:" {
		t.Fatalf("prefix execution produced %q", pf.CmdLine.Edit.GetText())
	}
	registration.Unregister()
	pf.CmdLine.Edit.SetText("")
	if executeCommandPaletteEntry(entry) || pf.CmdLine.Edit.GetText() != "" {
		t.Fatal("stale command-prefix entry executed after unregister")
	}

	registration, err = api.RegisterCommandPrefix("test.palette-prefix", "deploy", func(vfs.App, string) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
	pf.Closed = true
	if executeCommandPaletteEntry(entry) || pf.CmdLine.Edit.GetText() != "" {
		t.Fatal("command-prefix entry mutated a closed panels frame")
	}
}

func TestCommandPalettePrefixAndDriveRejectPreviousWorkspace(t *testing.T) {
	api := &CoreAPI{}
	registration, err := api.RegisterCommandPrefix("test.stale-workspace-prefix", "stale", func(vfs.App, string) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	pf := &panel.PanelsFrame{
		CmdLine: cmdline.NewCommandLine(""),
		Panels:  [2]panel.Panel{&panel.FileSystemPanel{}, &panel.FileSystemPanel{}},
	}
	setCommandPaletteActivePanelsForTest(t, pf)
	prefixEntries := commandPalettePrefixEntries("Shell", pf)
	var prefixEntry commandPaletteEntry
	for _, entry := range prefixEntries {
		if entry.ID == "test.stale-workspace-prefix" {
			prefixEntry = entry
			break
		}
	}
	if prefixEntry.Key == "" {
		t.Fatal("stale-workspace prefix entry is missing")
	}

	factoryCalls := 0
	restoreDrives := replaceDriveRegistryForCommandPaletteTest([]sysinfo.DriveEntry{{
		Name: "Stale workspace drive",
		Factory: func() vfs.VFS {
			factoryCalls++
			return nil
		},
	}})
	t.Cleanup(restoreDrives)
	var driveEntry commandPaletteEntry
	for _, entry := range commandPaletteDriveEntries(pf) {
		if entry.ID == "Stale workspace drive" {
			driveEntry = entry
			break
		}
	}
	if driveEntry.Key == "" {
		t.Fatal("stale-workspace drive entry is missing")
	}

	current := &panel.PanelsFrame{CmdLine: cmdline.NewCommandLine(""), Panels: [2]panel.Panel{&panel.FileSystemPanel{}, &panel.FileSystemPanel{}}}
	t.Cleanup(testutil.AppendFrameManagerScreen(t, &vtui.AppScreen{Number: 2, Frames: []vtui.Frame{current}}, 1))
	if executeCommandPaletteEntry(prefixEntry) || pf.CmdLine.Edit.GetText() != "" {
		t.Fatal("stale prefix entry mutated the previous workspace")
	}
	if executeCommandPaletteEntry(driveEntry) || factoryCalls != 0 {
		t.Fatalf("stale drive entry resolved its factory %d times", factoryCalls)
	}
}

func TestCommandPaletteKeysAreNotCapturedWhileRecording(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.Push(&commandPaletteOtherFrame{})

	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Common": {"CtrlShiftP": CommandPaletteActionName}},
		Bindings: map[string]map[string]string{"Common": {"CtrlShiftP": CommandPaletteActionName}},
	}
	manager := &macro.MacroManager{Recording: true, Buffer: make([]*vtinput.InputEvent, 0)}
	macro.MacroMgr = manager
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	open := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_P, Char: 'P',
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	}
	if !macroFilter(manager, open) {
		t.Fatal("recording shadowed the command palette shortcut")
	}
	if len(manager.Buffer) != 0 {
		t.Fatalf("opening shortcut was recorded: %#v", manager.Buffer)
	}
	if _, ok := vtui.FrameManager.GetTopFrame().(*CommandPaletteDialog); !ok {
		t.Fatalf("top frame = %T, want command palette", vtui.FrameManager.GetTopFrame())
	}
	if !macroFilter(manager, open) {
		t.Fatal("an already-open palette did not consume its shortcut")
	}
	if len(manager.Buffer) != 0 {
		t.Fatalf("repeated palette shortcut was recorded: %#v", manager.Buffer)
	}
	query := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_H, Char: 'h'}
	if macroFilter(manager, query) {
		t.Fatal("palette query key was consumed by the macro manager")
	}
	if len(manager.Buffer) != 0 {
		t.Fatalf("palette query was recorded: %#v", manager.Buffer)
	}
}

func TestCommandPaletteOpensInOtherFullScreenAreas(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	vtui.FrameManager.Push(&commandPaletteOtherFrame{})

	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Common": {"CtrlShiftP": CommandPaletteActionName}},
		Bindings: map[string]map[string]string{"Common": {"CtrlShiftP": CommandPaletteActionName}},
	}
	manager := &macro.MacroManager{Macros: make(map[string]map[string][]*vtinput.InputEvent)}
	macro.MacroMgr = manager
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	event := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_P, Char: 'P',
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	}
	if !macroFilter(manager, event) {
		t.Fatal("Ctrl+Shift+P was not consumed in an Other full-screen area")
	}
	if _, ok := vtui.FrameManager.GetTopFrame().(*CommandPaletteDialog); !ok {
		t.Fatalf("top frame = %T, want command palette", vtui.FrameManager.GetTopFrame())
	}
}

func TestCommandPaletteIndexesImageAndQueueFrameCommands(t *testing.T) {
	imageEntries := commandPaletteImageEntries(&media.ImageView{})
	wantImage := map[string]bool{
		"Image.Reload": true, "Image.FullScreen": true, "Image.SlideShow": true,
		"Image.ZoomIn": true, "Image.RotateClockwise": true, "Image.Gallery": true,
		"Image.Close": true,
	}
	for _, entry := range imageEntries {
		delete(wantImage, entry.ID)
		if entry.Description != entry.Label {
			t.Errorf("image %s displayed non-localized description %q instead of label %q", entry.ID, entry.Description, entry.Label)
		}
	}
	if len(wantImage) != 0 {
		t.Fatalf("missing image commands: %v", wantImage)
	}
	results := rankCommandPaletteEntries(imageEntries, "перезагрузить изображение", nil)
	if len(results) == 0 || results[0].ID != "Image.Reload" {
		t.Fatalf("Russian image query = %#v", results)
	}

	queueEntries := commandPaletteQueueEntries(&fileops.QueueFrame{})
	wantQueue := map[string]bool{"Queue.OpenDetails": true, "Queue.Cancel": true, "Queue.Clear": true, "Queue.Close": true}
	for _, entry := range queueEntries {
		delete(wantQueue, entry.ID)
		if entry.Description != entry.Label {
			t.Errorf("queue %s displayed non-localized description %q instead of label %q", entry.ID, entry.Description, entry.Label)
		}
	}
	if len(wantQueue) != 0 {
		t.Fatalf("missing queue commands: %v", wantQueue)
	}
}

func TestCommandPaletteImageGalleryCommandsUseGalleryCursor(t *testing.T) {
	initFrameworkActionTestScreen(t)
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = nil
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })

	image := media.NewGalleryView("first.png", []string{"first.png", "second.png", "third.png"}, 0, 1)
	var selectedPath string
	var selectedState bool
	image.OnSelect = func(path string, selected bool) {
		selectedPath, selectedState = path, selected
	}

	vtui.FrameManager.Push(image)

	entries := make(map[string]commandPaletteEntry)
	for _, entry := range commandPaletteImageEntries(image) {
		entries[entry.ID] = entry
	}
	if !executeCommandPaletteEntry(entries["Image.Next"]) {
		t.Fatal("gallery Next command was not executed")
	}
	if image.Gal.Cursor != 2 || image.Path != "first.png" || image.Index != 0 {
		t.Fatalf("gallery Next cursor=%d path=%q index=%d, want cursor-only move to 2", image.Gal.Cursor, image.Path, image.Index)
	}
	if !executeCommandPaletteEntry(entries["Image.Previous"]) {
		t.Fatal("gallery Previous command was not executed")
	}
	if image.Gal.Cursor != 1 || image.Path != "first.png" || image.Index != 0 {
		t.Fatalf("gallery Previous cursor=%d path=%q index=%d, want cursor-only move to 1", image.Gal.Cursor, image.Path, image.Index)
	}
	if !executeCommandPaletteEntry(entries["Image.First"]) {
		t.Fatal("gallery First command was not executed")
	}
	if image.Gal.Cursor != 0 || image.Path != "first.png" || image.Index != 0 {
		t.Fatalf("gallery First cursor=%d path=%q index=%d, want cursor-only move to 0", image.Gal.Cursor, image.Path, image.Index)
	}
	if !executeCommandPaletteEntry(entries["Image.Last"]) {
		t.Fatal("gallery Last command was not executed")
	}
	if image.Gal.Cursor != 2 || image.Path != "first.png" || image.Index != 0 {
		t.Fatalf("gallery Last cursor=%d path=%q index=%d, want cursor-only move to 2", image.Gal.Cursor, image.Path, image.Index)
	}

	image.Gal.Cursor = 1
	if !executeCommandPaletteEntry(entries["Image.Select"]) {
		t.Fatal("gallery Select command was not executed")
	}
	if !image.Selected["second.png"] || image.Selected["first.png"] || image.Gal.Cursor != 2 {
		t.Fatalf("gallery Select selected=%v cursor=%d, want second.png and cursor 2", image.Selected, image.Gal.Cursor)
	}
	if selectedPath != "second.png" || !selectedState {
		t.Fatalf("gallery Select callback = (%q, %v), want (second.png, true)", selectedPath, selectedState)
	}

	image.Gal.Cursor = 1
	selectedPath, selectedState = "", true
	if !executeCommandPaletteEntry(entries["Image.ClearSelection"]) {
		t.Fatal("gallery Clear Selection command was not executed")
	}
	if image.Selected["second.png"] || image.Gal.Cursor != 2 {
		t.Fatalf("gallery Clear selected=%v cursor=%d, want cleared second.png and cursor 2", image.Selected, image.Gal.Cursor)
	}
	if selectedPath != "second.png" || selectedState {
		t.Fatalf("gallery Clear callback = (%q, %v), want (second.png, false)", selectedPath, selectedState)
	}

	workspaceList, _ := GetAction("Workspace.List")
	if got := keymap.NativeShortcutsForAction("Other", workspaceList); len(got) != 0 {
		t.Fatalf("image context advertised gallery-owned F12 for Workspace.List: %v", got)
	}
	workspaceNew, _ := GetAction("Workspace.New")
	if got := keymap.NativeShortcutsForAction("Other", workspaceNew); len(got) != 0 {
		t.Fatalf("image context advertised inactive-stack Ctrl+N fallback: %v", got)
	}
}

func TestCommandPaletteQueueClosePreservesVetoAndClosesWhenIdle(t *testing.T) {
	initFrameworkActionTestScreen(t)
	previousQueue := fileops.GlobalQueueManager
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = nil
	t.Cleanup(func() {
		fileops.GlobalQueueManager = previousQueue
		keymap.GlobalHotkeysMgr = previousHotkeys
	})

	vtui.FrameManager.Push(&commandPaletteOtherFrame{})
	task := &fileops.QueueTask{ID: 1, State: "Running"}
	fileops.GlobalQueueManager = fileops.NewQueueManagerWithTasks(task)
	queue := fileops.NewQueueFrame()
	queue.UpdateTasks([]*fileops.QueueTask{task})
	vtui.FrameManager.AddScreen(queue)

	var closeEntry commandPaletteEntry
	for _, entry := range commandPaletteQueueEntries(queue) {
		if entry.ID == "Queue.Close" {
			closeEntry = entry
			break
		}
	}
	if closeEntry.ID == "" {
		t.Fatal("Queue.Close palette entry is missing")
	}
	if !ActionWorkspaceClose() || len(vtui.FrameManager.Screens) != 2 {
		t.Fatal("generic Workspace.Close bypassed the active queue veto")
	}
	if !executeCommandPaletteEntry(closeEntry) || len(vtui.FrameManager.Screens) != 2 {
		t.Fatal("Queue.Close bypassed the active queue veto")
	}
	workspaceClose, _ := GetAction("Workspace.Close")
	if got := keymap.NativeShortcutsForAction("Other", workspaceClose); len(got) != 0 {
		t.Fatalf("active queue advertised veto-owned Ctrl+W workspace close: %v", got)
	}

	task.SetState("Done")
	if got := keymap.NativeShortcutsForAction("Other", workspaceClose); len(got) != 1 || got[0] != "Ctrl+W" {
		t.Fatalf("idle queue workspace-close shortcut = %v, want [Ctrl+W]", got)
	}
	if !executeCommandPaletteEntry(closeEntry) {
		t.Fatal("Queue.Close did not close an idle queue workspace")
	}
	if got := len(vtui.FrameManager.Screens); got != 1 {
		t.Fatalf("screens after idle Queue.Close = %d, want 1", got)
	}
}

func TestCommandPaletteIndexesPanelContextAndPlatformDriveCommands(t *testing.T) {
	left, right := &panel.FileSystemPanel{}, &panel.FileSystemPanel{}
	pf := &panel.PanelsFrame{ShowPanels: true, Panels: [2]panel.Panel{left, right}}
	entries := commandPalettePanelsContextEntries(pf)
	want := map[string]bool{
		"Panel.ActivateSelected": true,
		"Panel.SwitchActive":     true,
		"Panel.ToggleSelection":  true,
		"Bookmark.Save.0":        true,
		"Bookmark.Save.9":        true,
		"Bookmark.Home":          true,
	}
	for _, entry := range entries {
		delete(want, entry.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing panel-context commands: %v", want)
	}
	if results := rankCommandPaletteEntries(entries, "переключить активную панель", nil); len(results) == 0 || results[0].ID != "Panel.SwitchActive" {
		t.Fatalf("Russian panel-context query = %#v", results)
	}
	activateEntry, found := commandPaletteTestEntryByID(entries, "Panel.ActivateSelected")
	if !found || activateEntry.Description != i18n.Msg("CommandPalette.Panel.ActivateSelected.Desc") || activateEntry.EnglishDescription != "Open the selected item or execute it" {
		t.Fatalf("localized panel activation metadata = %#v", activateEntry)
	}

	quick := panel.NewQuickViewPanel(left)
	quick.SetFocus(true)
	pf.AltPanels[0] = quick
	entries = commandPalettePanelsContextEntries(pf)
	foundQuickView := false
	for _, entry := range entries {
		if entry.ID == "QuickView.ToggleWrap" {
			foundQuickView = true
			if entry.Description != i18n.Msg("CommandPalette.QuickView.ToggleWrap.Desc") || entry.EnglishDescription != "Toggle long-line wrapping in Quick View" {
				t.Fatalf("localized Quick View metadata = %#v", entry)
			}
			break
		}
	}
	if !foundQuickView {
		t.Fatal("focused Quick View wrap command is missing")
	}

	driveEntries := commandPaletteDriveEntries(pf)
	otherCount, platformCount := 0, 0
	for _, entry := range driveEntries {
		switch {
		case entry.ID == "Panel.Other":
			otherCount++
		case strings.HasPrefix(entry.ID, "Platform."):
			platformCount++
		}
	}
	if otherCount != 2 {
		t.Fatalf("other-panel drive commands = %d, want 2", otherCount)
	}
	if wantPlatform := len(sysinfo.GetPlatformDrives()) * 2; platformCount != wantPlatform {
		t.Fatalf("platform drive commands = %d, want %d", platformCount, wantPlatform)
	}
}

func TestCommandPaletteBookmarksRejectPluginOnlyAndStalePanelTargets(t *testing.T) {
	// Keep unrelated process-wide config users on the TestMain directory.
	_ = config.GetF4ConfigDir()
	configRoot := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return configRoot, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })

	bookmarks := panel.BookmarkSet{}
	bookmarks[1] = panel.Bookmark{Plugin: "legacy-plugin", PluginData: "opaque"}
	bookmarks[2] = panel.Bookmark{Path: `C:\valid`}
	if err := panel.SaveBookmarks(panel.BookmarksFilePath(), bookmarks); err != nil {
		t.Fatalf("save bookmark fixture: %v", err)
	}

	pf := &panel.PanelsFrame{
		ShowPanels: true,
		Panels: [2]panel.Panel{
			&panel.FileSystemPanel{Vfs: vfs.NewNullVFS(0)},
			&panel.FileSystemPanel{Vfs: vfs.NewNullVFS(0)},
		},
	}
	entries := commandPaletteBookmarkEntries(pf)
	byID := make(map[string]commandPaletteEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	if _, found := byID["Bookmark.GoTo.1"]; found {
		t.Fatal("plugin-only bookmark with an empty Path produced a Go To command")
	}
	if _, found := byID["Bookmark.GoTo.2"]; !found {
		t.Fatal("bookmark with a path did not produce a Go To command")
	}
	driveBookmarkCounts := make(map[string]int)
	for _, entry := range commandPaletteDriveEntries(pf) {
		if strings.HasPrefix(entry.ID, "Bookmark.") {
			driveBookmarkCounts[entry.ID]++
		}
	}
	if driveBookmarkCounts["Bookmark.1"] != 0 {
		t.Fatal("plugin-only bookmark with an empty Path produced drive commands")
	}
	if driveBookmarkCounts["Bookmark.2"] != 2 {
		t.Fatalf("bookmark drive commands = %d, want left and right", driveBookmarkCounts["Bookmark.2"])
	}

	initFrameworkActionTestScreen(t)
	vtui.FrameManager.Push(&commandPaletteOtherFrame{})
	for _, id := range []string{"Bookmark.GoTo.2", "Bookmark.Save.0", "Bookmark.Home"} {
		entry, found := byID[id]
		if !found || entry.run == nil {
			t.Fatalf("bookmark command %s is missing", id)
		}
		if entry.run() {
			t.Errorf("bookmark command %s mutated a PanelsFrame that is no longer top", id)
		}
	}
	after, err := panel.LoadBookmarks(panel.BookmarksFilePath())
	if err != nil {
		t.Fatalf("reload bookmark fixture: %v", err)
	}
	if after != bookmarks {
		t.Fatalf("stale bookmark command changed the bookmark file: got %#v, want %#v", after, bookmarks)
	}
}

func TestCommandPaletteIndexesFocusedInfoAndAIChatCommands(t *testing.T) {
	left, right := &panel.FileSystemPanel{}, &panel.FileSystemPanel{}
	pf := &panel.PanelsFrame{
		ShowPanels: true,
		ActiveIdx:  0,
		Panels:     [2]panel.Panel{left, right},
	}

	info := &panel.InfoPanel{Focused: true}
	pf.AltPanels[0] = info
	infoEntries := commandPalettePanelsContextEntries(pf)
	infoEntry, found := commandPaletteTestEntryByID(infoEntries, "InfoPanel.CopyCurrent")
	if !found {
		t.Fatal("focused InfoPanel copy command is missing")
	}
	if infoEntry.Description != i18n.Msg("CommandPalette.Info.CopyCurrent.Desc") ||
		infoEntry.EnglishDescription != "Copy the focused information value or selected rows to the clipboard" {
		t.Fatalf("InfoPanel copy descriptions = (%q, %q)", infoEntry.Description, infoEntry.EnglishDescription)
	}
	if results := rankCommandPaletteEntries(infoEntries, "скопировать текущее значение информации", nil); len(results) == 0 || results[0].ID != "InfoPanel.CopyCurrent" {
		t.Fatalf("Russian InfoPanel copy query = %#v", results)
	}
	info.Focused = false
	if commandPaletteTestHasID(commandPalettePanelsContextEntries(pf), "InfoPanel.CopyCurrent") {
		t.Fatal("unfocused InfoPanel exposed its copy command")
	}

	chat := &AIChatPanel{
		focused:        true,
		focusedLinkIdx: 0,
		visibleLinks:   []chatLink{{target: "ai://out/result.txt"}},
	}
	pf.AltPanels[0] = chat
	linkEntries := commandPalettePanelsContextEntries(pf)
	for _, id := range []string{"AI.CopyLastResponse", "AI.OpenFocusedLink", "AI.CopyFocusedLinkTarget"} {
		if !commandPaletteTestHasID(linkEntries, id) {
			t.Errorf("focused AI link command %s is missing", id)
		}
	}
	if results := rankCommandPaletteEntries(linkEntries, "скопировать объект выбранной ссылки ии", nil); len(results) == 0 || results[0].ID != "AI.CopyFocusedLinkTarget" {
		t.Fatalf("Russian focused AI link query = %#v", results)
	}

	chat.focusedLinkIdx = -2
	patchEntries := commandPaletteAIChatFocusedEntries(pf, chat, aiBarPatch)
	for _, id := range []string{"AI.ActivateFocusedBar", "AI.InspectFocusedPatch"} {
		if !commandPaletteTestHasID(patchEntries, id) {
			t.Errorf("focused AI patch-bar command %s is missing", id)
		}
	}
	filesEntries := commandPaletteAIChatFocusedEntries(pf, chat, aiBarFiles)
	if !commandPaletteTestHasID(filesEntries, "AI.ActivateFocusedBar") || commandPaletteTestHasID(filesEntries, "AI.InspectFocusedPatch") {
		t.Fatalf("focused AI files-bar commands = %#v", filesEntries)
	}
	chat.focused = false
	if entries := commandPaletteAIChatFocusedEntries(pf, chat, aiBarPatch); len(entries) != 0 {
		t.Fatalf("unfocused AI chat exposed focused commands: %#v", entries)
	}
}

func commandPaletteTestHasID(entries []commandPaletteEntry, id string) bool {
	_, found := commandPaletteTestEntryByID(entries, id)
	return found
}

func commandPaletteTestEntryByID(entries []commandPaletteEntry, id string) (commandPaletteEntry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return commandPaletteEntry{}, false
}
