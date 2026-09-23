package app

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestNavigationToggleShortcut(t *testing.T) {
	for _, mods := range []vtinput.ControlKeyState{
		vtinput.LeftCtrlPressed, vtinput.RightCtrlPressed,
		vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
		vtinput.RightCtrlPressed | vtinput.ShiftPressed,
	} {
		for _, vk := range []uint16{vtinput.VK_OEM_3, 0} {
			for _, char := range []rune{'`', '~', 'ё'} {
				t.Run(keymap.EventToFarString(&vtinput.InputEvent{VirtualKeyCode: vk, Char: char, ControlKeyState: mods}), func(t *testing.T) {
					original, hotkeys := config.App, keymap.GlobalHotkeysMgr
					t.Cleanup(func() { config.App, keymap.GlobalHotkeysMgr = original, hotkeys })
					config.App.NavigationMode = config.NavigationSearchFirst
					config.App.AutoSaveSettings = false
					config.App.SearchCommandHideUnfocused = true
					config.App.CommandLineAutoComplete = false
					keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
					pf, left, right := newSearchFirstTestFrame(t)
					pf.MenuBar, pf.KeyBar = vtui.NewMenuBar(nil), vtui.NewKeyBar()
					pf.ResizeConsole(80, 25)
					pf.CmdLine.Edit.SetText("retained command")
					left.FastFindMode, left.FastFindStr = true, "b"
					left.SetCursorIndex(1)
					event := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: char, ControlKeyState: mods}
					pressKey(pf, event)
					if config.App.NavigationMode != config.NavigationClassic {
						t.Fatalf("shortcut did not switch to Classic: %+v", event)
					}
					if left.FastFindMode || left.FastFindStr != "" || !pf.CmdLine.IsVisible() || !pf.CmdLine.IsFocused() {
						t.Fatal("Classic must dismiss search and restore visible command input")
					}
					pressKey(pf, event)
					if config.App.NavigationMode != config.NavigationSearchFirst || pf.CommandLineFocused || pf.CmdLine.IsFocused() || pf.CmdLine.IsVisible() {
						t.Fatal("second shortcut must return to search-first panel focus and hidden input")
					}
					if !left.IsFocused() || right.IsFocused() || left.GetSelectedName() != "beta.txt" || pf.CmdLine.Edit.GetText() != "retained command" {
						t.Fatal("toggle lost panel focus, selection, or retained command")
					}
					event.KeyDown = false
					pressKey(pf, event)
					if config.App.NavigationMode != config.NavigationSearchFirst {
						t.Fatal("key release toggled navigation")
					}
				})
			}
		}
	}
}

func TestNavigationToggleActionAndRebinding(t *testing.T) {
	original, hotkeys := config.App, keymap.GlobalHotkeysMgr
	t.Cleanup(func() { config.App, keymap.GlobalHotkeysMgr = original, hotkeys })
	config.App.NavigationMode = config.NavigationVim
	config.App.AutoSaveSettings = false
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	a, ok := GetAction("Panel.ToggleNavigationMode")
	if !ok {
		t.Fatal("navigation toggle is missing from the action registry")
	}
	if a.MenuPath != "Options" || a.Checked == nil || a.LabelKey == "" || a.DescKey == "" || len(a.NativeKeys) != 0 {
		t.Fatal("navigation toggle must be a localized, checked, reassignable menu action")
	}
	pf, _, _ := newSearchFirstTestFrame(t)
	pf.MenuBar, pf.KeyBar = vtui.NewMenuBar(nil), vtui.NewKeyBar()
	vtui.FrameManager.Push(pf)
	if !RunAction(a.Name) || config.App.NavigationMode != config.NavigationSearchFirst || !a.Checked() {
		t.Fatal("macro/palette action must enter Search first from Vim")
	}
	entry, found := commandPaletteTestEntryByID(commandPaletteActionEntries("Shell"), a.Name)
	if !found {
		t.Fatal("navigation toggle missing from command palette")
	}
	for _, query := range []string{"search", "classic", "навигации"} {
		if len(rankCommandPaletteEntries([]commandPaletteEntry{entry}, query, nil)) != 1 {
			t.Errorf("toggle not searchable by %q", query)
		}
	}
	for _, key := range a.DefaultKeys {
		keymap.GlobalHotkeysMgr.Bind("Shell", key, "None")
	}
	keymap.GlobalHotkeysMgr.Bind("Shell", "CtrlF12", a.Name)
	pressKey(pf, keymap.ParseFarKey("CtrlVK_C0"))
	if config.App.NavigationMode != config.NavigationSearchFirst {
		t.Fatal("unbound default still toggles navigation")
	}
	pressKey(pf, keymap.ParseFarKey("CtrlF12"))
	if config.App.NavigationMode != config.NavigationClassic || a.Checked() {
		t.Fatal("reassigned shortcut did not toggle navigation")
	}
}

func TestNavigationToggleUpdatesOtherWorkspaces(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.SearchCommandHideUnfocused = true
	config.App.AutoSaveSettings = false
	pf, _, _ := newSearchFirstTestFrame(t)
	pf.MenuBar, pf.KeyBar = vtui.NewMenuBar(nil), vtui.NewKeyBar()
	vtui.FrameManager.Push(pf)
	other := paneltest.SetupMockPanelsFrame(t)
	t.Cleanup(other.Close)
	other.ResizeConsole(80, 25)
	vtui.FrameManager.AddScreenBackground(other)
	other.SetCommandLineFocus(true)
	other.CmdLine.Edit.SetText("keep me")
	active := other.GetActivePanel()
	active.FastFindMode, active.FastFindStr = true, "old query"
	pf.ToggleNavigationMode()
	if active.FastFindMode || other.CommandLineFocused || !other.CmdLine.IsVisible() || !other.CmdLine.IsFocused() {
		t.Fatal("background workspace retained search-first state")
	}
	pf.ToggleNavigationMode()
	if other.CmdLine.IsVisible() || other.CmdLine.IsFocused() || !active.IsFocused() || other.CmdLine.Edit.GetText() != "keep me" {
		t.Fatal("background workspace did not acquire search-first panel focus")
	}
	// A workspace displaying its terminal must keep its input available.
	other.ShowPanels = false
	other.CmdLine.SetVisible(true)
	pf.ToggleNavigationMode()
	pf.ToggleNavigationMode()
	if !other.CmdLine.IsFocused() || !other.CmdLine.IsVisible() {
		t.Fatal("navigation mode stole background terminal input")
	}
	before := config.App.NavigationMode
	other.ToggleNavigationMode()
	if config.App.NavigationMode != before {
		t.Fatal("terminal workspace must not toggle panel navigation")
	}
}

func TestNavigationToggleRightCtrlBindingPrecedence(t *testing.T) {
	manager := keymap.NewHotkeyManager("")
	for _, key := range []string{"RCtrlVK_C0", "RCtrlShiftVK_C0"} {
		if got := keymap.ConfiguredHotkeyAction(manager, "Shell", key); got != "Panel.ToggleNavigationMode" {
			t.Fatalf("%s resolved to %q", key, got)
		}
	}
	if keymap.ConfigurableHotkeyOwnsPanelBookmark(manager, "Shell", keymap.ParseFarKey("CtrlAltVK_C0")) {
		t.Fatal("Ctrl+Alt+grave must retain the home bookmark shortcut")
	}
	manager.Bind("Shell", "RCtrlVK_C0", "Panel.GoRoot")
	if got := keymap.ConfiguredHotkeyAction(manager, "Shell", "RCtrlVK_C0"); got != "Panel.GoRoot" {
		t.Fatalf("explicit physical Control binding lost: %q", got)
	}
	manager.Bind("Shell", "CtrlVK_C0", "None")
	manager.Unbind("Shell", "RCtrlVK_C0")
	if got := keymap.ConfiguredHotkeyAction(manager, "Shell", "RCtrlVK_C0"); got != "None" {
		t.Fatalf("unbinding Ctrl did not silence physical Control: %q", got)
	}
}

func TestNavigationToggleFromCommandFocusRestoresAltPanel(t *testing.T) {
	original := config.App
	t.Cleanup(func() { config.App = original })
	config.App.NavigationMode = config.NavigationSearchFirst
	config.App.AutoSaveSettings = false
	pf, left, _ := newSearchFirstTestFrame(t)
	pf.MenuBar, pf.KeyBar = vtui.NewMenuBar(nil), vtui.NewKeyBar()
	info := panel.NewInfoPanel(left)
	pf.AltPanels[0] = info
	pf.SetCommandLineFocus(true)
	pf.ToggleNavigationMode()
	if !info.IsFocused() || !pf.CmdLine.IsFocused() || pf.CommandLineFocused {
		t.Fatal("Classic must restore active alt-panel and command-line focus")
	}
}
