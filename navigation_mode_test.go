package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestPanelNavigationModeConfigRoundTripAndMigration(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "settings.ini")
	origUserPath := getUserConfigIniPath
	origPaths := getConfigIniPaths
	oldCfg := AppConfig
	defer func() {
		getUserConfigIniPath = origUserPath
		getConfigIniPaths = origPaths
		AppConfig = oldCfg
	}()
	getUserConfigIniPath = func() string { return iniPath }
	getConfigIniPaths = func() []string { return []string{iniPath} }

	AppConfig.NavigationMode = NavigationSearchFirst
	AppConfig.SearchCommandStayFocused = true
	SaveConfig()
	body, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NavigationMode = search", "SearchCommandStayFocused = 1", "VimHotkeys = 0"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("saved config missing %q:\n%s", want, body)
		}
	}

	AppConfig.NavigationMode = NavigationClassic
	AppConfig.SearchCommandStayFocused = false
	LoadConfig()
	if AppConfig.NavigationMode != NavigationSearchFirst || !AppConfig.SearchCommandStayFocused {
		t.Fatalf("round trip got mode=%v stay=%v", AppConfig.NavigationMode, AppConfig.SearchCommandStayFocused)
	}

	if err := os.WriteFile(iniPath, []byte("[Panel]\nVimHotkeys = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	LoadConfig()
	if AppConfig.NavigationMode != NavigationVim {
		t.Fatalf("legacy VimHotkeys migration got %v", AppConfig.NavigationMode)
	}

	if err := os.WriteFile(iniPath, []byte("[Panel]\nNavigationMode = classic\nVimHotkeys = 1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	LoadConfig()
	if AppConfig.NavigationMode != NavigationClassic {
		t.Fatalf("NavigationMode must override legacy VimHotkeys, got %v", AppConfig.NavigationMode)
	}
}

func newSearchFirstTestFrame(t *testing.T) (*PanelsFrame, *FileSystemPanel, *FileSystemPanel) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	left := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	right := NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	left.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "alpha.txt"}}, {VFSItem: vfs.VFSItem{Name: "beta.txt"}}}
	right.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "right.txt"}}}
	left.Refresh()
	right.Refresh()
	pf := &PanelsFrame{
		panels:         [2]Panel{left, right},
		activeIdx:      0,
		showPanels:     true,
		showLeftPanel:  true,
		showRightPanel: true,
		showKeyBar:     true,
		lastW:          80,
		lastH:          25,
		cmdLine:        NewCommandLine("$ "),
		termView:       NewTerminalView(80, 24),
	}
	pf.cmdLine.SetPosition(0, 23, 79, 23)
	pf.applyNavigationMode()
	return pf, left, right
}

func TestSearchFirstKeyboardRoutingAndFocusToggle(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst
	AppConfig.CommandLineAutoComplete = false

	pf, left, _ := newSearchFirstTestFrame(t)
	if pf.commandLineFocused || pf.cmdLine.IsFocused() || !left.IsFocused() {
		t.Fatal("search-first must start with panel focus")
	}

	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'b', VirtualKeyCode: 'B'})
	if !left.fastFindMode || left.fastFindStr != "b" || left.GetSelectedName() != "beta.txt" {
		t.Fatalf("plain input did not start fast find: mode=%v text=%q selected=%q", left.fastFindMode, left.fastFindStr, left.GetSelectedName())
	}

	grave := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ё', VirtualKeyCode: vtinput.VK_OEM_3}
	pf.ProcessKey(grave)
	if !pf.commandLineFocused || !pf.cmdLine.IsFocused() || left.IsFocused() || left.fastFindMode {
		t.Fatal("grave key did not move focus to command line and close fast find")
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'x', VirtualKeyCode: 'X'})
	if got := pf.cmdLine.Edit.GetText(); got != "x" {
		t.Fatalf("command input got %q, want x", got)
	}

	pf.ProcessKey(grave)
	if pf.commandLineFocused || pf.cmdLine.Edit.GetText() != "x" {
		t.Fatal("second grave must return to panel without clearing command text")
	}
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if got := pf.cmdLine.Edit.GetText(); got != "x" {
		t.Fatalf("panel Enter executed retained command text: %q", got)
	}
}

func TestSearchFirstFocusToggleAcceptsGUITextOnlyGraveEvents(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst

	for _, char := range []rune{'`', 'ё'} {
		pf, _, _ := newSearchFirstTestFrame(t)
		event := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char}
		if !pf.ProcessKey(event) || !pf.commandLineFocused {
			t.Fatalf("text-only %q event did not focus command line", char)
		}
		if !pf.ProcessKey(event) || pf.commandLineFocused {
			t.Fatalf("second text-only %q event did not restore panel focus", char)
		}
		if !pf.cmdLine.IsEmpty() {
			t.Fatalf("toggle character %q leaked into command line", char)
		}
	}
}

func TestSearchFirstAltGraveInsertsBacktickInCommandFocus(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst

	events := []*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_OEM_3, ControlKeyState: vtinput.LeftAltPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: '`', ControlKeyState: vtinput.LeftAltPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ё', ControlKeyState: vtinput.LeftAltPressed},
	}
	for _, event := range events {
		pf, _, _ := newSearchFirstTestFrame(t)
		pf.setCommandLineFocus(true)
		pf.cmdLine.Edit.SetText("echo ")
		if !pf.ProcessKey(event) {
			t.Fatalf("Alt+grave event was not handled: %+v", event)
		}
		if got := pf.cmdLine.Edit.GetText(); got != "echo `" {
			t.Fatalf("Alt+grave inserted %q, want %q", got, "echo `")
		}
		if !pf.commandLineFocused {
			t.Fatal("Alt+grave unexpectedly moved focus out of command line")
		}
	}
}

func TestSearchFirstCommandEnterPolicyAndTab(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst
	AppConfig.CommandLineAutoComplete = false

	pf, _, _ := newSearchFirstTestFrame(t)
	pf.setCommandLineFocus(true)
	pf.cmdLine.Edit.SetText("exit")
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if pf.commandLineFocused {
		t.Fatal("default Enter policy must return focus to panel")
	}

	AppConfig.SearchCommandStayFocused = true
	pf.setCommandLineFocus(true)
	pf.cmdLine.Edit.SetText("exit")
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if !pf.commandLineFocused {
		t.Fatal("stay-focused policy lost command-line focus")
	}
	active := pf.activeIdx
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.activeIdx != active {
		t.Fatal("Tab in command focus switched panels")
	}
	pf.setCommandLineFocus(false)
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.activeIdx == active {
		t.Fatal("Tab in panel focus did not switch panels")
	}
}

func TestSearchFirstHistoryAndPromptFocusColors(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst

	pf, _, _ := newSearchFirstTestFrame(t)
	inactivePrompt := pf.buildPrompt()
	if len(inactivePrompt) == 0 {
		t.Fatal("inactive prompt is empty")
	}
	for _, cell := range inactivePrompt {
		if cell.Char != vtui.WideCharFiller && cell.Attributes != vtui.Palette[ColCommandLineInactivePrompt] {
			t.Fatal("panel-focused prompt contains an active color")
		}
	}

	pf.setCommandLineFocus(true)
	activePrompt := pf.buildPrompt()
	allInactive := true
	for _, cell := range activePrompt {
		if cell.Char != vtui.WideCharFiller && cell.Attributes != vtui.Palette[ColCommandLineInactivePrompt] {
			allInactive = false
			break
		}
	}
	if allInactive {
		t.Fatal("command-focused prompt did not restore active colors")
	}

	pf.cmdLine.Edit.History = []string{"previous command"}
	pf.cmdLine.Edit.HistoryPos = -1
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if got := pf.cmdLine.Edit.GetText(); got != "previous command" {
		t.Fatalf("Up in command focus did not navigate history: %q", got)
	}
}

func TestSearchFirstMouseFocusAndInactiveCursor(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	AppConfig.NavigationMode = NavigationSearchFirst
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()

	pf, left, _ := newSearchFirstTestFrame(t)
	pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 2, MouseY: 23, ButtonState: vtinput.FromLeft1stButtonPressed})
	if !pf.commandLineFocused {
		t.Fatal("click on command row did not focus command line")
	}
	pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 2, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed})
	if pf.commandLineFocused {
		t.Fatal("click on panel did not restore panel focus")
	}

	pf.setCommandLineFocus(true)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	left.Show(scr)
	found := false
	for y := left.Y1; y <= left.Y2; y++ {
		for x := left.X1; x <= left.X2; x++ {
			cell := scr.GetCell(x, y)
			if cell.Char == 'a' && vtui.GetRGBBack(cell.Attributes) == vtui.GetRGBBack(vtui.Palette[ColPanelInactiveCursor]) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("active panel cursor was not rendered with inactive background")
	}
}

func TestDetailedHorizontalArrowsMatchPageNavigationExceptVim(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	fp := NewFileSystemPanel(0, 0, 50, 20, vfs.NewOSVFS(t.TempDir()))
	fp.SetViewMode(ViewModeDetailed)
	fp.entries = make([]*fileEntry, 60)
	for i := range fp.entries {
		fp.entries[i] = &fileEntry{VFSItem: vfs.VFSItem{Name: "item"}}
	}
	fp.Refresh()

	key := func(vk uint16) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	}

	AppConfig.NavigationMode = NavigationClassic
	fp.SetCursorIndex(4)
	fp.ProcessKey(key(vtinput.VK_RIGHT))
	rightPos := fp.GetCursorIndex()
	fp.SetCursorIndex(4)
	fp.ProcessKey(key(vtinput.VK_NEXT))
	if got := fp.GetCursorIndex(); rightPos != got {
		t.Fatalf("Detailed Right moved to %d, Page Down moved to %d", rightPos, got)
	}

	fp.SetCursorIndex(40)
	fp.ProcessKey(key(vtinput.VK_LEFT))
	leftPos := fp.GetCursorIndex()
	fp.SetCursorIndex(40)
	fp.ProcessKey(key(vtinput.VK_PRIOR))
	if got := fp.GetCursorIndex(); leftPos != got {
		t.Fatalf("Detailed Left moved to %d, Page Up moved to %d", leftPos, got)
	}

	AppConfig.NavigationMode = NavigationVim
	fp.SetCursorIndex(20)
	if fp.ProcessKey(key(vtinput.VK_RIGHT)) || fp.GetCursorIndex() != 20 {
		t.Fatal("Vim mode must retain the previous Detailed Right behavior")
	}
}

func TestDetailedArrowRoutingByNavigationFocus(t *testing.T) {
	oldCfg := AppConfig
	defer func() { AppConfig = oldCfg }()

	key := func(vk uint16) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	}
	typeChar := func(r rune) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, VirtualKeyCode: uint16(r)}
	}

	AppConfig.NavigationMode = NavigationClassic
	pf, left, _ := newSearchFirstTestFrame(t)
	left.SetViewMode(ViewModeDetailed)
	left.SetCursorIndex(0)
	pf.cmdLine.Edit.SetText("abcd")
	pf.ProcessKey(key(vtinput.VK_LEFT))
	pf.ProcessKey(typeChar('X'))
	if got := pf.cmdLine.Edit.GetText(); got != "abcXd" {
		t.Fatalf("Classic non-empty command line did not own Left: %q", got)
	}
	if left.GetCursorIndex() != 0 {
		t.Fatal("Classic command-line Left moved the panel cursor")
	}

	pf.cmdLine.Clear()
	pf.ProcessKey(key(vtinput.VK_RIGHT))
	if left.GetCursorIndex() == 0 {
		t.Fatal("Classic empty command line did not page the Detailed panel")
	}

	AppConfig.NavigationMode = NavigationSearchFirst
	pf.applyNavigationMode()
	left.SetCursorIndex(0)
	pf.ProcessKey(key(vtinput.VK_RIGHT))
	if left.GetCursorIndex() == 0 {
		t.Fatal("Search-first panel focus did not page the Detailed panel")
	}

	panelPos := left.GetCursorIndex()
	pf.setCommandLineFocus(true)
	pf.cmdLine.Edit.SetText("abcd")
	pf.ProcessKey(key(vtinput.VK_LEFT))
	pf.ProcessKey(typeChar('X'))
	if got := pf.cmdLine.Edit.GetText(); got != "abcXd" {
		t.Fatalf("Search-first command focus did not own Left: %q", got)
	}
	if left.GetCursorIndex() != panelPos {
		t.Fatal("Search-first command focus moved the panel cursor")
	}
}
