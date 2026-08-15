package vtui

import "fmt"

// NewTableDialog creates a centered modal dialog whose body is a single
// elastic table with a button row underneath: the recurring "table with a
// toolbar" pattern. The table stretches with the window; the button row
// stays centered and anchored to the bottom edge, even after resizes.
//
// At least one column must be flexible (Width 0), otherwise the table
// cannot follow the window width and the function panics. The window is
// not allowed to shrink narrower than the button row.
//
// Behavioral options (Sortable, QuickSearch, colors, rows, handlers) stay
// with the caller.
func NewTableDialog(width, height int, title string, columns []TableColumn, buttons ...*Button) (*Window, *Table) {
	dlg := NewCenteredDialog(width, height, title)
	dlg.ShowClose = true
	table := NewTableWithButtons(&dlg.BaseWindow, columns, buttons...)
	return dlg, table
}

// NewTableWithButtons is the window-agnostic core of NewTableDialog: it
// lays out an elastic table with a centered button row at the bottom of an
// existing window, adds both to it and returns the table.
//
// BaseWindow.AddItem derives the minimum window size from item positions,
// which for a full-width table would pin the minimum at the initial size.
// This helper overrides both minima: the window may shrink until the button
// row no longer fits horizontally and a header plus one data row no longer
// fits vertically.
func NewTableWithButtons(win *BaseWindow, columns []TableColumn, buttons ...*Button) *Table {
	flexible := false
	for _, c := range columns {
		if c.Width <= 0 {
			flexible = true
			break
		}
	}
	if !flexible {
		panic(fmt.Sprintf("vtui.NewTableWithButtons: window %q needs at least one flexible column (Width 0)", win.frame.title))
	}

	width := win.X2 - win.X1 + 1
	height := win.Y2 - win.Y1 + 1

	// Button row: a centered HBox added as a real window item. On resize the
	// group's Resize moves it (GrowLoY|GrowHiY) and stretches it (GrowHiX),
	// and HBoxLayout.SetPosition re-centers the buttons inside. The buttons
	// themselves keep GrowNone: the row owns their position absolutely.
	const spacing = 2
	totalW := 0
	for _, b := range buttons {
		bx1, _, bx2, _ := b.GetPosition()
		totalW += bx2 - bx1 + 1
	}
	if len(buttons) > 1 {
		totalW += spacing * (len(buttons) - 1)
	}
	btnY := win.Y1 + height - 3
	hbox := NewHBoxLayout(win.X1+2, btnY, width-4, 1)
	hbox.HorizontalAlign = AlignCenter
	hbox.Spacing = spacing
	for _, b := range buttons {
		hbox.Add(b, Margins{}, AlignTop)
	}
	hbox.Apply()
	hbox.SetGrowMode(GrowLoY | GrowHiY | GrowHiX)

	// Table: fills everything above the button row, one blank row apart,
	// and follows the window on resize. It is added before the buttons so
	// it gets the initial focus.
	table := NewTable(win.X1+2, win.Y1+2, width-4, height-6, columns)
	table.SetGrowMode(GrowHiX | GrowHiY)
	win.AddItem(table)
	win.AddItem(hbox)
	for _, b := range buttons {
		win.AddItem(b)
	}

	// See the doc comment: the button row sets the horizontal floor, a
	// header plus one data row the vertical one.
	win.MinW = totalW + 4
	win.MinH = 8

	return table
}
