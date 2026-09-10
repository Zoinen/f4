package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/settings"
	terminalpkg "github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorkspaceSemanticSwitchUsesExistingScreenModel(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewWindow(0, 0, 20, 10, "One"))
	vtui.FrameManager.AddScreen(vtui.NewWindow(0, 0, 20, 10, "Two"))

	if len(vtui.FrameManager.Screens) != 2 {
		t.Fatalf("workspace count = %d, want 2", len(vtui.FrameManager.Screens))
	}
	if got := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].GetTitle(); got != "Two" {
		t.Fatalf("initial active workspace = %q, want Two", got)
	}

	if !HandleSemanticAction(map[string]any{
		"target": "workspace-tab-1",
		"action": "workspace.activate",
		"index":  0,
	}) {
		t.Fatal("workspace switch action was not handled")
	}
	if got := vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx].GetTitle(); got != "One" {
		t.Fatalf("active workspace after switch = %q, want One", got)
	}

	if HandleSemanticAction(map[string]any{
		"target": "workspace-tab-99",
		"action": "workspace.activate",
		"index":  99,
	}) {
		t.Fatal("out-of-range workspace switch was accepted")
	}
}

func TestToastDismissSemanticAction(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.ShowToast("PlugRing: 1 plugin update available!", time.Minute)
	deadline := time.After(time.Second)
	for vtui.FrameManager.GetActiveToast() == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("toast was not shown")
		}
	}

	if !HandleSemanticAction(map[string]any{
		"action": "toast.dismiss",
		"target": "toast",
	}) {
		t.Fatal("toast dismiss action was not handled")
	}
	if got := vtui.FrameManager.GetActiveToast(); got != "" {
		t.Fatalf("toast remained visible after dismiss: %q", got)
	}
}

func TestMenuBarPointerSelectionRejectsStalePopup(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewDesktop())

	menuBar := vtui.NewMenuBar([]string{"&Left", "&Right"})
	menuBar.SetPosition(0, 0, 79, 0)
	menuBar.Items[0].SubItems = []vtui.MenuItem{
		{Text: "First"},
		{Text: "Second"},
	}
	menuBar.Items[1].SubItems = []vtui.MenuItem{
		{Text: "Other first"},
		{Text: "Other second"},
	}
	vtui.FrameManager.MenuBar = menuBar
	menuBar.Active = true
	menuBar.ActivateSubMenu(0)

	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	firstFrame := frames[len(frames)-1]
	firstMenu, _ := nativeui.FrameVMenu(firstFrame)
	if firstMenu == nil {
		t.Fatal("first menu-bar submenu was not materialized")
	}
	firstID := vtui.SemanticID(firstFrame)
	if !HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 0,
		"index":     1,
	}) {
		t.Fatal("current popup hover was rejected")
	}
	if firstMenu.SelectPos != 1 {
		t.Fatalf("first submenu selection = %d, want 1", firstMenu.SelectPos)
	}

	menuBar.ActivateSubMenu(1)
	frames = vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	secondFrame := frames[len(frames)-1]
	secondMenu, _ := nativeui.FrameVMenu(secondFrame)
	if secondMenu == nil {
		t.Fatal("second menu-bar submenu was not materialized")
	}
	if HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 0,
		"index":     0,
	}) {
		t.Fatal("stale hover action unexpectedly changed the active submenu")
	}
	if menuBar.SelectPos != 1 || secondMenu.SelectPos != 0 {
		t.Fatalf("stale hover mutated active menu: bar=%d row=%d",
			menuBar.SelectPos, secondMenu.SelectPos)
	}
	if HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemSelect",
		"target":    firstID,
		"menuIndex": 1,
		"index":     1,
	}) {
		t.Fatal("hover with a replaced popup id was accepted")
	}
}

func TestSemanticRenderedSurfacePreservesColorsAndCursor(t *testing.T) {
	foreground := uint32(0x12abef)
	background := uint32(0x230f41)
	attr := vtui.SetRGBBoth(vtui.ForegroundIntensity|vtui.CommonLvbUnderscore,
		foreground, background)
	rendered := semantic.SemanticRenderSurface(2, 3, 5, 4, func(scr *vtui.ScreenBuf) {
		scr.FillRect(2, 3, 5, 4, ' ', attr)
		scr.Write(2, 3, vtui.StringToCharInfo("test", attr))
		scr.SetCursorPos(4, 3)
		scr.SetCursorVisible(true)
		scr.SetCursorShape(vtui.CursorShapeBlock)
	})

	if len(rendered.Rows) != 2 || len(rendered.Rows[0]) != 1 {
		t.Fatalf("rendered rows = %#v", rendered.Rows)
	}
	run := rendered.Rows[0][0]
	if run.Foreground != "#12abef" || run.Background != "#230f41" {
		t.Fatalf("run colors = foreground %q background %q", run.Foreground, run.Background)
	}
	if !run.Bold || !run.Underline {
		t.Fatalf("run styles were lost: %#v", run)
	}
	if !rendered.CursorVisible || rendered.CursorX != 2 || rendered.CursorY != 0 || rendered.CursorShape != "block" {
		t.Fatalf("cursor = %#v", rendered)
	}
}

func TestSemanticAttrColorHonorsReverse(t *testing.T) {
	attr := vtui.SetRGBBoth(vtui.CommonLvbReverse, 0x102030, 0xa0b0c0)
	if got := semantic.SemanticAttrColor(attr, true); got != "#a0b0c0" {
		t.Fatalf("reversed foreground = %q", got)
	}
	if got := semantic.SemanticAttrColor(attr, false); got != "#102030" {
		t.Fatalf("reversed background = %q", got)
	}
}

func TestEditorMenuBarSemanticClickOpensSubmenu(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	ev := editor.NewEditorView(piecetable.New([]byte("package main\n")), nil, "main.go")
	defer ev.Close()
	ev.SetPosition(0, 1, 79, 23)
	ev.SetVisible(true)
	vtui.FrameManager.Push(ev)

	if !HandleSemanticAction(map[string]any{
		"action": "menuBar.toggle",
		"index":  0,
	}) {
		t.Fatal("editor menu-bar click was not handled")
	}
	if !semantic.Bool(vtui.FrameManager.ExportSemanticScene()["menuBar"].(map[string]any)["active"]) || semantic.Int(vtui.FrameManager.ExportSemanticScene()["menuBar"].(map[string]any)["selected"]) != 0 {
		t.Fatalf("editor menu bar was not activated: active=%v selected=%d",
			semantic.Bool(vtui.FrameManager.ExportSemanticScene()["menuBar"].(map[string]any)["active"]), semantic.Int(vtui.FrameManager.ExportSemanticScene()["menuBar"].(map[string]any)["selected"]))
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	if len(frames) < 2 {
		t.Fatalf("editor submenu was not pushed: %#v", frames)
	}
	if menu, _ := nativeui.FrameVMenu(frames[len(frames)-1]); menu == nil {
		t.Fatalf("top frame is not the editor submenu: %T", frames[len(frames)-1])
	}

	if !HandleSemanticAction(map[string]any{
		"action":    "menuBar.itemActivate",
		"menuIndex": 0,
		// Save, Save As, Switch to Viewer, Quit follow registry order.
		"index": 3,
	}) {
		t.Fatal("editor Exit menu item was not activated")
	}
	frames = vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for _, frame := range frames {
		if frame == ev {
			t.Fatal("editor remained open after activating File > Exit")
		}
	}
}

func TestSettingsSemanticMenuActivationOwnsGoFocusAndScene(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 42)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(vtui.NewDesktop())

	// Keep this route free of a ConPTY: Settings does not need a live
	// filesystem or shell, but PanelsFrame's focus contract does require its
	// ordinary command-line and terminal view objects.
	pf := &panel.PanelsFrame{
		ActiveIdx: 1, WidePanel: -1, ShowPanels: true, ShowKeyBar: true,
		ShowLeftPanel: true, ShowRightPanel: true,
	}
	pf.MenuBar = vtui.NewMenuBar(nil)
	pf.MenuBar.SetOwner(pf)
	pf.MenuBar.Items = pf.BuildMenuItems()
	pf.CmdLine = cmdline.NewCommandLine(">")
	pf.KeyBar = vtui.NewKeyBar()
	pf.KeyBar.SetOwner(pf)
	pf.TermView = terminalpkg.NewTerminalView(100, 40)
	vtui.FrameManager.Push(pf)

	panelSettings, ok := GetAction("Settings.Open")
	if !ok {
		t.Fatal("Settings.Open action is not registered")
	}
	wantLabel := action.PlainLabel(panelSettings.DisplayLabel())
	menuIndex, itemIndex := -1, -1
	for topIndex, top := range pf.GetMenuBar().Items {
		for subIndex, sub := range top.SubItems {
			// Registry-generated menu items invoke actions through OnClick rather
			// than the legacy numeric Command field. Match the user-visible action
			// label so the test exercises the same generated menu as Qt.
			gotLabel := action.PlainLabel(strings.TrimSpace(strings.TrimPrefix(sub.Text, "√")))
			if gotLabel == wantLabel {
				menuIndex, itemIndex = topIndex, subIndex
				break
			}
		}
	}
	if menuIndex < 0 || itemIndex < 0 {
		t.Fatal("Options > Settings is absent from the panels menu")
	}
	if !HandleSemanticAction(map[string]any{
		"action": "menuBar.toggle", "index": menuIndex,
	}) {
		t.Fatal("Options menu was not opened through the semantic route")
	}
	if !HandleSemanticAction(map[string]any{
		"action": "menuBar.itemActivate", "menuIndex": menuIndex,
		"index": itemIndex,
	}) {
		t.Fatal("Settings was not activated through the semantic route")
	}

	dialog, ok := vtui.FrameManager.GetTopFrame().(*settings.Center)
	if !ok || dialog.GetTitle() != i18n.Msg("SettingsCenter.Title") {
		t.Fatalf("Go top frame = %T %q, want Settings dialog",
			vtui.FrameManager.GetTopFrame(), vtui.FrameManager.GetTopFrame().GetTitle())
	}
	if !dialog.IsFocused() || pf.IsFocused() {
		t.Fatalf("Go focus stayed on panels: dialog=%v panels=%v",
			dialog.IsFocused(), pf.IsFocused())
	}
	focused := dialog.GetFocusedItem()
	if focused == nil || !focused.IsFocused() {
		t.Fatalf("Settings has no focused Go control: %T", focused)
	}
	if _, ok := focused.(*vtui.Edit); !ok {
		t.Fatalf("initial Settings focus = %T, want search edit", focused)
	}

	projected, supported := nativeui.BuildAppIncrementalScene(&vtui.SemanticContext{
		Width: 100, Height: 42, ActiveScreen: 0,
	})
	if !supported || projected == nil {
		t.Fatal("Settings is absent from the bounded Go scene")
	}
	dialogs := semantic.AppMapSlice(projected.Scene["dialogs"])
	if len(dialogs) != 1 || semantic.String(dialogs[0]["title"]) != i18n.Msg("SettingsCenter.Title") {
		t.Fatalf("projected dialogs = %#v", dialogs)
	}
	children := semantic.AppMapSlice(dialogs[0]["children"])
	if len(children) == 0 || !semantic.AppBool(children[0]["focused"]) {
		t.Fatalf("projected Settings focus = %#v", children)
	}
}

func TestCommandLineSemanticModelUsesRenderedRunsAndCursor(t *testing.T) {
	cl := cmdline.NewCommandLine(">")
	cl.SetPosition(0, 0, 19, 0)
	cl.InsertString("abc")

	model := cl.SemanticModel(nil)
	if len(model.Runs) == 0 {
		t.Fatal("command line did not export its rendered color runs")
	}
	if !model.CursorVisible {
		t.Fatal("focused command line did not export a visible cursor")
	}
	if len(model.CursorPrefixRuns) == 0 {
		t.Fatal("command line did not export the rendered prefix used to place its cursor")
	}
	if model.CursorX != 4 {
		t.Fatalf("command cursor x=%d, want prompt width 1 + text width 3", model.CursorX)
	}
	if model.InputX != 1 {
		t.Fatalf("command input x=%d, want prompt width 1", model.InputX)
	}
	if model.CursorPosition != 3 {
		t.Fatalf("command text cursor=%d, want UTF-16 position 3", model.CursorPosition)
	}

	cl.Edit.SetText("a😀b")
	model = cl.SemanticModel(nil)
	if model.CursorPosition != 4 {
		t.Fatalf("unicode command cursor=%d, want UTF-16 position 4", model.CursorPosition)
	}
	cl.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_LEFT,
	})
	model = cl.SemanticModel(nil)
	if model.CursorPosition != 3 {
		t.Fatalf("unicode cursor after Left=%d, want UTF-16 position 3", model.CursorPosition)
	}
}

func TestCommandCompletePreservesTextWithoutExplicitSelection(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &panel.PanelsFrame{CmdLine: cmdline.NewCommandLine(">")}
	pf.CmdLine.Edit.SetText("git st")
	if !pf.HandleSemanticAction(map[string]any{"action": "command.complete"}) {
		t.Fatal("command.complete was not handled")
	}
	if got := pf.CmdLine.Edit.GetText(); got != "git st" {
		t.Fatalf("untouched completion changed command to %q", got)
	}

	if !pf.HandleSemanticAction(map[string]any{
		"action": "command.complete",
		"text":   "git status",
	}) {
		t.Fatal("explicit command.complete was not handled")
	}
	if got := pf.CmdLine.Edit.GetText(); got != "git status" {
		t.Fatalf("explicit completion produced %q", got)
	}
}

func TestCommandSubmitWritesPTYAndRevealsTerminal(t *testing.T) {
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(100, 30)
	pf.CmdLine.Edit.SetText("ls")
	pty, ok := pf.Pty.(*paneltest.MockPty)
	if !ok {
		t.Fatalf("test PTY has unexpected type %T", pf.Pty)
	}
	pty.Reset()

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit was not handled")
	}
	if !strings.Contains(pty.String(), "ls") {
		t.Fatalf("command was not written to PTY: %q", pty.String())
	}
	if pf.ShowPanels {
		t.Fatal("command submission did not reveal the terminal")
	}
	if !pf.ReturnToPanels {
		t.Fatal("foreground command would not restore panels on completion")
	}
	if !pf.CmdLine.IsEmpty() {
		t.Fatalf("submitted command line was not cleared: %q", pf.CmdLine.Edit.GetText())
	}
	// ConPTY echoes the command; the managed POSIX shell publishes it at OSC C.
	if runtime.GOOS == "windows" {
		pf.Parser.Process([]byte("ls\r\n"))
	} else {
		pf.Parser.Process([]byte("\x1b]133;C\x07"))
	}
	terminal := pf.TermView.SemanticModel(nil)
	var rendered strings.Builder
	for _, row := range terminal.Rows {
		for _, run := range row.Runs {
			rendered.WriteString(run.Text)
		}
	}
	if !strings.Contains(rendered.String(), "ls") {
		t.Fatalf("submitted command was absent from terminal semantic rows: %q",
			rendered.String())
	}
}

type synchronousCommandEchoPTY struct {
	mockPty
	parser *terminalpkg.AnsiParser
}

func (p *synchronousCommandEchoPTY) Write(data []byte) (int, error) {
	p.mockPty.Write(data)
	// Model the native race: the read goroutine may consume a fragmented shell
	// echo, OSC C, stdout and OSC D before Write returns to PanelsFrame.
	middle := len(data) / 2
	p.parser.Process(data[:middle])
	p.parser.Process(data[middle:])
	p.parser.Process([]byte("\r\n\x1b]133;C\x07F4_OUTPUT_OK\r\n\x1b]133;D\x07"))
	return len(data), nil
}

func TestCommandSubmitPreparesCleanOutputBeforeSynchronousPTYEcho(t *testing.T) {
	path := t.TempDir()
	left := &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(path)}
	right := &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(path)}
	terminal := terminalpkg.NewTerminalView(100, 24)
	pty := &synchronousCommandEchoPTY{}
	parser := terminalpkg.NewAnsiParser(terminal, pty)
	pty.parser = parser
	pf := &panel.PanelsFrame{
		Panels:         [2]panel.Panel{left, right},
		ActiveIdx:      1,
		ShowPanels:     true,
		ShowLeftPanel:  true,
		ShowRightPanel: true,
		CmdLine:        cmdline.NewCommandLine(">"),
		TermView:       terminal,
		Parser:         parser,
		Pty:            pty,
	}
	pf.CmdLine.Edit.SetText("echo F4_USER_COMMAND")

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit was not handled")
	}

	got := string(terminal.GetAllLogBytes())
	if !strings.Contains(got, "echo F4_USER_COMMAND") || !strings.Contains(got, "F4_OUTPUT_OK") {
		t.Fatalf("clean command/output missing after synchronous PTY echo: %q", got)
	}
	for _, technical := range []string{"set +H", "FARVTRESULT", `printf "\033]133`} {
		if strings.Contains(got, technical) {
			t.Fatalf("technical wrapper leaked through pre-Write race (%q): %q", technical, got)
		}
	}
	if terminal.Muted {
		t.Fatalf("managed command did not settle: muted=%v", terminal.Muted)
	}
}

func TestGlobalCommandSubmitBypassesAutocompleteOverlay(t *testing.T) {
	oldFM := *vtui.FrameManager
	defer func() { *vtui.FrameManager = oldFM }()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(100, 30)
	pf.CmdLine.Edit.SetText("ls")
	pty := pf.Pty.(*paneltest.MockPty)
	pty.Reset()
	vtui.FrameManager.Push(pf)
	autocomplete := vtui.NewAutoCompleteMenu(pf.CmdLine.Edit)
	vtui.FrameManager.Push(autocomplete)

	if !HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(pf),
		"action": "command.submit",
	}) {
		t.Fatal("global command.submit was not handled")
	}
	if got := pty.String(); !strings.Contains(got, "ls") {
		t.Fatalf("autocomplete overlay intercepted command submission: %q", got)
	}
	if pf.ShowPanels {
		t.Fatal("submitted command did not reveal terminal")
	}
}

func TestCommandSubmitWithoutPTYKeepsCommandAndPanels(t *testing.T) {
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.PtyMutex.Lock()
	oldPTY := pf.Pty
	pf.Pty = nil
	pf.PtyMutex.Unlock()
	defer func() {
		_ = oldPTY.Close()
		pf.Close()
	}()
	pf.CmdLine.Edit.SetText("ls")

	if !pf.HandleSemanticAction(map[string]any{"action": "command.submit"}) {
		t.Fatal("command.submit without PTY was not consumed")
	}
	if !pf.ShowPanels {
		t.Fatal("missing PTY hid the panels")
	}
	if got := pf.CmdLine.Edit.GetText(); got != "ls" {
		t.Fatalf("missing PTY discarded command %q", got)
	}
}
