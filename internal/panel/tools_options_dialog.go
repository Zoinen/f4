package panel

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// toolsOptionsList is the list of the tools window: a row per tool with its
// check box. A click on the box or Space turns the tool on or off, a click on
// the name only selects it (f4#918).
type toolsOptionsList struct {
	*vtui.ListBox
	onToggle func(idx int)
	onSelect func(idx int)
}

func (l *toolsOptionsList) ProcessMouse(e *vtinput.InputEvent) bool {
	handled := l.ListBox.ProcessMouse(e)
	if handled && e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed &&
		vtui.IsMousePress(e) && int(e.MouseX) <= l.X1+4 && l.hasRowAt(l.SelectPos) {
		l.onToggle(l.SelectPos)
	}
	if l.onSelect != nil {
		l.onSelect(l.SelectPos)
	}
	return handled
}

func (l *toolsOptionsList) ProcessKey(e *vtinput.InputEvent) bool {
	handled := l.ListBox.ProcessKey(e)
	if l.onSelect != nil {
		l.onSelect(l.SelectPos)
	}
	return handled
}

func (l *toolsOptionsList) hasRowAt(idx int) bool { return idx >= 0 && idx < len(l.Items) }

// toolsOptionsRowText is a row as the window shows it.
func toolsOptionsRowText(shown bool, name string) string {
	if shown {
		return "[x] " + name
	}
	return "[ ] " + name
}

// ShowToolsOptions opens the window of the F11 menu's tools: every entry with a
// check box that shows or hides it at once, and a Settings button that is live
// when the selected tool has settings of its own (f4#918).
func (pf *PanelsFrame) ShowToolsOptions() {
	entries := PluginMenuEntriesSnapshot()
	if len(entries) == 0 {
		vtui.ShowMessage(i18n.Msg("Plugins.ToolsOptionsTitle"), i18n.Msg("Plugins.ToolsOptionsEmpty"), []string{i18n.Msg("vtui.Ok")})
		return
	}
	hidden := hiddenPluginMenuEntries()

	names := make([]string, len(entries))
	width := runewidth.StringWidth(i18n.Msg("Plugins.ToolsOptionsTitle")) + 6
	for i, entry := range entries {
		names[i] = action.PlainLabel(entry.Label)
		if w := runewidth.StringWidth(names[i]) + 4 + 4; w > width {
			width = w
		}
	}
	settingsLabel := i18n.Msg("Plugins.ToolsOptionsSettings")
	if w := runewidth.StringWidth(settingsLabel) + 8; w > width {
		width = w
	}
	width = max(width, 30)
	screenW, screenH := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	if screenW > 0 && width > screenW-2 {
		width = screenW - 2
	}
	listH := len(entries)
	if screenH > 0 && listH > screenH-8 {
		listH = max(3, screenH-8)
	}
	height := listH + 6

	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("Plugins.ToolsOptionsTitle"))
	dlg.ShowClose = true

	items := make([]string, len(entries))
	for i := range entries {
		items[i] = toolsOptionsRowText(!hidden[entries[i].ActionName], names[i])
	}
	list := &toolsOptionsList{ListBox: vtui.NewListBox(0, 0, width-4, listH, items)}
	settings := vtui.NewButton(0, 0, settingsLabel)
	settings.IsDefault = false

	configFor := func(idx int) (vfs.PluginCommand, bool) {
		if idx < 0 || idx >= len(entries) || entries[idx].CommandID == "" {
			return vfs.PluginCommand{}, false
		}
		return pluginConfigCommandFor(entries[idx].CommandID, pf)
	}
	refreshButton := func(idx int) {
		_, ok := configFor(idx)
		settings.SetDisabled(!ok)
	}
	list.onSelect = refreshButton
	list.onToggle = func(idx int) {
		if idx < 0 || idx >= len(entries) {
			return
		}
		name := entries[idx].ActionName
		hidden[name] = !hidden[name]
		if err := SetPluginMenuEntryHidden(name, hidden[name]); err != nil {
			hidden[name] = !hidden[name]
			vtui.ShowMessage(i18n.Msg("Plugins.ToolsOptionsTitle"), err.Error(), []string{i18n.Msg("vtui.Ok")})
			return
		}
		list.Items[idx] = toolsOptionsRowText(!hidden[name], names[idx])
		list.UpdateRows()
	}
	list.OnKeyDown = func(e *vtinput.InputEvent) bool {
		switch {
		case e.VirtualKeyCode == vtinput.VK_SPACE || e.Char == ' ':
			list.onToggle(list.SelectPos)
			return true
		case e.VirtualKeyCode == vtinput.VK_RETURN:
			if command, ok := configFor(list.SelectPos); ok {
				plughost.ExecutePluginCommand(vfs.PluginCommandConfig, command.ID, pf)
			} else {
				list.onToggle(list.SelectPos)
			}
			return true
		}
		return false
	}
	settings.OnClick = func() {
		if command, ok := configFor(list.SelectPos); ok {
			plughost.ExecutePluginCommand(vfs.PluginCommandConfig, command.ID, pf)
		}
	}
	refreshButton(0)

	dlg.AddItem(list)
	dlg.AddItem(settings)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(list, vtui.Margins{}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Add(settings, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	dlg.SetFocusedItem(list)

	vtui.FrameManager.Push(dlg)
}
