package settings

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

const (
	fileGroupSortListToggle = "filegroups.sort-list-toggle"
	fileGroupSortListID     = "filegroups.sort-list"
)

type fileGroupSortListRow struct {
	name   string
	number int
}

func (r fileGroupSortListRow) GetCellText(column int) string {
	if column == 0 {
		return r.name
	}
	return strconv.Itoa(r.number)
}

func (c *settingsCenter) addFileGroupSortList(session *settingsSession, numberField f4settings.Field) {
	label := i18n.Msg("FileGroups.ViewExistingSortGroups")
	if c.fileGroupSortListExpanded {
		label = i18n.Msg("FileGroups.HideExistingSortGroups")
	}
	button := vtui.NewButton(0, 0, label)
	button.SetId(fileGroupSortListToggle)
	button.OnClick = func() {
		c.fileGroupSortListExpanded = !c.fileGroupSortListExpanded
		c.rebuildCategory()
		if c.fileGroupSortListExpanded {
			for _, row := range c.page.rows {
				if row.field.ID == fileGroupSortListID {
					c.page.SetFocusedItem(row.control)
					c.page.notifyFocus()
					break
				}
			}
		}
	}
	field := f4settings.Field{
		ID: fileGroupSortListToggle, Category: numberField.Category, Group: numberField.Group,
		Label: f4settings.Text{English: label, Literal: true},
	}
	c.page.AddItem(button)
	c.page.rows = append(c.page.rows, &settingsRow{
		field: field, session: session, control: button, controlHeight: 1, match: true,
	})
	if !c.fileGroupSortListExpanded {
		return
	}

	var entries []fileGroupSortListRow
	for _, record := range session.draft.Records["filegroups"] {
		if record.ID == c.fileGroupEditorID {
			continue
		}
		number, err := strconv.Atoi(strings.TrimSpace(record.Values["group.Sort.Number"]))
		if err != nil {
			continue
		}
		group := fileGroupRecordGroup(record)
		name := group.Label()
		if scope := group.IconTheme(); scope != "" {
			name += " (" + scope + ")"
		}
		if !group.Enabled("Sort") {
			name += " " + i18n.Msg("FileGroups.SortGroupDisabled")
		}
		entries = append(entries, fileGroupSortListRow{name: name, number: number})
	}
	slices.SortStableFunc(entries, func(a, b fileGroupSortListRow) int {
		return cmp.Compare(a.number, b.number)
	})
	rows := make([]vtui.TableRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, entry)
	}
	if len(rows) == 0 {
		rows = append(rows, emptyFileGroupSortListRow{})
	}
	height := min(8, len(rows)+2)
	table := vtui.NewTable(0, 0, 20, height, []vtui.TableColumn{
		{Title: i18n.Msg("FileGroups.Group"), MinWidth: 24},
		{Title: i18n.Msg("FileGroups.SortNumber"), Width: 12},
	})
	settingsDialogTable(table)
	table.SetId(fileGroupSortListID)
	table.ShowHeader = true
	table.Sortable = false
	table.SetRows(rows)
	c.page.AddItem(table)
	field.ID = table.GetId()
	field.Label = f4settings.Text{}
	c.page.rows = append(c.page.rows, &settingsRow{
		field: field, session: session, control: table, controlHeight: height, match: true,
	})
}

type emptyFileGroupSortListRow struct{}

func (emptyFileGroupSortListRow) GetCellText(column int) string {
	if column == 0 {
		return i18n.Msg("FileGroups.NoExistingSortGroups")
	}
	return ""
}
