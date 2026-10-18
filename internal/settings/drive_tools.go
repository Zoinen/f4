package settings

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// The Tools page of the drive menu's settings window (f4#1148): one check box
// per tool the drive menu can list, ticked when the tool is shown. It lives in
// this window only, not in the Drive chooser category of the main Settings,
// which is left as it was. Apply writes drive-tools-visibility.txt, the file the
// drive menu already reads.

const driveToolFieldPrefix = "drivetool."

// driveToolsOrderValue is a draft value with no field of its own: the tools'
// indexes in p.tools, comma separated, in the order the page shows them. Up
// and Down change it, and Apply writes the order the drive menu reads
// (drive-tools-order.txt, f4#1148).
const driveToolsOrderValue = "drivetools-order"

// tools are listed in the order of the drive menu, the built-in ones
// included (panel.DriveMenuToolChoices, f4#1148).
type driveToolsProvider struct {
	tools []panel.DriveToolChoice
}

// entries are the tools as the visibility file names them.
func (p driveToolsProvider) entries() []sysinfo.DriveEntry {
	entries := make([]sysinfo.DriveEntry, len(p.tools))
	for i, t := range p.tools {
		entries[i] = sysinfo.DriveEntry{Name: t.Key}
	}
	return entries
}

func (p driveToolsProvider) Catalog() f4settings.Catalog {
	cat := f4settings.Catalog{ID: "drive-tools", Categories: driveChooserPages}
	for i, tool := range p.tools {
		cat.Fields = append(cat.Fields, f4settings.Field{
			ID:          driveToolField(i),
			Category:    driveToolsPage,
			Label:       f4settings.Text{English: tool.Label, Literal: true},
			Description: f4settings.Text{Key: "SettingsCenter.DriveTools.Show", English: "Show this tool in the drive menu."},
			Kind:        f4settings.Boolean,
			Default:     "true",
		})
	}
	return cat
}

func driveToolField(i int) string { return driveToolFieldPrefix + strconv.Itoa(i) }

// driveToolHotkeyValue is the draft value, with no field of its own, of the
// i-th tool's hot key: a letter or a digit, or "" for none. The "Hot key:"
// field under Up and Down shows and edits the one of the selected tool, and
// Apply binds it as F4 in the drive menu does (mitek1977-coder, f4#1148).
func driveToolHotkeyValue(i int) string { return "drivetools-hotkey." + strconv.Itoa(i) }

// orderText is the order value for fields, the Tools page's fields in the
// order they are shown.
func driveToolsOrderText(fields []f4settings.Field) string {
	var ids []string
	for _, f := range fields {
		if strings.HasPrefix(f.ID, driveToolFieldPrefix) {
			ids = append(ids, strings.TrimPrefix(f.ID, driveToolFieldPrefix))
		}
	}
	return strings.Join(ids, ",")
}

// orderedKeys are the tools' keys in the order of the order value, followed
// by the names of the order file the page does not show (a plugin that is not
// installed now keeps its place for when it returns).
func (p driveToolsProvider) orderedKeys(order string, file []string) []string {
	var keys []string
	seen := map[string]bool{}
	for _, s := range strings.Split(order, ",") {
		if i, err := strconv.Atoi(s); err == nil && i >= 0 && i < len(p.tools) && !seen[p.tools[i].Key] {
			keys = append(keys, p.tools[i].Key)
			seen[p.tools[i].Key] = true
		}
	}
	for _, name := range file {
		if !seen[name] {
			keys = append(keys, name)
			seen[name] = true
		}
	}
	return keys
}

func (p driveToolsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	disabled, err := panel.LoadDisabledDriveTools(panel.DriveToolsVisibilityFilePath())
	if err != nil {
		return nil, err
	}
	hidden := map[string]bool{}
	for _, name := range disabled {
		hidden[name] = true
	}
	values := map[string]string{}
	for i, tool := range p.tools {
		values[driveToolField(i)] = boolText(!hidden[tool.Key])
	}
	values[driveToolsOrderValue] = driveToolsOrderText(p.Catalog().Fields)
	for i, tool := range p.tools {
		values[driveToolHotkeyValue(i)] = panel.DriveToolHotkeyText(tool.Key, tool.Label)
	}
	d := f4settings.NewDraft(values, nil)
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		result := f4settings.Result{Errors: map[string]error{}, Values: map[string]string{}}
		if p.commitHotkeys(d, &result) {
			return result
		}
		changed := false
		shown := make([]bool, len(p.tools))
		for i := range p.tools {
			id := driveToolField(i)
			shown[i] = d.Values[id] != "false"
			if d.Dirty(id) {
				changed = true
			}
		}
		if d.Dirty(driveToolsOrderValue) {
			file, err := panel.LoadDriveToolsOrder(panel.DriveToolsOrderFilePath())
			if err == nil {
				err = panel.SaveDriveToolsOrder(panel.DriveToolsOrderFilePath(), p.orderedKeys(d.Values[driveToolsOrderValue], file))
			}
			if err != nil {
				result.Errors[driveToolsPage] = err
				return result
			}
			result.Applied = append(result.Applied, driveToolsOrderValue)
		}
		if !changed {
			return result
		}
		// The file is read again: it may have changed since the window opened.
		current, err := panel.LoadDisabledDriveTools(panel.DriveToolsVisibilityFilePath())
		if err != nil {
			result.Errors[driveToolsPage] = err
			return result
		}
		if err := panel.SaveDisabledDriveTools(panel.DriveToolsVisibilityFilePath(), panel.HiddenDriveToolsAfter(current, p.entries(), shown)); err != nil {
			result.Errors[driveToolsPage] = err
			return result
		}
		for i := range p.tools {
			if id := driveToolField(i); d.Dirty(id) {
				result.Applied = append(result.Applied, id)
			}
		}
		return result
	}
	return d, nil
}

// commitHotkeys binds the hot keys changed on the page. A letter belongs to one
// row of the drive menu, so giving it to a tool may take it from another one:
// every tool's hot key is read back afterwards. It reports true when a hot key
// could not be set and the rest of the page must wait.
func (p driveToolsProvider) commitHotkeys(d *f4settings.Draft, result *f4settings.Result) bool {
	dirty := false
	for i, tool := range p.tools {
		id := driveToolHotkeyValue(i)
		if !d.Dirty(id) {
			continue
		}
		dirty = true
		if _, ok := panel.SetDriveToolHotkey(tool.Key, tool.Label, d.Values[id]); !ok {
			result.Errors[driveToolsPage] = errors.New(i18n.Msg("Plugins.HotkeyFieldInvalid"))
			return true
		}
	}
	if !dirty {
		return false
	}
	for i, tool := range p.tools {
		id := driveToolHotkeyValue(i)
		result.Values[id] = panel.DriveToolHotkeyText(tool.Key, tool.Label)
		result.Applied = append(result.Applied, id)
	}
	return false
}

func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// newDriveToolsSession is the session of the Tools page for the tools the
// drive menu knows now.
func newDriveToolsSession() (*settingsSession, error) {
	p := driveToolsProvider{tools: panel.DriveMenuToolChoices()}
	d, err := p.Begin(context.Background())
	if err != nil {
		return nil, err
	}
	return &settingsSession{provider: p, catalog: settingsCatalogChoiceHelp(p.Catalog()), draft: d}, nil
}

// addDriveToolMoves puts Up and Down under the tools of the Tools page: they
// move the tool last selected there one place in the drive menu's order, as
// Shift+Up/Down does in the menu itself (mitek1977-coder, f4#1148).
func (c *settingsCenter) addDriveToolMoves(category string) {
	for _, s := range c.sessions {
		if _, ok := s.provider.(driveToolsProvider); !ok {
			continue
		}
		bar := &settingsButtonRow{Group: vtui.NewGroup(0, 0, 20, 1)}
		bar.SetId("drive-tools-move")
		for _, delta := range []int{-1, 1} {
			label := "Down"
			if delta < 0 {
				label = "Up"
			}
			b := vtui.NewButton(0, 0, Phrase(label))
			b.SetId("drive-tools-move:" + label)
			b.OnClick = func() { c.moveDriveTool(s, delta) }
			bar.buttons = append(bar.buttons, b)
			bar.AddItem(b)
		}
		c.page.AddItem(bar)
		// One blank line between the tools and Up / Down, another before
		// "Hot key:" (mitek1977-coder, f4#1148).
		c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: category, Label: f4settings.Text{English: "Up / Down"}, Description: f4settings.Text{Key: "SettingsCenter.DriveTools.Move", English: "Move the selected tool up or down in the drive menu."}}, session: s, control: bar, match: true, padTop: 1})

		hotkey := panel.NewHotkeyEdit("")
		hotkey.SetId("drive-tools-hotkey")
		hotkey.OnChange = func(text string) {
			if i, ok := driveToolIndex(c.driveToolSelected); ok {
				s.draft.Values[driveToolHotkeyValue(i)] = text
			}
		}
		c.driveToolHotkey = hotkey
		c.page.AddItem(hotkey)
		c.page.rows = append(c.page.rows, &settingsRow{field: f4settings.Field{Category: category, Label: f4settings.Text{Key: "DriveLink.Hotkey", English: "Hot key:"}, Description: f4settings.Text{Key: "SettingsCenter.DriveTools.Hotkey", English: "The hot key of the selected tool in the drive menu: a letter or a digit; empty for none. F4 on the tool in the menu sets the same key."}}, session: s, control: hotkey, match: true, padTop: 1})
		if _, ok := driveToolIndex(c.driveToolSelected); !ok {
			for _, f := range s.catalog.Fields {
				if strings.HasPrefix(f.ID, driveToolFieldPrefix) {
					c.driveToolSelected = f.ID
					break
				}
			}
		}
		c.showDriveToolHotkey()
	}
}

// driveToolIndex is the tool's index in driveToolsProvider.tools for the
// field id of its check box.
func driveToolIndex(id string) (int, bool) {
	if !strings.HasPrefix(id, driveToolFieldPrefix) {
		return 0, false
	}
	i, err := strconv.Atoi(strings.TrimPrefix(id, driveToolFieldPrefix))
	return i, err == nil
}

// showDriveToolHotkey puts the selected tool's hot key, as the page holds it,
// into the "Hot key:" field.
func (c *settingsCenter) showDriveToolHotkey() {
	if c.driveToolHotkey == nil {
		return
	}
	text := ""
	if i, ok := driveToolIndex(c.driveToolSelected); ok {
		for _, s := range c.sessions {
			if _, isTools := s.provider.(driveToolsProvider); isTools {
				text = s.draft.Values[driveToolHotkeyValue(i)]
			}
		}
	}
	c.driveToolHotkey.SetText(text)
}

// moveDriveTool moves the selected tool's field delta places on the page and
// keeps it selected, so the next Up or Down moves it again.
func (c *settingsCenter) moveDriveTool(s *settingsSession, delta int) {
	if panel.DriveToolsSortedByName() {
		c.status = i18n.Msg("Drive.ToolsSortedByName")
		return
	}
	fields := s.catalog.Fields
	i := -1
	for k, f := range fields {
		if f.ID == c.driveToolSelected {
			i = k
		}
	}
	j := i + delta
	if i < 0 || j < 0 || j >= len(fields) || !strings.HasPrefix(fields[j].ID, driveToolFieldPrefix) {
		return
	}
	fields[i], fields[j] = fields[j], fields[i]
	s.draft.Values[driveToolsOrderValue] = driveToolsOrderText(fields)
	moved := c.driveToolSelected
	c.rebuildCategory()
	for _, r := range c.page.rows {
		if r.control != nil && r.control.GetId() == "setting:"+moved {
			c.page.SetFocusedItem(r.control)
			c.page.notifyFocus()
		}
	}
}
