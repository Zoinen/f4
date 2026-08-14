package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// commandPaletteDialog keeps the query editor focused while the table acts as
// a cursor-controlled result view. This mirrors command palettes in graphical
// applications: typing never requires moving focus away from the search box.
type commandPaletteDialog struct {
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
			return Msg("CommandPalette.Empty")
		}
		return ""
	}

	switch column {
	case 0:
		label := commandPaletteDisplayLabel(row.entry)
		if row.entry.Checked {
			label = plainLabel(Msg("CommandPalette.CheckedPrefix")) + " " + label
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
			return plainLabel(label)
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
	return fmt.Sprintf(Msg("CommandPalette.DescriptionWithID"), description, entry.ID)
}

// newCommandPaletteDialog creates a self-contained modal command picker. It
// deliberately accepts entries and an executor so filtering and UI behavior
// can be tested without constructing PanelsFrame (and therefore ConPTY).
func newCommandPaletteDialog(
	entries []commandPaletteEntry,
	recent []string,
	onExecute func(commandPaletteEntry),
) *commandPaletteDialog {
	width, height := commandPaletteDialogSize(len(entries))
	window := vtui.NewCenteredDialog(width, height, Msg("CommandPalette.Title"))
	window.ShowClose = true

	contentWidth := max(1, width-4)
	queryPrompt := vtui.NewText(0, 0, Msg("CommandPalette.QueryPrompt"), 0)
	query := vtui.NewEdit(0, 0, max(1, contentWidth-2), "")
	table := vtui.NewTable(0, 0, contentWidth, max(1, height-4), commandPaletteColumns(width))
	useDialogTableColors(table)
	table.ShowScrollBar = true
	table.AlwaysShowCursor = true
	table.SetCanFocus(false)
	if table.ScrollBar != nil {
		table.ScrollBar.ColorIdx = vtui.ColDialogBox
	}
	description := vtui.NewText(0, 0, "", 0)

	dialog := &commandPaletteDialog{
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

	dialog.layoutControls()

	query.OnTextChange = func(text string) {
		dialog.refilter(text)
	}
	table.OnSelect = func(int) {
		dialog.refreshDescription()
	}

	dialog.refilter("")
	window.SetFocusedItem(query)
	return dialog
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
	// need 49 dialog cells. Narrow terminals keep the useful command column
	// and expose the other metadata in the description line/search index.
	if dialogWidth < 49 {
		return []vtui.TableColumn{{Title: Msg("CommandPalette.ColumnCommand"), Width: 0, MinWidth: 1}}
	}
	return []vtui.TableColumn{
		{Title: Msg("CommandPalette.ColumnCommand"), Width: 0, MinWidth: 12},
		{Title: Msg("CommandPalette.ColumnCategory"), Width: 16},
		{Title: Msg("CommandPalette.ColumnShortcut"), Width: 14},
	}
}

func (dialog *commandPaletteDialog) layoutControls() {
	if dialog == nil || dialog.Window == nil {
		return
	}
	width := dialog.X2 - dialog.X1 + 1
	height := dialog.Y2 - dialog.Y1 + 1
	inset := 2
	left, right := dialog.X1+inset, dialog.X2-inset
	if right < left {
		right = left
	}

	queryY := dialog.Y1 + 2
	tableY1, tableY2 := dialog.Y1+4, dialog.Y2-4
	descriptionY := dialog.Y2 - 2
	showDescription := true
	showQuery := true
	if height < 10 {
		queryY = dialog.Y1 + 2
		tableY1 = dialog.Y1 + 3
		descriptionY = dialog.Y2 - 2
		tableY2 = dialog.Y2 - 3
		showDescription = height >= 7
		if !showDescription {
			tableY2 = dialog.Y2 - 2
		}
		showQuery = height >= 6
		if !showQuery {
			tableY1 = dialog.Y1 + 2
		}
	}
	if tableY1 > dialog.Y2-1 {
		tableY1 = dialog.Y2 - 1
	}
	if tableY2 < tableY1 {
		tableY2 = tableY1
	}

	dialog.queryPrompt.SetVisible(showQuery && queryY > dialog.Y1 && queryY < dialog.Y2)
	dialog.queryPrompt.SetPosition(left, queryY, left, queryY)
	editLeft := left + 2
	if editLeft > right {
		editLeft = left
	}
	dialog.query.SetVisible(showQuery)
	dialog.query.SetPosition(editLeft, queryY, right, queryY)
	dialog.table.Columns = commandPaletteColumns(width)
	dialog.table.ShowHeader = tableY2-tableY1+1 >= 2
	dialog.table.SetPosition(left, tableY1, right, tableY2)
	dialog.description.SetVisible(showDescription)
	dialog.description.SetPosition(left, descriptionY, right, descriptionY)
}

// ResizeConsole recomputes both the dialog and all child coordinates. The
// embedded BaseWindow implementation only recenters the old rectangle, which
// can leave a palette outside a newly narrowed terminal.
func (dialog *commandPaletteDialog) ResizeConsole(screenWidth, screenHeight int) {
	width, height := commandPaletteDialogSizeForScreen(len(dialog.entries), screenWidth, screenHeight)
	x1, y1 := (screenWidth-width)/2, (screenHeight-height)/2
	dialog.Window.SetPosition(x1, y1, x1+width-1, y1+height-1)
	dialog.layoutControls()
}

func (dialog *commandPaletteDialog) refilter(query string) {
	if dialog == nil || dialog.table == nil {
		return
	}
	dialog.lastQuery = query
	dialog.filtered = rankCommandPaletteEntries(dialog.entries, query, dialog.recent)

	rows := make([]vtui.TableRow, 0, len(dialog.filtered))
	for _, entry := range dialog.filtered {
		rows = append(rows, commandPaletteRow{entry: entry})
	}
	if len(rows) == 0 {
		rows = append(rows, commandPaletteRow{empty: true})
	}
	dialog.table.SetRows(rows)
	dialog.table.SetSelectPos(0)
	dialog.refreshDescription()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func (dialog *commandPaletteDialog) refreshDescription() {
	if dialog == nil || dialog.description == nil {
		return
	}
	index := 0
	if dialog.table != nil {
		index = dialog.table.SelectPos
	}
	if index < 0 || index >= len(dialog.filtered) {
		dialog.description.SetText(escapeAmpersand(Msg("CommandPalette.Empty")))
		return
	}
	dialog.description.SetText(escapeAmpersand(commandPaletteDisplayDescription(dialog.filtered[index])))
}

func (dialog *commandPaletteDialog) executeCurrent() bool {
	if dialog == nil || dialog.table == nil {
		return false
	}
	index := dialog.table.SelectPos
	if index < 0 || index >= len(dialog.filtered) {
		return false
	}
	entry := dialog.filtered[index]
	dialog.Close()
	if dialog.onExecute != nil {
		dialog.onExecute(entry)
	}
	return true
}

func (dialog *commandPaletteDialog) ProcessKey(event *vtinput.InputEvent) bool {
	if event == nil {
		return false
	}
	if event.KeyDown {
		switch event.VirtualKeyCode {
		case vtinput.VK_ESCAPE:
			dialog.Close()
			return true
		case vtinput.VK_RETURN:
			dialog.executeCurrent()
			return true
		case vtinput.VK_UP, vtinput.VK_DOWN,
			vtinput.VK_PRIOR, vtinput.VK_NEXT,
			vtinput.VK_HOME, vtinput.VK_END:
			// Consume navigation even at a boundary so it never escapes the
			// result view and changes focus away from the query editor.
			dialog.table.ProcessKey(event)
			dialog.refreshDescription()
			return true
		}
	}

	before := dialog.query.GetText()
	handled := dialog.Window.ProcessKey(event)
	after := dialog.query.GetText()
	// Edit normally calls OnTextChange itself. This comparison also covers
	// edit operations such as cutting a selection whose vtui path currently
	// changes the value without notifying OnTextChange.
	if after != before && after != dialog.lastQuery {
		dialog.refilter(after)
	}
	return handled
}

func (dialog *commandPaletteDialog) ProcessMouse(event *vtinput.InputEvent) bool {
	if event == nil || event.Type != vtinput.MouseEventType {
		return false
	}
	mx, my := int(event.MouseX), int(event.MouseY)
	tableHit := dialog.table != nil && dialog.table.HitTest(mx, my)
	captured := dialog.tableMouseCaptured
	if tableHit || captured {
		before := dialog.table.SelectPos
		clickedIndex := -1
		if tableHit && mx < dialog.table.X1+dialog.table.GetContentWidth() {
			clickedIndex = dialog.table.GetClickIndex(my)
		}
		handled := dialog.table.ProcessMouse(event)
		if event.KeyDown && event.ButtonState != 0 && tableHit {
			dialog.tableMouseCaptured = true
		}
		if event.ButtonState == 0 {
			dialog.tableMouseCaptured = false
		}
		if dialog.table.SelectPos != before {
			dialog.refreshDescription()
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
		}
		isLeftDoubleClick := event.KeyDown &&
			event.ButtonState == vtinput.FromLeft1stButtonPressed &&
			(event.MouseEventFlags&vtinput.DoubleClick) != 0
		if isLeftDoubleClick && clickedIndex >= 0 && clickedIndex < len(dialog.filtered) {
			dialog.executeCurrent()
			return true
		}
		// Header and empty-row clicks must not fall through to BaseWindow,
		// where they would begin dragging the dialog. Middle click only selects.
		return handled || tableHit || captured
	}
	return dialog.Window.ProcessMouse(event)
}
