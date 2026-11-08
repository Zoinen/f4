package settings

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type fileGroupsOverview struct {
	*vtui.Group
	center         *settingsCenter
	session        *settingsSession
	themes         *vtui.ComboBox
	colors         *vtui.ComboBox
	colorIDs       []string
	themeIDs       []string
	themeID        string
	table          *fileGroupsTable
	actions        *settingsButtonRow
	marked         map[string]bool
	previews       map[string]fileGroupPreview
	samples        map[string]fileGroupSampleData
	highlighter    *theme.FileHighlighter
	rows           []fileGroupOverviewRow
	selectedID     string
	editor         *settingsCenter
	pendingAddedID string
}

type fileGroupOverviewRow struct {
	view      *fileGroupsOverview
	record    f4settings.Record
	separator string
	cells     [4]string
}

func (r fileGroupOverviewRow) GetCellText(col int) string {
	if col < 0 || col >= len(r.cells) {
		return ""
	}
	return r.cells[col]
}

// Prepare display text with the draft projection, never during painting.
func (r fileGroupOverviewRow) cellText(col int) string {
	if r.separator != "" {
		if col == 0 {
			return "── " + r.separator + " ──"
		}
		return ""
	}
	v := r.record.Values
	switch col {
	case 0:
		p := r.view.previews[r.record.ID]
		label := p.item.Name
		if !p.matched {
			label += " (?)"
		}
		return p.style.Marker + label
	case 1:
		return v["group.Name"]
	case 2:
		return fileGroupRecordConditions(r.record)
	case 3:
		features := []string{}
		group := fileGroupRecordGroup(r.record)
		for _, f := range []string{"Highlight", "Icons", "Sort", "Associations"} {
			enabled := v["group."+f+".Enabled"] == "true"
			if f == "Highlight" {
				enabled = group.Highlighting(config.App.ColorStyle).Enabled("Highlight")
				if len(r.view.session.draft.Records["filegroups.colorthemes"]) > 0 && r.view.highlighter != nil {
					enabled = false
					for _, rules := range [][]theme.HighlightRule{r.view.highlighter.UserRules, r.view.highlighter.ThemeRules} {
						for _, rule := range rules {
							if rule.RuleID == r.record.ID {
								enabled = true
								break
							}
						}
					}
				}
			}
			if enabled {
				features = append(features, f)
			}
		}
		return strings.Join(features, ", ")
	}
	return ""
}

func (r fileGroupOverviewRow) IsSelected() bool { return r.view.marked[r.record.ID] }
func (r fileGroupOverviewRow) GetCellAttr(col int, attr uint64) uint64 {
	if col != 0 || r.separator != "" || r.view.highlighter == nil {
		return attr
	}
	p := r.view.previews[r.record.ID]
	state := 0
	if r.IsSelected() {
		state++
	}
	if r.view.selectedID == r.record.ID {
		state += 2
	}
	return p.attrs[state]
}

type fileGroupsTable struct {
	*vtui.Table
	view *fileGroupsOverview
}

func newFileGroupsOverview(c *settingsCenter, s *settingsSession) *fileGroupsOverview {
	v := &fileGroupsOverview{Group: vtui.NewGroup(0, 0, 1, 1), center: c, session: s,
		themeID: fileGroupActiveTheme, marked: map[string]bool{}, previews: map[string]fileGroupPreview{}, samples: map[string]fileGroupSampleData{}}
	v.SetId("settings-page")
	v.themes = vtui.NewComboBox(0, 0, 24, nil)
	v.themes.SetId("filegroups:theme")
	v.themes.DropdownOnly = true
	v.themes.Menu.OnAction = func(i int) {
		if i >= 0 && i < len(v.themeIDs) {
			v.themeID = v.themeIDs[i]
			v.refresh()
			vtui.DebugLog("FILEGROUPS: editor theme=%s", v.themeID)
		}
	}
	v.colors = vtui.NewComboBox(0, 0, 24, nil)
	v.colors.SetId("filegroups:colors")
	v.colors.DropdownOnly = true
	v.colors.Menu.OnAction = func(i int) {
		if i < 0 || i >= len(v.colorIDs) {
			return
		}
		v.session.draft.Values[fileGroupColorThemeKey] = v.colorIDs[i]
		vtui.DebugLog("FILEGROUPS: draft colour theme=%s, preview icon theme=%s", v.colorIDs[i], v.themeID)
		v.changed()
	}
	v.table = &fileGroupsTable{Table: vtui.NewTable(0, 0, 70, 15, []vtui.TableColumn{
		{Title: i18n.Msg("FileGroups.Preview"), Width: 23}, {Title: i18n.Msg("FileGroups.Group"), Width: 22}, {Title: i18n.Msg("FileGroups.Conditions"), Width: 30}, {Title: i18n.Msg("FileGroups.Features"), Width: 21},
	}), view: v}
	v.table.SetId("filegroups:overview")
	settingsDialogTable(v.table.Table)
	v.table.AlwaysShowCursor = true
	v.table.Sortable = false
	v.table.QuickSearch = true
	v.table.OnSelect = func(i int) {
		i = v.table.RowAt(i)
		if i < 0 || i >= len(v.rows) || v.rows[i].separator != "" {
			return
		}
		v.selectedID = v.rows[i].record.ID
		for index, record := range s.draft.Records["filegroups"] {
			if record.ID == v.selectedID {
				c.offsets["record:filegroups"] = index
			}
		}
		c.help.text = fileGroupRecordConditions(v.rows[i].record) + "\n" + v.previews[v.selectedID].contributors
	}
	v.table.OnAction = func(int) { v.edit() }
	v.table.OnSearchChange = func(string) {
		if v.table.ItemCount > 0 {
			v.table.OnSelect(v.table.SelectPos)
		} else {
			v.selectedID = ""
		}
	}
	v.actions = &settingsButtonRow{Group: vtui.NewGroup(0, 0, 1, 1)}
	v.actions.SetId("filegroups:actions")
	for _, a := range []struct {
		name string
		run  func()
	}{
		{"Add", v.add}, {"Edit F4", v.edit}, {"Duplicate", v.duplicate}, {"Delete", v.remove},
		{"Up", func() { v.move(-1) }}, {"Down", func() { v.move(1) }}, {"More", v.more},
		{"Restore standard set", v.restoreStandardSet},
	} {
		b := vtui.NewButton(0, 0, a.name)
		b.OnClick = a.run
		b.SetId(fmt.Sprintf("filegroups:action:%d", len(v.actions.buttons)))
		v.actions.buttons = append(v.actions.buttons, b)
		v.actions.AddItem(b)
	}
	v.AddItem(v.themes)
	v.AddItem(v.colors)
	v.AddItem(v.table)
	v.AddItem(v.actions)
	v.SetFocusedItem(v.table)
	v.refresh()
	return v
}

func (v *fileGroupsOverview) SetPosition(x1, y1, x2, y2 int) {
	v.Group.SetPosition(x1, y1, x2, y2)
	middle := x1 + (x2-x1+1)/2
	v.themes.SetPosition(x1, y1, max(x1, middle-2), y1)
	v.colors.SetPosition(middle, y1, x2, y1)
	positions := make([][3]int, 0, len(v.actions.buttons))
	x, row := x1, 0
	for _, button := range v.actions.buttons {
		width := min(vtui.StringWidth(button.GetCaption())+4, max(1, x2-x1+1))
		if x > x1 && x+width-1 > x2 {
			x, row = x1, row+1
		}
		positions = append(positions, [3]int{x, row, width})
		x += width + 1
	}
	actionsY := max(y1+4, y2-row)
	v.table.SetPosition(x1, y1+2, x2, max(y1+3, actionsY-2))
	v.actions.SetPosition(x1, actionsY, x2, y2)
	for i, position := range positions {
		v.actions.buttons[i].SetPosition(position[0], actionsY+position[1], position[0]+position[2]-1, actionsY+position[1])
	}
}

func (v *fileGroupsOverview) ProcessKey(e *vtinput.InputEvent) bool {
	if e.KeyDown && e.VirtualKeyCode == vtinput.VK_TAB {
		items := []vtui.UIElement{v.themes}
		if len(v.colorIDs) > 0 {
			items = append(items, v.colors)
		}
		items = append(items, v.table, v.actions)
		direction := 1
		if e.ControlKeyState&vtinput.ShiftPressed != 0 {
			direction = -1
		}
		for i, item := range items {
			if item == v.GetFocusedItem() {
				next := i + direction
				if next < 0 || next >= len(items) {
					return false
				}
				v.SetFocusedItem(items[next])
				return true
			}
		}
	}
	return v.Group.ProcessKey(e)
}

func (v *fileGroupsOverview) refresh() {
	top := v.table.TopPos
	v.themeIDs = []string{}
	names := []string{}
	for _, set := range fileGroupDraftThemes(v.session.draft) {
		if !set.Enabled() && set.ID != v.themeID {
			continue
		}
		v.themeIDs = append(v.themeIDs, set.ID)
		names = append(names, "Icons: "+set.Name())
	}
	if len(v.themeIDs) == 0 {
		v.themeIDs = []string{v.themeID}
		names = []string{"Theme: " + fileGroupThemeName(v.themeID)}
	}
	v.themes.Menu.Items = nil
	for _, name := range names {
		v.themes.Menu.AddItem(vtui.MenuItem{Text: name})
	}
	for i, id := range v.themeIDs {
		if id == v.themeID {
			v.themes.Menu.SetSelectPos(i)
			v.themes.Edit.SetText(names[i])
		}
	}
	v.colors.Menu.Items = nil
	v.colorIDs = []string{}
	for _, set := range fileGroupDraftColorThemes(v.session.draft) {
		v.colorIDs = append(v.colorIDs, set.ID)
		label := "Colors: " + set.Name()
		v.colors.Menu.AddItem(vtui.MenuItem{Text: label})
		if set.ID == fileGroupDraftColorThemeID(v.session.draft) {
			v.colors.Menu.SetSelectPos(len(v.colorIDs) - 1)
			v.colors.Edit.SetText(label)
		}
	}
	selectedColor := fileGroupDraftColorThemeID(v.session.draft)
	if selectedColor != "" && !slices.Contains(v.colorIDs, selectedColor) {
		v.colorIDs = append(v.colorIDs, selectedColor)
		label := "Colors: " + selectedColor + " (unavailable)"
		v.colors.Menu.AddItem(vtui.MenuItem{Text: label})
		v.colors.Menu.SetSelectPos(len(v.colorIDs) - 1)
		v.colors.Edit.SetText(label)
	}
	v.colors.SetVisible(len(v.colorIDs) > 0)
	v.colors.SetDisabled(len(v.colorIDs) == 0)
	highlighter, previews, err := buildFileGroupPreviews(v.session.draft, v.themeID, v.samples)
	if err != nil {
		v.center.status = err.Error()
	} else {
		v.highlighter, v.previews = highlighter, previews
	}
	v.rows = []fileGroupOverviewRow{}
	for _, scope := range []string{"", v.themeID} {
		for _, record := range v.session.draft.Records["filegroups"] {
			if style := record.Values["group.ColorStyle"]; style != "" && style != config.App.ColorStyle {
				continue
			}
			if record.Values["group.IconTheme"] == scope {
				v.rows = append(v.rows, fileGroupOverviewRow{view: v, record: record})
			}
		}
	}
	// Show the general file/folder rules first without changing first-match
	// evaluation or the persisted priority order.
	slices.Reverse(v.rows)
	rows := make([]vtui.TableRow, len(v.rows))
	for i := range v.rows {
		for col := range v.rows[i].cells {
			v.rows[i].cells[col] = v.rows[i].cellText(col)
		}
	}
	selected := -1
	for i, r := range v.rows {
		rows[i] = r
		if r.record.ID != "" && (selected < 0 || r.record.ID == v.selectedID) {
			selected = i
			if r.record.ID == v.selectedID {
				break
			}
		}
	}
	// Fill after selection search: every row must remain represented.
	for i := range v.rows {
		rows[i] = v.rows[i]
	}
	v.table.SetRows(rows)
	if selected >= 0 {
		for pos := 0; pos < v.table.ItemCount; pos++ {
			if v.table.RowAt(pos) == selected {
				v.table.SetSelectPos(pos)
				v.table.OnSelect(pos)
				break
			}
		}
	}
	v.table.TopPos = min(top, max(0, len(v.rows)-1))
	vtui.DebugLog("FILEGROUPS: overview rebuilt rows=%d theme=%s", len(rows), v.themeID)
}

func (t *fileGroupsTable) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_F4 || e.VirtualKeyCode == vtinput.VK_RETURN {
		t.view.edit()
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_INSERT {
		t.view.toggleMark(t.SelectPos)
		return true
	}
	if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN || e.VirtualKeyCode == vtinput.VK_HOME || e.VirtualKeyCode == vtinput.VK_END {
		direction := 1
		if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_HOME {
			direction = -1
		}
		next := t.SelectPos + direction
		if e.VirtualKeyCode == vtinput.VK_HOME {
			next = 0
			direction = 1
		}
		if e.VirtualKeyCode == vtinput.VK_END {
			next = t.ItemCount - 1
			direction = -1
		}
		if e.ControlKeyState&vtinput.ShiftPressed != 0 {
			t.view.toggleMark(t.SelectPos)
		}
		for next >= 0 && next < t.ItemCount && t.view.rows[t.RowAt(next)].separator != "" {
			next += direction
		}
		if next >= 0 && next < t.ItemCount {
			t.SetSelectPos(next)
			t.OnSelect(next)
		}
		return true
	}
	return t.Table.ProcessKey(e)
}

func (t *fileGroupsTable) HandleSemanticAction(a map[string]any) bool {
	i := semantic.Int(a["index"])
	action := semantic.String(a["action"])
	if action == "control.mark" {
		t.view.toggleMark(i)
		return true
	}
	if action == "control.select" || action == "control.activate" || action == "select" {
		if raw := t.RowAt(i); raw >= 0 && raw < len(t.view.rows) && t.view.rows[raw].separator != "" {
			return true
		}
	}
	return t.Table.HandleSemanticAction(a)
}

func (v *fileGroupsOverview) toggleMark(i int) {
	i = v.table.RowAt(i)
	if i < 0 || i >= len(v.rows) || v.rows[i].separator != "" {
		return
	}
	id := v.rows[i].record.ID
	v.marked[id] = !v.marked[id]
}

func (v *fileGroupsOverview) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	n := v.Group.SemanticNode(ctx)
	n["fileGroupsOverview"] = true
	children, _ := n["children"].([]map[string]any)
	for _, child := range children {
		switch semantic.String(child["id"]) {
		case vtui.SemanticID(v.themes):
			child["layoutRole"] = "overview-theme"
		case vtui.SemanticID(v.colors):
			child["layoutRole"] = "overview-color-theme"
		case vtui.SemanticID(v.actions):
			child["layoutRole"] = "overview-actions"
		case vtui.SemanticID(v.table):
			child["layoutRole"] = "overview-table"
			child["previewCatalog"] = v.catalog()
			rows, _ := child["rows"].([]map[string]any)
			for i := range rows {
				index := v.table.RowAt(i)
				if index < 0 || index >= len(v.rows) {
					continue
				}
				r := v.rows[index]
				rows[i]["id"] = r.record.ID
				rows[i]["separator"] = r.separator
				if r.separator != "" {
					continue
				}
				p := v.previews[r.record.ID]
				rows[i]["appearance"] = p.style.ToMap()
				rows[i]["directory"] = p.item.IsDir
				rows[i]["sampleName"] = p.item.Name
				rows[i]["marked"] = v.marked[r.record.ID]
			}
		}
	}
	return n
}

func (v *fileGroupsOverview) catalog() string {
	for _, t := range fileGroupDraftThemes(v.session.draft) {
		if t.ID == v.themeID {
			return t.Catalog()
		}
	}
	return ""
}

func (v *fileGroupsOverview) recordIndex() int {
	for i, r := range v.session.draft.Records["filegroups"] {
		if r.ID == v.selectedID {
			return i
		}
	}
	return -1
}
func (v *fileGroupsOverview) changed() {
	if p := v.session.draft.PreviewFunc; p != nil {
		if err := p(v.session.draft); err != nil {
			v.center.status = err.Error()
		}
	}
	v.refresh()
}
func (v *fileGroupsOverview) edit() {
	if i := v.recordIndex(); i >= 0 {
		v.openEditor(i)
	}
}
func (v *fileGroupsOverview) add() {
	r := f4settings.Record{ID: filegroups.NewID(), Values: map[string]string{"group.Name": "New group", "group.Mask": "*", "group.MaskSyntax": "far", "group.Case": "insensitive", "group.Enabled": "true", "group.IconTheme": "", "group.Icons.Enabled": "true"}}
	v.center.appendFileGroup(v.session, r)
	v.selectedID = r.ID
	v.pendingAddedID = r.ID
	v.refresh()
	v.edit()
}
func (v *fileGroupsOverview) duplicate() {
	i := v.recordIndex()
	if i < 0 {
		return
	}
	r := v.session.draft.Records["filegroups"][i]
	old := r.ID
	r.ID = filegroups.NewID()
	r.Values = maps.Clone(r.Values)
	r.Values["group.Name"] += " (copy)"
	for _, k := range []string{"Factory", "LegacyID", "FactoryFallback", "ColorStyle"} {
		delete(r.Values, "group."+k)
	}
	v.center.appendFileGroup(v.session, r)
	for _, cmd := range append([]f4settings.Record{}, v.session.draft.Records["filegroups.commands"]...) {
		if cmd.Values["command.Group"] != old {
			continue
		}
		cmd.Values = maps.Clone(cmd.Values)
		cmd.Values["command.Group"] = r.ID
		cmd.ID = r.ID + "/" + filegroups.NewID()
		v.session.draft.Records["filegroups.commands"] = append(v.session.draft.Records["filegroups.commands"], cmd)
	}
	v.selectedID = r.ID
	v.changed()
}
func (v *fileGroupsOverview) move(direction int) {
	i := v.recordIndex()
	if i < 0 {
		return
	}
	records := v.session.draft.Records["filegroups"]
	scope := records[i].Values["group.IconTheme"]
	// The visible table runs in the opposite direction to rule priority.
	direction = -direction
	for j := i + direction; j >= 0 && j < len(records); j += direction {
		if records[j].Values["group.IconTheme"] == scope {
			records[i], records[j] = records[j], records[i]
			reorderFileGroupPalette(records, scope)
			v.changed()
			return
		}
	}
}
func (v *fileGroupsOverview) remove() {
	i := v.recordIndex()
	if i < 0 {
		return
	}
	id := v.selectedID
	confirm := vtui.ShowMessageOn(v.center, "Delete group", "Remove the selected group and its applications? Changes are saved on Apply.", []string{"Delete", "Cancel"})
	confirm.OnResult = func(choice int) {
		if choice != 0 {
			return
		}
		records := v.session.draft.Records["filegroups"]
		for index, record := range records {
			if record.ID == id {
				v.session.draft.Records["filegroups"] = append(records[:index], records[index+1:]...)
				break
			}
		}
		removeFileGroupCommands(v.session.draft, id)
		delete(v.marked, id)
		delete(v.samples, id)
		v.selectedID = ""
		v.changed()
	}
}

func (v *fileGroupsOverview) restoreStandardSet() {
	confirm := vtui.ShowMessageOn(v.center, "Restore standard set", "Replace all groups and their applications, including groups in other themes, with the standard set and order? Changes are saved on Apply.", []string{"Restore", "Cancel"})
	confirm.OnResult = func(choice int) {
		if choice != 0 {
			return
		}
		rows := groupsToRecords(currentFileGroupStore().StandardGroups())
		v.session.draft.Records["filegroups"] = rows["filegroups"]
		v.session.draft.Records["filegroups.commands"] = rows["filegroups.commands"]
		v.marked = map[string]bool{}
		v.samples = map[string]fileGroupSampleData{}
		v.selectedID, v.pendingAddedID = "", ""
		v.changed()
	}
}
