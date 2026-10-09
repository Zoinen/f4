package panel

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/sysinfo"
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

// toolsWindow describes one tools window: the F11 menu's and the drive
// menu's differ only in where the rows come from and what the buttons do.
type toolsWindow struct {
	title, empty string
	names        []string
	shown        []bool
	// setShown stores the choice of row idx; an error leaves the row as it was.
	setShown func(idx int, shown bool) error
	// configure returns what the Settings button runs for row idx; nil, or a
	// false result, dims the button. A nil configure leaves the button out.
	configure func(idx int) (func(), bool)
	// extraLabel and extra add a button of the window's own, always live.
	extraLabel string
	extra      func()
}

// show opens the window: every row with a check box that works at once.
func (tw *toolsWindow) show() {
	if len(tw.names) == 0 {
		vtui.ShowMessage(tw.title, tw.empty, []string{i18n.Msg("vtui.Ok")})
		return
	}
	settingsLabel := i18n.Msg("Plugins.ToolsOptionsSettings")
	buttonsWidth := 0
	if tw.configure != nil {
		buttonsWidth += runewidth.StringWidth(settingsLabel) + 8
	}
	if tw.extra != nil {
		buttonsWidth += runewidth.StringWidth(tw.extraLabel) + 8
	}
	width := runewidth.StringWidth(tw.title) + 6
	for _, name := range tw.names {
		if w := runewidth.StringWidth(name) + 4 + 4; w > width {
			width = w
		}
	}
	width = max(width, buttonsWidth+4, 30)
	screenW, screenH := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	if screenW > 0 && width > screenW-2 {
		width = screenW - 2
	}
	listH := len(tw.names)
	if screenH > 0 && listH > screenH-8 {
		listH = max(3, screenH-8)
	}
	height := listH + 6

	dlg := vtui.NewCenteredDialog(width, height, tw.title)
	dlg.ShowClose = false

	items := make([]string, len(tw.names))
	for i := range tw.names {
		items[i] = toolsOptionsRowText(tw.shown[i], tw.names[i])
	}
	list := &toolsOptionsList{ListBox: vtui.NewListBox(0, 0, width-4, listH, items)}
	// The rows sit on the dialog background in the dialog text colour, as the
	// lists of the Settings dialog do (f4#918).
	list.ColorTextIdx = vtui.ColDialogText
	list.ColorItemSelectTextIdx = vtui.ColDialogText

	var settings *vtui.Button
	configFor := func(idx int) (func(), bool) {
		if tw.configure == nil || idx < 0 || idx >= len(tw.names) {
			return nil, false
		}
		return tw.configure(idx)
	}
	refreshButton := func(idx int) {
		if settings != nil {
			_, ok := configFor(idx)
			settings.SetDisabled(!ok)
		}
	}
	list.onSelect = refreshButton
	list.onToggle = func(idx int) {
		if idx < 0 || idx >= len(tw.names) {
			return
		}
		shown := !tw.shown[idx]
		if err := tw.setShown(idx, shown); err != nil {
			vtui.ShowMessage(tw.title, err.Error(), []string{i18n.Msg("vtui.Ok")})
			return
		}
		tw.shown[idx] = shown
		list.Items[idx] = toolsOptionsRowText(shown, tw.names[idx])
		list.UpdateRows()
	}
	list.OnKeyDown = func(e *vtinput.InputEvent) bool {
		switch {
		case e.VirtualKeyCode == vtinput.VK_SPACE || e.Char == ' ':
			list.onToggle(list.SelectPos)
			return true
		case e.VirtualKeyCode == vtinput.VK_RETURN:
			if run, ok := configFor(list.SelectPos); ok {
				run()
			} else {
				list.onToggle(list.SelectPos)
			}
			return true
		}
		return false
	}
	dlg.AddItem(list)

	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	if tw.configure != nil {
		settings = vtui.NewButton(0, 0, settingsLabel)
		settings.IsDefault = false
		settings.OnClick = func() {
			if run, ok := configFor(list.SelectPos); ok {
				run()
			}
		}
		dlg.AddItem(settings)
		buttons.Add(settings, vtui.Margins{Right: 1}, vtui.AlignTop)
		refreshButton(0)
	}
	if tw.extra != nil {
		extra := vtui.NewButton(0, 0, tw.extraLabel)
		extra.IsDefault = false
		extra.OnClick = tw.extra
		dlg.AddItem(extra)
		buttons.Add(extra, vtui.Margins{}, vtui.AlignTop)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(list, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	dlg.SetFocusedItem(list)

	vtui.FrameManager.Push(dlg)
}

// ShowToolsOptions opens the window of the F11 menu's tools: every entry with a
// check box that shows or hides it at once, and a Settings button that is live
// when the selected tool has settings of its own (f4#918).
func (pf *PanelsFrame) ShowToolsOptions() {
	entries := PluginMenuEntriesSnapshot()
	hidden := hiddenPluginMenuEntries()
	tw := &toolsWindow{
		title: i18n.Msg("Plugins.ToolsOptionsTitle"),
		empty: i18n.Msg("Plugins.ToolsOptionsEmpty"),
		setShown: func(idx int, shown bool) error {
			name := entries[idx].ActionName
			if err := SetPluginMenuEntryHidden(name, !shown); err != nil {
				return err
			}
			hidden[name] = !shown
			return nil
		},
		configure: func(idx int) (func(), bool) {
			if entries[idx].CommandID == "" {
				return nil, false
			}
			command, ok := pluginConfigCommandFor(entries[idx].CommandID, pf)
			if !ok {
				return nil, false
			}
			return func() { plughost.ExecutePluginCommand(vfs.PluginCommandConfig, command.ID, pf) }, true
		},
	}
	for _, entry := range entries {
		tw.names = append(tw.names, action.PlainLabel(entry.Label))
		tw.shown = append(tw.shown, !hidden[entry.ActionName])
	}
	tw.show()
}

// ShowDriveToolsOptions is the same window for the drive menu's tools, opened
// by F9 there. Drive tools have no settings of their own; the old F9 page, the
// drive menu's own options, stays one button away (f4#918).
func (pf *PanelsFrame) ShowDriveToolsOptions(menuOptions func()) {
	drives := sysinfo.DriveRegistrySnapshot()
	disabled, err := LoadDisabledDriveTools(DriveToolsVisibilityFilePath())
	if err != nil {
		vtui.DebugLog("DRIVE TOOLS: load visibility failed: %v", err)
	}
	hidden := map[string]bool{}
	for _, name := range disabled {
		hidden[name] = true
	}
	tw := &toolsWindow{
		title:      i18n.Msg("Drive.ToolsOptionsTitle"),
		empty:      i18n.Msg("Drive.ToolsOptionsEmpty"),
		extraLabel: i18n.Msg("Drive.ToolsOptionsMenu"),
		extra:      menuOptions,
		setShown: func(idx int, shown bool) error {
			name := drives[idx].Name
			var names []string
			found := false
			for _, n := range disabled {
				if n == name {
					found = true
					if shown {
						continue
					}
				}
				names = append(names, n)
			}
			if !shown && !found {
				names = append(names, name)
			}
			if err := SaveDisabledDriveTools(DriveToolsVisibilityFilePath(), names); err != nil {
				return err
			}
			disabled = names
			return nil
		},
	}
	for _, drv := range drives {
		tw.names = append(tw.names, driveMenuNameWithoutMarker(drv.Name))
		tw.shown = append(tw.shown, !hidden[drv.Name])
	}
	tw.show()
}
