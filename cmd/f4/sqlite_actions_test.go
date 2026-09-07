package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// sqliteMenuItem finds the SQLite client entry in a built menu bar.
func sqliteMenuItem(items []vtui.MenuBarItem) (vtui.MenuBarItem, vtui.MenuItem, bool) {
	label := plainLabel(Msg("Action.App.SQLite"))
	for _, bar := range items {
		for _, item := range bar.SubItems {
			if plainLabel(item.Text) == label {
				return bar, item, true
			}
		}
	}
	return vtui.MenuBarItem{}, vtui.MenuItem{}, false
}

// TestSQLiteClientIsInTheCommandsMenuWithItsKey covers what the user actually
// looks for: a row that names the command and the key that runs it.
//
// The second half is the spreadsheet's lesson. Menu items are rebuilt on every
// GetMenuBar call and the open dropdown is the top frame while they are, so a
// menu-visible entry must not depend on what sits on top.
func TestSQLiteClientIsInTheCommandsMenuWithItsKey(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldHotkeys := GlobalHotkeysMgr
	GlobalHotkeysMgr = NewHotkeyManager("")
	t.Cleanup(func() { GlobalHotkeysMgr = oldHotkeys })

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	bar, item, ok := sqliteMenuItem(pf.GetMenuBar().Items)
	if !ok {
		t.Fatal("the SQLite client is missing from the panels menu")
	}
	if commands := plainLabel(Msg("Menu.Shell.Commands")); !strings.Contains(bar.Label, commands) {
		t.Errorf("the SQLite client sits in %q, expected %q", bar.Label, commands)
	}
	if want := FormatKeyForUI("CtrlAltD"); item.Shortcut != want {
		t.Errorf("the SQLite client shows %q, expected %q", item.Shortcut, want)
	}

	popup := vtui.NewVMenu("dropdown")
	popup.SetPosition(1, 1, 20, 10)
	vtui.FrameManager.Push(popup)
	if _, _, ok := sqliteMenuItem(BuildMenuBarItems("Shell")); !ok {
		t.Error("the SQLite client disappeared from the menu while a popup was on top")
	}
}

// TestSQLiteActionReachesThePluginCommand checks both ends of the bridge: no
// plugin means no success and no panic, and a registered plugin is handed the
// panels frame even when a popup is what the command was chosen from.
func TestSQLiteActionReachesThePluginCommand(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	if actionSQLiteClient() {
		t.Error("the action reported success with no SQLite plugin loaded")
	}

	var ran vfs.App
	registration, err := (&coreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       sqlitePluginCommandID,
		Location: vfs.PluginCommandPanel,
		Label:    "SQLite client",
		Run:      func(app vfs.App) { ran = app },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	popup := vtui.NewVMenu("dropdown")
	popup.SetPosition(1, 1, 20, 10)
	vtui.FrameManager.Push(popup)

	if !actionSQLiteClient() {
		t.Fatal("the action did not reach the registered plugin command")
	}
	if ran != vfs.App(pf) {
		t.Errorf("the command ran against %v, expected the panels frame", ran)
	}
}
