package dialog

import (
	"fmt"
	"strings"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// ConfigEditorApply makes an edit take effect in the running application:
// the composition root refreshes whatever depends on the changed F4Config
// fields, as it does after Settings Center's Apply.
type ConfigEditorApply func(before config.F4Config, changed []string)

// configEditorState is what survives the editor closing and reopening around
// an edit: the two view toggles and the row the user was on.
type configEditorState struct {
	hideUnchanged bool
	alignDot      bool
	selected      string // Section.Key
}

// configEditorRow is one settings.ini key with its default.
type configEditorRow struct {
	option     config.Option
	def        string
	hasDefault bool
}

func (r configEditorRow) name() string { return r.option.Section + "." + r.option.Key }

// changed is far2l's IsNotDefault: a key without a default counts as changed,
// so that hiding unchanged rows never hides it.
func (r configEditorRow) changed() bool { return !r.hasDefault || r.option.Value != r.def }

// mark is far2l's first column: blank for the default, * for a changed value,
// ? for a key that has no default to compare with.
func (r configEditorRow) mark() string {
	switch {
	case !r.hasDefault:
		return "?"
	case r.option.Value != r.def:
		return "*"
	default:
		return " "
	}
}

// ShowConfigEditor opens f4:config, modelled on far2l's far:config: every key
// f4 writes to settings.ini, marked when it differs from the default. Enter,
// F4 or a double click edit a value, Del resets it, Ctrl+H keeps only the
// changed rows and Ctrl+A aligns the names on the dot.
func ShowConfigEditor(apply ConfigEditorApply) {
	showConfigEditor(apply, configEditorState{})
}

func showConfigEditor(apply ConfigEditorApply, st configEditorState) {
	if vtui.FrameManager == nil {
		return
	}
	hint := i18n.Msg("ConfigEditor.Hint")
	menu := vtui.NewVMenu(i18n.Msg("ConfigEditor.Title"))
	menu.SetBottomTitle(hint)
	// A single click only moves the selection; a double click edits.
	menu.IgnoreSingleClick = true
	defaults := config.Options(config.DefaultConfig())

	var rows []configEditorRow
	var index []int
	remember := func() {
		if pos := menu.SelectPos; pos >= 0 && pos < len(index) {
			st.selected = rows[index[pos]].name()
		}
	}
	fill := func() {
		rows = configEditorRows(config.Options(config.App), defaults)
		var lines []string
		lines, index = configEditorLines(rows, st)
		menu.SetTitle(configEditorTitle(st.hideUnchanged))

		scrW := vtui.FrameManager.GetScreenSize()
		scrH := vtui.FrameManager.GetScreenHeight()
		const chrome = 5
		w := vtui.StringWidth(menu.GetTitle()) + chrome + 1
		if hw := vtui.StringWidth(hint) + chrome + 1; hw > w {
			w = hw
		}
		for _, line := range lines {
			if lw := vtui.StringWidth(line) + chrome; lw > w {
				w = lw
			}
		}
		if maxW := scrW - 4; maxW >= 20 && w > maxW {
			w = maxW
		} else if w > scrW {
			w = scrW
		}
		h := len(lines) + 2
		if maxH := scrH - 4; maxH >= 3 && h > maxH {
			h = maxH
		} else if h > scrH {
			h = scrH
		}
		if h < 3 {
			h = 3
		}

		items := make([]vtui.MenuItem, 0, len(lines))
		for _, line := range lines {
			items = append(items, vtui.MenuItem{Text: strings.ReplaceAll(vtui.TruncateMiddle(line, w-chrome), "&", "&&")})
		}
		menu.Items = items
		menu.ItemCount = len(items)
		x, y := (scrW-w)/2, (scrH-h)/2
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		menu.SetPosition(x, y, x+w-1, y+h-1)
		selected := 0
		for pos, row := range index {
			if rows[row].name() == st.selected {
				selected = pos
				break
			}
		}
		menu.SetSelectPos(selected)
	}
	fill()

	// Enter and a double click close the menu before OnAction's edit dialog
	// comes back, so the editor is reopened where it was once that is done.
	menu.OnAction = func(pos int) {
		if pos < 0 || pos >= len(index) {
			return
		}
		row := rows[index[pos]]
		st.selected = row.name()
		editConfigOption(row, apply, func() { showConfigEditor(apply, st) })
	}
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		switch {
		case e.VirtualKeyCode == vtinput.VK_F4 && !ctrl:
			// far2l opens the editor on F4, Shift+F4 and Alt+F4 alike.
			return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
		case e.VirtualKeyCode == vtinput.VK_DELETE && !ctrl && !alt && !shift:
			pos := menu.SelectPos
			if pos < 0 || pos >= len(index) {
				return true
			}
			row := rows[index[pos]]
			if !row.hasDefault || !row.changed() {
				return true
			}
			st.selected = row.name()
			dlg := vtui.ShowMessage(i18n.Msg("ConfigEditor.ResetTitle"),
				fmt.Sprintf(i18n.Msg("ConfigEditor.ResetPrompt"), row.name(), configEditorShownValue(row.option.Section, row.option.Key, row.def)),
				[]string{i18n.Msg("ConfigEditor.ResetButton"), i18n.Msg("vtui.Cancel")})
			dlg.OnResult = func(code int) {
				if code == 0 {
					setConfigOption(row, row.def, apply)
					fill()
					vtui.FrameManager.Redraw()
				}
			}
			return true
		case ctrl && !alt && !shift && e.VirtualKeyCode == vtinput.VK_H:
			remember()
			st.hideUnchanged = !st.hideUnchanged
			fill()
			vtui.FrameManager.Redraw()
			return true
		case ctrl && !alt && !shift && e.VirtualKeyCode == vtinput.VK_A:
			remember()
			st.alignDot = !st.alignDot
			fill()
			vtui.FrameManager.Redraw()
			return true
		}
		return false
	}
	vtui.FrameManager.Push(menu)
}

// configEditorTitle marks the title while unchanged rows are hidden, as
// far2l marks far:config.
func configEditorTitle(hideUnchanged bool) string {
	title := i18n.Msg("ConfigEditor.Title")
	if hideUnchanged {
		title += " *"
	}
	return title
}

// configEditorRows pairs each current option with its default.
func configEditorRows(current, defaults []config.Option) []configEditorRow {
	byName := make(map[string]string, len(defaults))
	for _, option := range defaults {
		byName[option.Section+"."+option.Key] = option.Value
	}
	rows := make([]configEditorRow, 0, len(current))
	for _, option := range current {
		def, ok := byName[option.Section+"."+option.Key]
		rows = append(rows, configEditorRow{option: option, def: def, hasDefault: ok})
	}
	return rows
}

// configEditorLines lays the rows out as far2l's far:config does and returns,
// for each line, the row it shows. Names are padded to the longest one, or,
// with alignDot, sections are right-aligned and keys left-aligned on the dot.
func configEditorLines(rows []configEditorRow, st configEditorState) ([]string, []int) {
	lenSections, lenKeys, lenNames := 0, 0, 0
	for _, row := range rows {
		lenSections = max(lenSections, len(row.option.Section))
		lenKeys = max(lenKeys, len(row.option.Key))
		lenNames = max(lenNames, len(row.name()))
	}
	var lines []string
	var index []int
	for i, row := range rows {
		if st.hideUnchanged && !row.changed() {
			continue
		}
		name := fmt.Sprintf("%-*s", lenNames, row.name())
		if st.alignDot {
			name = fmt.Sprintf("%*s.%-*s", lenSections, row.option.Section, lenKeys, row.option.Key)
		}
		value := configEditorShownValue(row.option.Section, row.option.Key, row.option.Value)
		lines = append(lines, row.mark()+" "+name+" │ "+value)
		index = append(index, i)
	}
	return lines, index
}

// configEditorShownValue hides the proxy password. settings.ini keeps it
// obfuscated, not encrypted, and a list on screen ends up in screenshots.
func configEditorShownValue(section, key, value string) string {
	if section == "Proxy" && key == "Password" && value != "" {
		return "********"
	}
	return value
}

// editConfigOption asks for a new value and calls done when the dialog
// closes, whichever way it closes.
func editConfigOption(row configEditorRow, apply ConfigEditorApply, done func()) {
	def := i18n.Msg("ConfigEditor.NoDefault")
	if row.hasDefault {
		def = configEditorShownValue(row.option.Section, row.option.Key, row.def)
	}
	dlg := vtui.InputBox(row.name(), fmt.Sprintf(i18n.Msg("ConfigEditor.DefaultPrompt"), def), row.option.Value,
		func(value string) { setConfigOption(row, value, apply) })
	dlg.OnResult = func(int) { done() }
}

// setConfigOption applies one value as f4 would read it back from
// settings.ini, lets the application react, and saves when autosave allows,
// the way far:config changes wait for the configuration to be saved. When
// f4 kept something other than what was typed, a toast says what.
func setConfigOption(row configEditorRow, value string, apply ConfigEditorApply) {
	before := config.App
	config.App = config.WithOption(before, row.option.Section, row.option.Key, value)
	config.SyncAutoSaveMaster()
	if changed := config.ChangedFields(before, config.App); len(changed) > 0 {
		if apply != nil {
			apply(before, changed)
		}
		config.RequestSaveConfig()
	}
	for _, option := range config.Options(config.App) {
		if option.Section != row.option.Section || option.Key != row.option.Key {
			continue
		}
		if option.Value != strings.TrimSpace(value) {
			toast.Show(fmt.Sprintf(i18n.Msg("ConfigEditor.Kept"), row.name(), configEditorShownValue(option.Section, option.Key, option.Value)), 4*time.Second)
		}
		break
	}
}
