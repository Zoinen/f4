package main

import (
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// filterGuiFontDisplayChoices performs the filtering independently of the UI
// so the matching contract stays testable. Matching is case-insensitive and
// accepts the query anywhere in the displayed font name.
func filterGuiFontDisplayChoices(choices []string, query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]string(nil), choices...)
	}

	filtered := make([]string, 0, len(choices))
	for _, choice := range choices {
		if strings.Contains(strings.ToLower(choice), query) {
			filtered = append(filtered, choice)
		}
	}
	return filtered
}

func guiFontComboShouldCloseMenu(text string, filteredCount int) bool {
	return strings.TrimSpace(text) == "" || filteredCount == 0
}

func configureGuiFontCombo(combo *vtui.ComboBox, choices []string) {
	if combo == nil {
		return
	}
	combo.DropdownOnly = false

	setItems := func(query string) []string {
		filtered := filterGuiFontDisplayChoices(choices, query)
		items := make([]vtui.MenuItem, len(filtered))
		for i, choice := range filtered {
			items[i] = vtui.MenuItem{Text: choice}
		}
		combo.Menu.Items = items
		combo.Menu.ItemCount = len(items)
		combo.Menu.SetSelectPos(0)
		return filtered
	}

	setItems("")

	// Keep the normal ComboBox selection behaviour, then restore the complete
	// catalog for the next opening. The selected label remains in the edit
	// field and is converted back to its stored value when the dialog is saved.
	defaultMenuAction := combo.Menu.OnAction
	combo.Menu.OnAction = func(idx int) {
		defaultMenuAction(idx)
		setItems("")
	}

	combo.Edit.OnTextChange = func(text string) {
		filtered := setItems(text)
		if vtui.FrameManager == nil {
			return
		}
		menuIsOpen := vtui.FrameManager.GetTopFrame() == combo.Menu
		if guiFontComboShouldCloseMenu(text, len(filtered)) {
			if menuIsOpen {
				combo.Menu.Close()
			}
			return
		}
		if !menuIsOpen {
			combo.Open()
		}
	}

	// Once the dropdown is open it owns keyboard input. Forward editing keys
	// back to the embedded edit so the user can continue typing and refine the
	// substring filter instead of having the menu's type-ahead consume it.
	combo.Menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		switch e.VirtualKeyCode {
		case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_PRIOR, vtinput.VK_NEXT,
			vtinput.VK_RETURN, vtinput.VK_ESCAPE, vtinput.VK_TAB:
			return false
		}
		return combo.Edit.ProcessKey(e)
	}
}
