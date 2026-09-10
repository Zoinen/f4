package app

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// commandPaletteDialog keeps the query editor focused while the table acts as
// a cursor-controlled result view. This mirrors command palettes in graphical
// applications: typing never requires moving focus away from the search box.
type CommandPaletteDialog struct {
	*vtui.Window

	query       *vtui.Edit
	queryPrompt *vtui.Text
	table       *vtui.Table
	description *vtui.Text

	entries  []commandPaletteEntry
	filtered []commandPaletteEntry
	recent   []string

	onExecute func(commandPaletteEntry)
	lastQuery string

	tableMouseCaptured bool
}

type commandPaletteRow struct {
	entry commandPaletteEntry
	empty bool
}

func (row commandPaletteRow) GetCellText(column int) string {
	if row.empty {
		if column == 0 {
			return i18n.Msg("CommandPalette.Empty")
		}
		return ""
	}

	switch column {
	case 0:
		label := commandPaletteDisplayLabel(row.entry)
		if row.entry.Checked {
			label = action.PlainLabel(i18n.Msg("CommandPalette.CheckedPrefix")) + " " + label
		}
		return label
	case 1:
		return row.entry.Category
	case 2:
		return row.entry.Shortcut
	default:
		return ""
	}
}

func commandPaletteDisplayLabel(entry commandPaletteEntry) string {
	for _, label := range []string{entry.Label, entry.EnglishLabel, entry.Key, entry.ID} {
		if label != "" {
			return action.PlainLabel(label)
		}
	}
	return ""
}

func commandPaletteDisplayDescription(entry commandPaletteEntry) string {
	description := entry.Description
	if description == "" {
		description = entry.EnglishDescription
	}
	if entry.ID == "" || strings.EqualFold(strings.TrimSpace(description), strings.TrimSpace(entry.ID)) {
		return description
	}
	if description == "" {
		return entry.ID
	}
	return fmt.Sprintf(i18n.Msg("CommandPalette.DescriptionWithID"), description, entry.ID)
}

// newCommandPaletteDialog creates a self-contained modal command picker. It
// deliberately accepts entries and an executor so filtering and UI behavior
// can be tested without constructing panel.PanelsFrame (and therefore ConPTY).
func newCommandPaletteDialog(
	entries []commandPaletteEntry,
	recent []string,
	onExecute func(commandPaletteEntry),
) *CommandPaletteDialog {
	width, height := commandPaletteDialogSize(len(entries))
	window := vtui.NewCenteredDialog(width, height, i18n.Msg("CommandPalette.Title"))
	window.ShowClose = true

	contentWidth := max(1, width-4)
	queryPrompt := vtui.NewText(0, 0, i18n.Msg("CommandPalette.QueryPrompt"), 0)
	query := vtui.NewEdit(0, 0, max(1, contentWidth-2), "")
	table := vtui.NewTable(0, 0, contentWidth, max(1, height-4), commandPaletteColumns(width))
	theme.UseTableColors(table)
	table.ShowScrollBar = true
	table.AlwaysShowCursor = true
	table.SetCanFocus(false)
	if table.ScrollBar != nil {
		table.ScrollBar.ColorIdx = vtui.ColDialogBox
	}
	description := vtui.NewText(0, 0, "", 0)

	d := &CommandPaletteDialog{
		Window:      window,
		query:       query,
		queryPrompt: queryPrompt,
		table:       table,
		description: description,
		entries:     append([]commandPaletteEntry(nil), entries...),
		recent:      append([]string(nil), recent...),
		onExecute:   onExecute,
	}

	for _, item := range []vtui.UIElement{queryPrompt, query, table, description} {
		window.AddItem(item)
	}

	d.layoutControls()

	query.OnTextChange = func(text string) {
		d.refilter(text)
	}
	table.OnSelect = func(int) {
		d.refreshDescription()
	}

	d.refilter("")
	window.SetFocusedItem(query)
	return d
}

func commandPaletteDialogSize(entryCount int) (int, int) {
	screenWidth, screenHeight := 80, 25
	if vtui.FrameManager != nil {
		if width := vtui.FrameManager.GetScreenSize(); width > 0 {
			screenWidth = width
		}
		if height := vtui.FrameManager.GetScreenHeight(); height > 0 {
			screenHeight = height
		}
	}

	return commandPaletteDialogSizeForScreen(entryCount, screenWidth, screenHeight)
}

func commandPaletteDialogSizeForScreen(entryCount, screenWidth, screenHeight int) (int, int) {
	if screenWidth < 1 {
		screenWidth = 1
	}
	if screenHeight < 1 {
		screenHeight = 1
	}

	width := screenWidth - 4
	if width > 110 {
		width = 110
	}
	if width < 60 {
		width = 60
		if width > screenWidth {
			width = screenWidth
		}
	}

	maxHeight := screenHeight - 4
	// Below ten terminal rows, keeping the usual two-row outer margin
	// would leave no usable result line. Use the whole screen instead.
	if maxHeight < 10 {
		maxHeight = screenHeight
	}
	// Normal layout needs nine non-data rows: two borders, query and
	// description lines, table header, and spacing. This avoids showing a
	// scrollbar when every result actually fits.
	height := entryCount + 9
	if height < 10 {
		height = 10
	}
	if height > maxHeight {
		height = maxHeight
	}
	if height < 1 {
		height = 1
	}
	return width, height
}

func commandPaletteColumns(dialogWidth int) []vtui.TableColumn {
	// Three columns, their separators, the command minimum and a scrollbar
	// need 49 d cells. Narrow terminals keep the useful command column
	// and expose the other metadata in the description line/search index.
	if dialogWidth < 49 {
		return []vtui.TableColumn{{Title: i18n.Msg("CommandPalette.ColumnCommand"), Width: 0, MinWidth: 1}}
	}
	return []vtui.TableColumn{
		{Title: i18n.Msg("CommandPalette.ColumnCommand"), Width: 0, MinWidth: 12},
		{Title: i18n.Msg("CommandPalette.ColumnCategory"), Width: 16},
		{Title: i18n.Msg("CommandPalette.ColumnShortcut"), Width: 14},
	}
}

func (d *CommandPaletteDialog) layoutControls() {
	if d == nil || d.Window == nil {
		return
	}
	width := d.X2 - d.X1 + 1
	height := d.Y2 - d.Y1 + 1
	inset := 2
	left, right := d.X1+inset, d.X2-inset
	if right < left {
		right = left
	}

	queryY := d.Y1 + 2
	tableY1, tableY2 := d.Y1+4, d.Y2-4
	descriptionY := d.Y2 - 2
	showDescription := true
	showQuery := true
	if height < 10 {
		queryY = d.Y1 + 2
		tableY1 = d.Y1 + 3
		descriptionY = d.Y2 - 2
		tableY2 = d.Y2 - 3
		showDescription = height >= 7
		if !showDescription {
			tableY2 = d.Y2 - 2
		}
		showQuery = height >= 6
		if !showQuery {
			tableY1 = d.Y1 + 2
		}
	}
	if tableY1 > d.Y2-1 {
		tableY1 = d.Y2 - 1
	}
	if tableY2 < tableY1 {
		tableY2 = tableY1
	}

	d.queryPrompt.SetVisible(showQuery && queryY > d.Y1 && queryY < d.Y2)
	d.queryPrompt.SetPosition(left, queryY, left, queryY)
	editLeft := left + 2
	if editLeft > right {
		editLeft = left
	}
	d.query.SetVisible(showQuery)
	d.query.SetPosition(editLeft, queryY, right, queryY)
	d.table.Columns = commandPaletteColumns(width)
	d.table.ShowHeader = tableY2-tableY1+1 >= 2
	d.table.SetPosition(left, tableY1, right, tableY2)
	d.description.SetVisible(showDescription)
	d.description.SetPosition(left, descriptionY, right, descriptionY)
}

// ResizeConsole recomputes both the d and all child coordinates. The
// embedded BaseWindow implementation only recenters the old rectangle, which
// can leave a palette outside a newly narrowed terminal.
func (d *CommandPaletteDialog) ResizeConsole(screenWidth, screenHeight int) {
	width, height := commandPaletteDialogSizeForScreen(len(d.entries), screenWidth, screenHeight)
	x1, y1 := (screenWidth-width)/2, (screenHeight-height)/2
	d.SetPosition(x1, y1, x1+width-1, y1+height-1)
	d.layoutControls()
}

func (d *CommandPaletteDialog) refilter(query string) {
	if d == nil || d.table == nil {
		return
	}
	d.lastQuery = query
	d.filtered = rankCommandPaletteEntries(d.entries, query, d.recent)

	rows := make([]vtui.TableRow, 0, len(d.filtered))
	for _, entry := range d.filtered {
		rows = append(rows, commandPaletteRow{entry: entry})
	}
	if len(rows) == 0 {
		rows = append(rows, commandPaletteRow{empty: true})
	}
	d.table.SetRows(rows)
	d.table.SetSelectPos(0)
	d.refreshDescription()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func (d *CommandPaletteDialog) refreshDescription() {
	if d == nil || d.description == nil {
		return
	}
	index := 0
	if d.table != nil {
		index = d.table.SelectPos
	}
	if index < 0 || index >= len(d.filtered) {
		d.description.SetText(dialog.EscapeAmpersand(i18n.Msg("CommandPalette.Empty")))
		return
	}
	d.description.SetText(dialog.EscapeAmpersand(commandPaletteDisplayDescription(d.filtered[index])))
}

func (d *CommandPaletteDialog) executeCurrent() bool {
	if d == nil || d.table == nil {
		return false
	}
	index := d.table.SelectPos
	if index < 0 || index >= len(d.filtered) {
		return false
	}
	entry := d.filtered[index]
	d.Close()
	if d.onExecute != nil {
		d.onExecute(entry)
	}
	return true
}

func (d *CommandPaletteDialog) ProcessKey(event *vtinput.InputEvent) bool {
	if event == nil {
		return false
	}
	if event.KeyDown {
		switch event.VirtualKeyCode {
		case vtinput.VK_ESCAPE:
			d.Close()
			return true
		case vtinput.VK_RETURN:
			d.executeCurrent()
			return true
		case vtinput.VK_UP, vtinput.VK_DOWN,
			vtinput.VK_PRIOR, vtinput.VK_NEXT,
			vtinput.VK_HOME, vtinput.VK_END:
			// Consume navigation even at a boundary so it never escapes the
			// result view and changes focus away from the query editor.
			d.table.ProcessKey(event)
			d.refreshDescription()
			return true
		}
	}

	before := d.query.GetText()
	handled := d.Window.ProcessKey(event)
	after := d.query.GetText()
	// Edit normally calls OnTextChange itself. This comparison also covers
	// edit operations such as cutting a selection whose vtui path currently
	// changes the value without notifying OnTextChange.
	if after != before && after != d.lastQuery {
		d.refilter(after)
	}
	return handled
}

func (d *CommandPaletteDialog) ProcessMouse(event *vtinput.InputEvent) bool {
	if event == nil || event.Type != vtinput.MouseEventType {
		return false
	}
	mx, my := int(event.MouseX), int(event.MouseY)
	tableHit := d.table != nil && d.table.HitTest(mx, my)
	captured := d.tableMouseCaptured
	if tableHit || captured {
		before := d.table.SelectPos
		clickedIndex := -1
		if tableHit && mx < d.table.X1+d.table.GetContentWidth() {
			clickedIndex = d.table.GetClickIndex(my)
		}
		handled := d.table.ProcessMouse(event)
		if event.KeyDown && event.ButtonState != 0 && tableHit {
			d.tableMouseCaptured = true
		}
		if event.ButtonState == 0 {
			d.tableMouseCaptured = false
		}
		if d.table.SelectPos != before {
			d.refreshDescription()
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
		}
		isLeftDoubleClick := event.KeyDown &&
			event.ButtonState == vtinput.FromLeft1stButtonPressed &&
			(event.MouseEventFlags&vtinput.DoubleClick) != 0
		if isLeftDoubleClick && clickedIndex >= 0 && clickedIndex < len(d.filtered) {
			d.executeCurrent()
			return true
		}
		// Header and empty-row clicks must not fall through to BaseWindow,
		// where they would begin dragging the d. Middle click only selects.
		return handled || tableHit || captured
	}
	return d.Window.ProcessMouse(event)
}
