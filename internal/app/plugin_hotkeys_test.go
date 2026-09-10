package app

import (
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestPluginMenuItemTextPlacesShortcutInLeftColumn(t *testing.T) {
	if got := panel.PluginMenuItemText("SQLite client", "Q", 3); got != "  &Q SQLite client" {
		t.Fatalf("plugin menu item = %q, want left-aligned shortcut", got)
	}
	if got := panel.PluginMenuItemText("Русский плагин", "Ф", 3); got != "  &Ф Русский плагин" {
		t.Fatalf("Cyrillic plugin menu item = %q, want left-aligned shortcut", got)
	}
	if got := panel.PluginMenuItemText("No shortcut", "", 3); got != "    No shortcut" {
		t.Fatalf("plugin menu item without shortcut = %q, want aligned label", got)
	}
}

func TestEventToHotkeyStringSupportsUnicodeLetters(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  *vtinput.InputEvent
		want string
	}{
		{
			name: "text-only terminal input",
			key:  &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ф'},
			want: "Ф",
		},
		{
			name: "Windows key with translated character",
			key:  &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, Char: 'й'},
			want: "Й",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := keymap.EventToHotkeyString(tc.key); got != tc.want {
				t.Fatalf("keymap.EventToHotkeyString = %q, want %q", got, tc.want)
			}
		})
	}
	parsed := keymap.ParseFarKey("Ф")
	if got := keymap.EventToHotkeyString(parsed); got != "Ф" {
		t.Fatalf("Unicode shortcut round trip = %q, want Ф", got)
	}
}

func TestPluginMenuItemShortcutActivatesWithItsCharacter(t *testing.T) {
	restoreManager := paneltest.SwapFrameManager(t)
	defer restoreManager()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	for _, tc := range []struct {
		name string
		text string
		key  rune
	}{
		{name: "Latin", text: panel.PluginMenuItemText("SQLite client", "Q", 1), key: 'q'},
		{name: "Cyrillic", text: panel.PluginMenuItemText("Русский клиент", "Ф", 1), key: 'ф'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			menu := vtui.NewVMenu("Plugins")
			menu.AddItem(vtui.MenuItem{
				Text: tc.text,
				OnClick: func() {
					called = true
				},
			})
			vtui.FrameManager.Push(menu)
			if !menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: tc.key}) {
				t.Fatal("plugin menu did not consume its character hotkey")
			}
			if !called {
				t.Fatal("plugin menu character hotkey did not activate the item")
			}
			menu.Close()
			vtui.FrameManager.Pop()
		})
	}
}

func TestPluginMenuHotkeysAreSingleCharacters(t *testing.T) {
	for _, key := range []string{"Q", "ф", "7"} {
		if !panel.IsPluginMenuHotkey(key) {
			t.Fatalf("%q should be a plugin menu hot key", key)
		}
	}
	for _, key := range []string{"", "Del", "NumDel", "Enter", "CtrlF9", "F4", "Space"} {
		if panel.IsPluginMenuHotkey(key) {
			t.Fatalf("%q must not be a plugin menu hot key", key)
		}
	}
}

func TestPluginHotkeyDialogRejectsDelAndAssignsLetters(t *testing.T) {
	restoreManager := paneltest.SwapFrameManager(t)
	defer restoreManager()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	hm := &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		Defaults: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}

	frame := panel.NewPluginHotkeyAssignFrame(hm, "Plugin.Legacy.0", "Visual File Renamer", nil)
	vtui.FrameManager.Push(frame)
	del := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE}
	if !frame.ProcessKey(del) {
		t.Fatal("plugin hotkey dialog ignored Del")
	}
	if got := hm.Bindings["Shell"]["Del"]; got != "DeleteFiles" {
		t.Fatalf("Del binding = %q, want the built-in action to survive", got)
	}

	frame = panel.NewPluginHotkeyAssignFrame(hm, "Plugin.Legacy.0", "Visual File Renamer", nil)
	vtui.FrameManager.Push(frame)
	if !frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Q, Char: 'q'}) {
		t.Fatal("plugin hotkey dialog ignored a letter")
	}
	if got := hm.Bindings["Shell"]["Q"]; got != "Plugin.Legacy.0" {
		t.Fatalf("assigned plugin hot key = %q, want Plugin.Legacy.0", got)
	}

	// A second letter replaces the first one: a plugin owns one hot key.
	frame = panel.NewPluginHotkeyAssignFrame(hm, "Plugin.Legacy.0", "Visual File Renamer", nil)
	vtui.FrameManager.Push(frame)
	if !frame.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ф'}) {
		t.Fatal("plugin hotkey dialog ignored a Cyrillic letter")
	}
	if _, ok := hm.Bindings["Shell"]["Q"]; ok {
		t.Fatal("the previous plugin hot key was kept")
	}
	if got := hm.Bindings["Shell"]["Ф"]; got != "Plugin.Legacy.0" {
		t.Fatalf("Cyrillic plugin hot key = %q, want Plugin.Legacy.0", got)
	}
}

func TestDeletePluginHotkeyGivesTheKeyBackToItsDefault(t *testing.T) {
	hm := &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{"Shell": {"Del": "Plugin.Legacy.0"}},
		Defaults: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}
	if !keymap.DeletePluginHotkey(hm, "Shell", "Del") {
		t.Fatal("deletePluginHotkey returned false")
	}
	if got := hm.Bindings["Shell"]["Del"]; got != "DeleteFiles" {
		t.Fatalf("Del binding after removal = %q, want DeleteFiles", got)
	}
}

func TestLoadDropsReservedPluginBindings(t *testing.T) {
	dir := t.TempDir()
	iniPath := filepath.Join(dir, "hotkeys.ini")
	ini := "[Shell]\nDel=Plugin.Legacy.0\nQ=Plugin.Legacy.1\n"
	if err := os.WriteFile(iniPath, []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	hm := &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		IniPath:  iniPath,
	}
	hm.Load()
	if got := hm.Bindings["Shell"]["Del"]; got != "DeleteFiles" {
		t.Fatalf("Del binding = %q, want the built-in action back", got)
	}
	if got := hm.Bindings["Shell"]["Q"]; got != "Plugin.Legacy.1" {
		t.Fatalf("letter binding = %q, want it left alone", got)
	}
}

func TestResolvePluginMenuHotkeysKeepsEveryLetterUnique(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{"Shell": {"E": "Plugin.Legacy.3"}},
		Defaults: map[string]map[string]string{},
	}
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })

	entries := []panel.PluginMenuEntry{
		{Label: "Add to archive", ActionName: "Plugin.Legacy.0", Declared: "Shift+F1"},
		{Label: "ID3 Tag &Editor", ActionName: "Plugin.Legacy.1"},
		{Label: "&Environment Manager", ActionName: "Plugin.Legacy.2"},
		{Label: "SQLite client", ActionName: "Plugin.Legacy.3"},
	}
	panel.RefreshPluginMenuEntries(entries)
	if entries[3].Hotkey != "E" {
		t.Fatalf("assigned hot key = %q, want the letter the user picked", entries[3].Hotkey)
	}
	if entries[1].Hotkey != "" || entries[2].Hotkey != "" {
		t.Fatalf("duplicate ampersand hot keys survived: %q and %q", entries[1].Hotkey, entries[2].Hotkey)
	}
	if entries[0].Shortcut() != "Shift+F1" {
		t.Fatalf("declared chord = %q, want Shift+F1", entries[0].Shortcut())
	}
	if got := panel.PluginMenuItemText(entries[2].Label, entries[2].Shortcut(), 1); got != "  Environment Manager" {
		t.Fatalf("deduplicated row = %q, want no accelerator marker", got)
	}
}

func TestPluginChordHotkeyStaysGlobal(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	previousMacro := macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{
			"Common": {
				"CtrlF9": "Plugin.Command.test.global-chord",
				"Q":      "Plugin.Command.test.global-chord",
			},
		},
		Defaults: map[string]map[string]string{},
	}
	macro.MacroMgr = &macro.MacroManager{Macros: make(map[string]map[string][]*vtinput.InputEvent)}
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	called := 0
	registration, err := (&CoreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.global-chord",
		Location: vfs.PluginCommandPanel,
		Label:    "Chord plugin command",
		Run:      func(vfs.App) { called++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	restoreManager := paneltest.SwapFrameManager(t)
	defer restoreManager()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := &panel.PanelsFrame{ShowPanels: true}
	defer testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Frames: []vtui.Frame{pf}}}, 0)()

	chord := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_F9,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	if !macroLookupHotkey(macro.MacroMgr, chord) {
		t.Fatal("a plugin command bound to a chord must still be dispatched")
	}
	if called != 1 {
		t.Fatalf("plugin command calls = %d, want 1", called)
	}

	// The same command reached through a bare letter stays a menu accelerator.
	letter := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_Q,
		Char:           'q',
	}
	if macroLookupHotkey(macro.MacroMgr, letter) {
		t.Fatal("a plugin menu accelerator must stay out of the command line")
	}
	if called != 1 {
		t.Fatalf("plugin command calls = %d, want 1", called)
	}
}

func TestPluginMenuKeyLabelsAdvertiseF4(t *testing.T) {
	labels := panel.PluginMenuKeyLabels(nil)
	if labels == nil || labels.Normal[3] != "F4" {
		t.Fatalf("plugin menu F4 keybar label = %#v, want F4", labels)
	}
}

func TestPanelsFrameDoesNotConsumePluginMenuShortcut(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	previousMacro := macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{
			"Shell": {"Q": "Plugin.Command.test.menu-only"},
		},
		Defaults: map[string]map[string]string{},
	}
	macro.MacroMgr = &macro.MacroManager{Macros: make(map[string]map[string][]*vtinput.InputEvent)}
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})
	called := 0
	registration, err := (&CoreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.menu-only",
		Location: vfs.PluginCommandPanel,
		Label:    "Menu-only plugin command",
		Run:      func(vfs.App) { called++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)
	restoreManager := paneltest.SwapFrameManager(t)
	defer restoreManager()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &panel.PanelsFrame{ShowPanels: true}
	defer testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Frames: []vtui.Frame{pf}}}, 0)()
	e := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_Q,
		Char:           'q',
	}
	if macroFilter(macro.MacroMgr, e) {
		t.Fatal("plugin menu shortcut must remain available to the panel command line")
	}
	if macroLookupHotkey(macro.MacroMgr, e) {
		t.Fatal("injected plugin menu shortcut must remain available to the panel command line")
	}
	if called != 0 {
		t.Fatalf("plugin menu shortcut ran a command %d times", called)
	}
	if pf.InterceptPluginKey(e) {
		t.Fatal("plugin menu shortcut must remain available to the panel command line")
	}
}

func TestDeletePluginHotkeyRemovesBinding(t *testing.T) {
	hm := &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{
			"Shell": {"Q": "Plugin.Command.test.delete"},
		},
		Defaults: map[string]map[string]string{},
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}
	if !keymap.DeletePluginHotkey(hm, "Shell", "Q") {
		t.Fatal("deletePluginHotkey returned false")
	}
	if _, ok := hm.Bindings["Shell"]["Q"]; ok {
		t.Fatal("plugin hotkey binding was not removed")
	}
	if _, err := os.Stat(hm.IniPath); err != nil {
		t.Fatalf("deletePluginHotkey did not persist the change: %v", err)
	}
}

func TestPluginCommandHotkeyUsesConfiguredShortcutAndRunsCommand(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{
			"Shell": {"CtrlF9": "Plugin.Command.test.hotkey"},
		},
		Defaults: map[string]map[string]string{},
	}
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })

	called := 0
	registration, err := (&CoreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.hotkey",
		Location: vfs.PluginCommandPanel,
		Label:    "Plugin hotkey command",
		Shortcut: "F1",
		Run: func(vfs.App) {
			called++
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if got := panel.PluginCommandShortcut(vfs.PluginCommand{ID: "test.hotkey", Shortcut: "F1"}); got != "Ctrl+F9" {
		t.Fatalf("configured plugin shortcut = %q, want Ctrl+F9", got)
	}
	action, ok := GetAction("Plugin.Command.test.hotkey")
	if !ok || action.Label != "Plugin hotkey command" || action.Area != "Shell" {
		t.Fatalf("plugin hotkey action = %#v, registered=%v", action, ok)
	}

	restoreManager := paneltest.SwapFrameManager(t)
	defer restoreManager()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := &panel.PanelsFrame{}
	defer testutil.SetFrameManagerScreens(t, []*vtui.AppScreen{{Frames: []vtui.Frame{pf}}}, 0)()
	if !RunAction(action.Name) {
		t.Fatal("configured plugin action was not dispatched")
	}
	if called != 1 {
		t.Fatalf("plugin command calls = %d, want 1", called)
	}
}

func TestBuildHotkeyRowsIncludesUnassignedPluginCommand(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{},
		Defaults: map[string]map[string]string{},
	}
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })

	registration, err := (&CoreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       "test.hotkey-row",
		Location: vfs.PluginCommandPanel,
		Label:    "Plugin row command",
		Shortcut: "Shift+F6",
		Run:      func(vfs.App) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	for _, row := range buildHotkeyRows(keymap.GlobalHotkeysMgr) {
		if row.Action == "Plugin.Command.test.hotkey-row" {
			if row.Label != "Plugin row command" || row.Key != "Shift+F6" || !row.Editable {
				t.Fatalf("plugin hotkey row = %#v", row)
			}
			return
		}
	}
	t.Fatal("unassigned plugin command is missing from the hotkey dialog")
}
