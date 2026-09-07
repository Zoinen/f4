package main

import (
	"github.com/unxed/f4/vfs"
)

// sqlitePluginCommandID mirrors the ID plugins/sqlite registers with.
//
// The plugin owns the command; this file only gives it a key and a menu row.
// Bindings resolve through the action registry (hotkeys.ini -> action name ->
// RunAction), so a plugin command cannot claim one on its own: its Shortcut
// field is display-only metadata, as vfs.PluginCommand says.
const sqlitePluginCommandID = "f4.sqlite.open"

// actionSQLiteClient opens the SQLite client on the database under the panel
// cursor.
//
// The panels frame is found by walking the frame stack rather than by asking
// for the top frame: when this runs from the menu or from the command palette,
// that popup is what sits on top.
//
// False means the SQLite plugin is not loaded at all. When it is, it is the
// plugin that says the cursor is not on a database, and it can be specific
// about which files it accepts.
func actionSQLiteClient() bool {
	pf := findPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return executeRegisteredPluginCommand(vfs.PluginCommandPanel, sqlitePluginCommandID, pf)
}

func init() {
	RegisterAction(Action{
		Name:        "App.SQLite",
		Area:        "Shell",
		Label:       "SQLite &client",
		LabelKey:    "Action.App.SQLite",
		Description: "Open the SQLite client on the database under the cursor",
		DescKey:     "Action.App.SQLite.Desc",
		// D for database, next to the spreadsheet's Ctrl+Alt+S. Nothing in
		// the registry claims it: Ctrl+Alt is otherwise taken by A, Ins, L,
		// M, P, S and the digits that switch workspaces and jump to
		// bookmarks. What the registry cannot know is that Ctrl+Alt is AltGr
		// on many keyboard layouts, where the combination composes a
		// character and never reaches us; rebinding in hotkeys.ini remains
		// the answer there.
		DefaultKeys: []string{"CtrlAltD"},
		MenuPath:    "Commands",
		// Deliberately no Visible predicate. It is asked every time the menu
		// is built, and a row that comes and goes with the panel cursor is
		// what made this command impossible to find.
		Handler: actionSQLiteClient,
	})
}
