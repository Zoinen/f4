package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The palette lists every command but gave no way to put a key on one of
// them (#1836): the key had to be found again in the hotkey list of the
// settings. Ctrl+K on a result now opens the same area and key dialogs that
// list uses, for the command under the cursor.

// commandPaletteAssignKey reports whether event is the chord that assigns a
// key to the selected command.
func commandPaletteAssignKey(event *vtinput.InputEvent) bool {
	const ctrl = vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed
	const other = vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.ShiftPressed
	return event.KeyDown && event.VirtualKeyCode == vtinput.VK_K &&
		event.ControlKeyState&ctrl != 0 && event.ControlKeyState&other == 0
}

// commandPaletteAssignTarget is the action a key can be bound to for entry,
// and the area the key dialog offers first. Entries that run a stored
// function rather than an action (workspaces, drives, macros) have none.
func commandPaletteAssignTarget(entry commandPaletteEntry) (act action.Action, ok bool) {
	switch entry.source {
	case commandPaletteSourceAction:
		return GetAction(entry.ID)
	case commandPaletteSourcePlugin:
		if entry.ID == "" {
			return action.Action{}, false
		}
		return GetAction(keymap.PluginCommandActionName(entry.ID))
	}
	return action.Action{}, false
}

func (d *commandPaletteDialog) assignKeyToSelected() {
	if d == nil || d.table == nil {
		return
	}
	index := d.table.SelectPos
	if index < 0 || index >= len(d.filtered) {
		return
	}
	entry := d.filtered[index]
	act, ok := commandPaletteAssignTarget(entry)
	hm := keymap.GlobalHotkeysMgr
	if !ok || hm == nil {
		vtui.ShowMessageOn(d.Window, i18n.Msg("CommandPalette.Title"),
			i18n.Msg("CommandPalette.AssignUnavailable"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	showAreaSelectDialog(hm, act.Name, act.Area, "", func() {
		hm.Save()
		d.reloadAfterAssign(entry.Key)
	})
}

// reloadAfterAssign rebuilds the rows, so the new key shows in the Shortcut
// column, and puts the cursor back on the command it was on.
func (d *commandPaletteDialog) reloadAfterAssign(key string) {
	if d.rebuild != nil {
		d.entries = d.rebuild()
	}
	d.refilter(d.query.GetText())
	for i, entry := range d.filtered {
		if entry.Key == key {
			d.table.SetSelectPos(i)
			d.refreshDescription()
			break
		}
	}
}
