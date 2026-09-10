package app

import (
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestPanelNavigationModeConfigRoundTripAndMigration(t *testing.T) {
	tmpDir := t.TempDir()
	iniPath := filepath.Join(tmpDir, "settings.ini")
	origUserPath := config.GetUserConfigIniPath
	origPaths := config.GetConfigIniPaths
	oldCfg := config.App
	defer func() {
		config.GetUserConfigIniPath = origUserPath
		config.GetConfigIniPaths = origPaths
		config.App = oldCfg
	}()
	config.GetUserConfigIniPath = func() string { return iniPath }
	config.GetConfigIniPaths = func() []string { return []string{iniPath} }

	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandStayFocused = true
	config.SaveConfig()
	body, err := os.ReadFile(iniPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NavigationMode = search", "SearchCommandStayFocused = 1", "VimHotkeys = 0"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("saved config missing %q:\n%s", want, body)
		}
	}

	config.App.NavigationMode = config.NavigationClassic
	config.App.SearchCommandStayFocused = false
	config.LoadConfig()
	if config.App.NavigationMode != config.NavigationSearchFirst || !config.App.SearchCommandStayFocused {
		t.Fatalf("round trip got mode=%v stay=%v", config.App.NavigationMode, config.App.SearchCommandStayFocused)
	}

	if err := os.WriteFile(iniPath, []byte("[Panel]\nVimHotkeys = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config.LoadConfig()
	if config.App.NavigationMode != config.NavigationVim {
		t.Fatalf("legacy VimHotkeys migration got %v", config.App.NavigationMode)
	}

	if err := os.WriteFile(iniPath, []byte("[Panel]\nNavigationMode = classic\nVimHotkeys = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	config.LoadConfig()
	if config.App.NavigationMode != config.NavigationClassic {
		t.Fatalf("NavigationMode must override legacy VimHotkeys, got %v", config.App.NavigationMode)
	}
}

func newSearchFirstTestFrame(t *testing.T) (*panel.PanelsFrame, *panel.FileSystemPanel, *panel.FileSystemPanel) {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	left := panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	paneltest.WaitForLoad(t, left)
	right := panel.NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	paneltest.WaitForLoad(t, right)
	left.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "alpha.txt"}}, {VFSItem: vfs.VFSItem{Name: "beta.txt"}}}
	right.Entries = []*panel.FileEntry{{VFSItem: vfs.VFSItem{Name: "right.txt"}}}
	left.Refresh()
	right.Refresh()
	pf := &panel.PanelsFrame{
		Panels:         [2]panel.Panel{left, right},
		ActiveIdx:      0,
		ShowPanels:     true,
		ShowLeftPanel:  true,
		ShowRightPanel: true,
		ShowKeyBar:     true,
		LastW:          80,
		LastH:          25,
		CmdLine:        cmdline.NewCommandLine("$ "),
		TermView:       terminal.NewTerminalView(80, 24),
	}
	pf.CmdLine.SetPosition(0, 23, 79, 23)
	pf.ApplyNavigationMode()
	t.Cleanup(pf.Close)
	return pf, left, right
}

func TestSearchFirstKeyboardRoutingAndFocusToggle(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.CommandLineAutoComplete = false

	pf, left, _ := newSearchFirstTestFrame(t)
	if pf.CommandLineFocused || pf.CmdLine.IsFocused() || !left.IsFocused() {
		t.Fatal("search-first must start with panel focus")
	}

	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'b', VirtualKeyCode: 'B'})
	if !left.FastFindMode || left.FastFindStr != "b" || left.GetSelectedName() != "beta.txt" {
		t.Fatalf("plain input did not start fast find: mode=%v text=%q selected=%q", left.FastFindMode, left.FastFindStr, left.GetSelectedName())
	}

	grave := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ё', VirtualKeyCode: vtinput.VK_OEM_3}
	pressKey(pf, grave)
	if !pf.CommandLineFocused || !pf.CmdLine.IsFocused() || left.IsFocused() || left.FastFindMode {
		t.Fatal("grave key did not move focus to command line and close fast find")
	}
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'x', VirtualKeyCode: 'X'})
	if got := pf.CmdLine.Edit.GetText(); got != "x" {
		t.Fatalf("command input got %q, want x", got)
	}

	pressKey(pf, grave)
	if pf.CommandLineFocused || pf.CmdLine.Edit.GetText() != "x" {
		t.Fatal("second grave must return to panel without clearing command text")
	}
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if got := pf.CmdLine.Edit.GetText(); got != "x" {
		t.Fatalf("panel Enter executed retained command text: %q", got)
	}
}

func TestSearchFirstFastFindCtrlEnterNavigation(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.CommandLineAutoComplete = false

	pf, left, _ := newSearchFirstTestFrame(t)
	left.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "alpha.txt"}},
		{VFSItem: vfs.VFSItem{Name: "beta.txt"}},
		{VFSItem: vfs.VFSItem{Name: "bravo.txt"}},
	}
	left.Refresh()

	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'b', VirtualKeyCode: 'B'})
	if got := left.GetSelectedName(); got != "beta.txt" {
		t.Fatalf("initial Fast Find selected %q, want beta.txt", got)
	}

	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if got := left.GetSelectedName(); got != "bravo.txt" {
		t.Fatalf("Ctrl+Enter selected %q, want bravo.txt", got)
	}
	if !left.FastFindMode || left.FastFindStr != "b" || !pf.CmdLine.IsEmpty() {
		t.Fatal("Ctrl+Enter must keep Fast Find active without changing the command line")
	}

	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})
	if got := left.GetSelectedName(); got != "beta.txt" {
		t.Fatalf("Ctrl+Shift+Enter selected %q, want beta.txt", got)
	}

	pf.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	})
	if left.FastFindMode || left.FastFindStr != "" {
		t.Fatal("Esc must close Fast Find")
	}
	if !pf.ShowPanels {
		t.Fatal("Esc used to close Fast Find must not hide the panels")
	}
}

func TestClassicFastFindEscapeDoesNotHidePanels(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationClassic
	config.App.EscTogglePanels = true

	pf, left, _ := newSearchFirstTestFrame(t)
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'b',
		VirtualKeyCode:  'B',
		ControlKeyState: vtinput.LeftAltPressed,
	})
	if !left.FastFindMode {
		t.Fatal("Alt+B did not start Fast Find in classic navigation")
	}

	escapeDown := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_ESCAPE,
	}
	pf.ProcessKey(escapeDown)
	if left.FastFindMode || left.FastFindStr != "" {
		t.Fatal("Esc did not close classic Fast Find")
	}
	if !pf.ShowPanels {
		t.Fatal("Esc used to close classic Fast Find hid the panels")
	}
}

func TestClassicFastFindF2TogglesAnywhereMatching(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationClassic

	pf, left, _ := newSearchFirstTestFrame(t)
	left.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "inside-target.txt"}},
		{VFSItem: vfs.VFSItem{Name: "target-prefix.txt"}},
	}
	left.Refresh()
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            't',
		VirtualKeyCode:  'T',
		ControlKeyState: vtinput.LeftAltPressed,
	})
	if got := left.GetSelectedName(); got != "target-prefix.txt" {
		t.Fatalf("prefix Fast Find selected %q, want target-prefix.txt", got)
	}

	f2 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2}
	if !pf.ProcessKey(f2) || left.FastFindStr != "*t" {
		t.Fatal("F2 did not enable anywhere matching in Fast Find")
	}
	if got := left.GetSelectedName(); got != "target-prefix.txt" {
		t.Fatalf("anywhere Fast Find moved away from current match to %q", got)
	}
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if got := left.GetSelectedName(); got != "inside-target.txt" {
		t.Fatalf("next anywhere match selected %q, want inside-target.txt", got)
	}
	if !pf.ProcessKey(f2) || left.FastFindStr != "t" {
		t.Fatal("second F2 did not restore prefix matching")
	}
	if got := left.GetSelectedName(); got != "target-prefix.txt" {
		t.Fatalf("restored prefix Fast Find selected %q, want target-prefix.txt", got)
	}

	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	left.SetCursorIndex(0)
	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            '*',
		ControlKeyState: vtinput.LeftAltPressed,
	})
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 't', VirtualKeyCode: 'T'})
	if left.FastFindStr != "*t" {
		t.Fatalf("manually entered anywhere query = %q, want *t", left.FastFindStr)
	}
	if got := left.GetSelectedName(); got != "inside-target.txt" {
		t.Fatalf("manual leading star selected %q, want inside-target.txt", got)
	}
}

func TestSearchFirstFocusToggleAcceptsGUITextOnlyGraveEvents(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst

	for _, char := range []rune{'`', 'ё'} {
		pf, _, _ := newSearchFirstTestFrame(t)
		event := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char}
		if !pressKey(pf, event) || !pf.CommandLineFocused {
			t.Fatalf("text-only %q event did not focus command line", char)
		}
		if !pressKey(pf, event) || pf.CommandLineFocused {
			t.Fatalf("second text-only %q event did not restore panel focus", char)
		}
		if !pf.CmdLine.IsEmpty() {
			t.Fatalf("toggle character %q leaked into command line", char)
		}
	}
}

func TestSearchFirstAltGraveInsertsBacktickInCommandFocus(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst

	events := []*vtinput.InputEvent{
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_OEM_3, ControlKeyState: vtinput.LeftAltPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: '`', ControlKeyState: vtinput.LeftAltPressed},
		{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ё', ControlKeyState: vtinput.LeftAltPressed},
	}
	for _, event := range events {
		pf, _, _ := newSearchFirstTestFrame(t)
		pf.SetCommandLineFocus(true)
		pf.CmdLine.Edit.SetText("echo ")
		if !pressKey(pf, event) {
			t.Fatalf("Alt+grave event was not handled: %+v", event)
		}
		if got := pf.CmdLine.Edit.GetText(); got != "echo `" {
			t.Fatalf("Alt+grave inserted %q, want %q", got, "echo `")
		}
		if !pf.CommandLineFocused {
			t.Fatal("Alt+grave unexpectedly moved focus out of command line")
		}
	}
}

func TestSearchFirstCommandEnterPolicyAndTab(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.CommandLineAutoComplete = false

	pf, _, _ := newSearchFirstTestFrame(t)
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("exit")
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if pf.CommandLineFocused {
		t.Fatal("default Enter policy must return focus to panel")
	}

	config.App.SearchCommandStayFocused = true
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("exit")
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if !pf.CommandLineFocused {
		t.Fatal("stay-focused policy lost command-line focus")
	}
	active := pf.ActiveIdx
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.ActiveIdx != active {
		t.Fatal("Tab in command focus switched panels")
	}
	pf.SetCommandLineFocus(false)
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.ActiveIdx == active {
		t.Fatal("Tab in panel focus did not switch panels")
	}
}

func TestSearchFirstHistoryAndPromptFocusColors(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst

	pf, _, _ := newSearchFirstTestFrame(t)
	inactivePrompt := pf.BuildPrompt()
	if len(inactivePrompt) == 0 {
		t.Fatal("inactive prompt is empty")
	}
	for _, cell := range inactivePrompt {
		if cell.Char != vtui.WideCharFiller && cell.Attributes != vtui.Palette[theme.ColCommandLineInactivePrompt] {
			t.Fatal("panel-focused prompt contains an active color")
		}
	}

	pf.SetCommandLineFocus(true)
	activePrompt := pf.BuildPrompt()
	allInactive := true
	for _, cell := range activePrompt {
		if cell.Char != vtui.WideCharFiller && cell.Attributes != vtui.Palette[theme.ColCommandLineInactivePrompt] {
			allInactive = false
			break
		}
	}
	if allInactive {
		t.Fatal("command-focused prompt did not restore active colors")
	}

	pf.CmdLine.Edit.History = []string{"previous command"}
	pf.CmdLine.Edit.HistoryPos = -1
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if got := pf.CmdLine.Edit.GetText(); got != "previous command" {
		t.Fatalf("Up in command focus did not navigate history: %q", got)
	}
}

func TestSearchFirstMouseFocusAndInactiveCursor(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	pf, left, _ := newSearchFirstTestFrame(t)
	pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 2, MouseY: 23, ButtonState: vtinput.FromLeft1stButtonPressed})
	if !pf.CommandLineFocused {
		t.Fatal("click on command row did not focus command line")
	}
	pf.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 2, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed})
	if pf.CommandLineFocused {
		t.Fatal("click on panel did not restore panel focus")
	}

	pf.SetCommandLineFocus(true)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	left.Show(scr)
	found := false
	for y := left.Y1; y <= left.Y2; y++ {
		for x := left.X1; x <= left.X2; x++ {
			cell := scr.GetCell(x, y)
			if cell.Char == 'a' && vtui.GetRGBBack(cell.Attributes) == vtui.GetRGBBack(vtui.Palette[theme.ColPanelInactiveCursor]) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("active panel cursor was not rendered with inactive background")
	}
}

func TestDetailedHorizontalArrowsMatchPageNavigationExceptVim(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	fp := panel.NewFileSystemPanel(0, 0, 50, 20, vfs.NewOSVFS(t.TempDir()))
	t.Cleanup(func() {
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	paneltest.WaitForLoad(t, fp)
	fp.SetViewMode(panel.ViewModeDetailed)
	fp.Entries = make([]*panel.FileEntry, 60)
	for i := range fp.Entries {
		fp.Entries[i] = &panel.FileEntry{VFSItem: vfs.VFSItem{Name: "item"}}
	}
	fp.Refresh()

	key := func(vk uint16) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	}

	config.App.NavigationMode = config.NavigationClassic
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

	config.App.NavigationMode = config.NavigationVim
	fp.SetCursorIndex(20)
	if fp.ProcessKey(key(vtinput.VK_RIGHT)) || fp.GetCursorIndex() != 20 {
		t.Fatal("Vim mode must retain the previous Detailed Right behavior")
	}
}

func TestDetailedArrowRoutingByNavigationFocus(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()

	key := func(vk uint16) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	}
	typeChar := func(r rune) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, VirtualKeyCode: testutil.Uint16Rune(r)}
	}

	config.App.NavigationMode = config.NavigationClassic
	pf, left, _ := newSearchFirstTestFrame(t)
	left.SetViewMode(panel.ViewModeDetailed)
	left.SetCursorIndex(0)
	pf.CmdLine.Edit.SetText("abcd")
	pressKey(pf, key(vtinput.VK_LEFT))
	pressKey(pf, typeChar('X'))
	if got := pf.CmdLine.Edit.GetText(); got != "abcXd" {
		t.Fatalf("Classic non-empty command line did not own Left: %q", got)
	}
	if left.GetCursorIndex() != 0 {
		t.Fatal("Classic command-line Left moved the panel cursor")
	}

	pf.CmdLine.Clear()
	pressKey(pf, key(vtinput.VK_RIGHT))
	if left.GetCursorIndex() == 0 {
		t.Fatal("Classic empty command line did not page the Detailed panel")
	}

	config.App.NavigationMode = config.NavigationSearchFirst
	pf.ApplyNavigationMode()
	left.SetCursorIndex(0)
	pressKey(pf, key(vtinput.VK_RIGHT))
	if left.GetCursorIndex() == 0 {
		t.Fatal("Search-first panel focus did not page the Detailed panel")
	}

	panelPos := left.GetCursorIndex()
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("abcd")
	pressKey(pf, key(vtinput.VK_LEFT))
	pressKey(pf, typeChar('X'))
	if got := pf.CmdLine.Edit.GetText(); got != "abcXd" {
		t.Fatalf("Search-first command focus did not own Left: %q", got)
	}
	if left.GetCursorIndex() != panelPos {
		t.Fatal("Search-first command focus moved the panel cursor")
	}
}

type searchFirstActivationRenderer struct {
	calls           int
	side            int
	invalidations   int
	activationMenus int
}

func (*searchFirstActivationRenderer) Render([]vtui.CharInfo, []vtui.CharInfo, int, int, bool) {
}

func (*searchFirstActivationRenderer) SetCursor(int, int, bool, vtui.CursorShape) {}

func (*searchFirstActivationRenderer) SetPalette(*[256]uint32) {}

func (*searchFirstActivationRenderer) SetWindowTitle(string) {}

func (*searchFirstActivationRenderer) Flush() {}

func (r *searchFirstActivationRenderer) QueuePanelActivationState(side int, _ string, _ map[string]any) {
	r.calls++
	r.side = side
}

func (r *searchFirstActivationRenderer) InvalidateSemanticSceneUpdate() {
	r.invalidations++
}

func (r *searchFirstActivationRenderer) AllowSemanticMenuAfterPanelActivation() {
	r.activationMenus++
}

func TestSearchFirstPanelTabQueuesDirectActivation(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.CommandLineAutoComplete = false

	pf, _, _ := newSearchFirstTestFrame(t)
	pf.SetCommandLineFocus(false)
	renderer := &searchFirstActivationRenderer{side: -1}
	vtui.FrameManager.Push(pf)
	vtui.FrameManager.Screen().Renderer = renderer

	if !pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB,
	}) {
		t.Fatal("panel-focused search Tab was not handled")
	}
	if renderer.calls != 1 || renderer.side != 1 {
		t.Fatalf("search-mode Tab direct activation = calls %d, side %d; want 1/1",
			renderer.calls, renderer.side)
	}
	if pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_TAB,
	}) {
		t.Fatal("Tab key-up was handled as another panel activation")
	}
	if pf.ActiveIdx != 1 || renderer.calls != 1 {
		t.Fatalf("Tab key-up changed activation = side %d, calls %d; want 1/1",
			pf.ActiveIdx, renderer.calls)
	}
}

func TestSearchFirstSemanticPanelActivationRestoresPanelFocus(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst

	pf, left, right := newSearchFirstTestFrame(t)
	pf.SetCommandLineFocus(true)
	pf.HandleSemanticAction(map[string]any{"action": "panel.activate", "side": 1})

	if pf.ActiveIdx != 1 || pf.CommandLineFocused || pf.CmdLine.IsFocused() {
		t.Fatalf("semantic activation focus = side %d, command %v/%v; want 1/false/false",
			pf.ActiveIdx, pf.CommandLineFocused, pf.CmdLine.IsFocused())
	}
	if left.IsFocused() || !right.IsFocused() {
		t.Fatalf("semantic activation panel focus = left %v, right %v; want false/true",
			left.IsFocused(), right.IsFocused())
	}
}

func TestSearchFirstSemanticPanelActivationNoopDoesNotRedraw(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.NavigationMode = config.NavigationSearchFirst

	pf, left, right := newSearchFirstTestFrame(t)
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	pf.HandleSemanticAction(map[string]any{"action": "panel.activate", "side": 0})
	select {
	case <-vtui.FrameManager.RedrawChan:
		t.Fatal("same-side semantic activation manufactured a redraw")
	default:
	}
	if pf.ActiveIdx != 0 || pf.CommandLineFocused || !left.IsFocused() || right.IsFocused() {
		t.Fatalf("same-side activation changed focus: side=%d command=%v left=%v right=%v",
			pf.ActiveIdx, pf.CommandLineFocused, left.IsFocused(), right.IsFocused())
	}
}
