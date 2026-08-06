package main

import (
	"strings"

	"github.com/unxed/vtui"
)

// BuildMenuBarItems generates the top-level menu structure for an area
// from the action registry. Every action with a MenuPath set appears in
// the corresponding top-level menu; items inside a menu follow
// registration order, with Common-area actions appended after the
// area's own. The shortcut column reflects the *active* bindings,
// including user overrides from hotkeys.ini.
func BuildMenuBarItems(area string) []vtui.MenuBarItem {
	type menu struct {
		title string
		items []vtui.MenuItem
	}
	var order []string
	menus := make(map[string]*menu)

	appendAction := func(a Action) {
		// Asked before the menu is created, so a group whose every action
		// is hidden does not appear as an empty one.
		if a.Visible != nil && !a.Visible() {
			return
		}
		m := menus[a.MenuPath]
		if m == nil {
			title := Msg("Menu." + area + "." + a.MenuPath)
			if strings.HasPrefix(title, "{") {
				title = a.MenuPath
			}
			m = &menu{title: title}
			menus[a.MenuPath] = m
			order = append(order, a.MenuPath)
		}
		if a.MenuSeparatorBefore {
			m.items = append(m.items, vtui.MenuItem{Separator: true})
		}
		text := a.DisplayLabel()
		if !strings.Contains(text, "&") {
			text = "&" + text // first letter becomes the menu hotkey
		}
		if a.Checked != nil && a.Checked() {
			text = "√ " + text
		}
		item := vtui.MenuItem{
			Text:    text,
			OnClick: func() { RunAction(a.Name) },
		}
		if hm := GlobalHotkeysMgr; hm != nil {
			item.Shortcut = FormatKeyForUI(hm.GetKeyForAction(area, a.Name))
		}
		m.items = append(m.items, item)
	}

	// The area's own actions first (stable registry order).
	for _, a := range GetOrderedActions() {
		if a.MenuPath != "" && a.Area == area {
			appendAction(a)
		}
	}
	// Common actions join only menu groups that already exist in the
	// area, so they cannot create stray top-level menus.
	for _, a := range GetOrderedActions() {
		if a.MenuPath != "" && a.Area == "Common" && menus[a.MenuPath] != nil {
			appendAction(a)
		}
	}

	result := make([]vtui.MenuBarItem, 0, len(order))
	for _, path := range order {
		m := menus[path]
		result = append(result, vtui.MenuBarItem{Label: m.title, SubItems: m.items})
	}
	return result
}
