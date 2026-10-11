package vtui

import (
	"strings"
	"unicode"

	"github.com/unxed/vtinput"
)

// The item filter of far2l's VMenu (vmenu.cpp: EnableFilter,
// FilterStringUpdated, ShouldSendKeyToFilter), reached the same way:
//
//	Ctrl+Alt+F  turn the filter on or off
//	Ctrl+Alt+L  lock it, so letters reach the menu's hotkeys again
//
// While it is on and unlocked, printable keys, Backspace, Ctrl+V and
// Shift+Ins edit the filter string before the menu's OnKeyDown sees them, and
// only the items whose text contains that string, ignoring case, are shown.
// The title carries the string as "[text]", or "<text>" once locked.
//
// Filtering hides rows; it never touches Items. SelectPos, OnAction, the exit
// code and every index a consumer keeps stay item indices, so code written
// for an unfiltered menu — e.History[menu.SelectPos], a map from row to
// bookmark slot — keeps working on a filtered one. While rows are hidden,
// filterTop is the first row drawn; TopPos keeps counting items and is
// reconciled when the filter lets go.

// filterSupported reports whether the filter can act on this menu at all.
// A virtual menu (ItemCount without backing Items) has no text to match.
func (m *VMenu) filterSupported() bool {
	return !m.DisableFilter && len(m.Items) == m.ItemCount
}

// filtering reports whether the filter hides rows right now. An enabled
// filter with an empty string shows everything and needs no row mapping.
func (m *VMenu) filtering() bool {
	return m.filterOn && len(m.filterText) > 0 && m.filterSupported()
}

// FilterText returns the current filter string, empty when the filter is off.
func (m *VMenu) FilterText() string {
	if !m.filterOn {
		return ""
	}
	return string(m.filterText)
}

// ClearFilter turns the item filter off and shows every item again. Code
// that selects an item by an index it did not take from the shown rows — a
// native GUI activating an entry of the full list — clears it first, or the
// selection would be steered away from a hidden item.
func (m *VMenu) ClearFilter() {
	if m.filterOn {
		m.setFilter(false)
	}
}

// visibleRows returns the indices of the items the filter lets through, in
// menu order. A separator survives only between two shown items, the way
// far2l hides the separators that no longer divide anything.
func (m *VMenu) visibleRows() []int {
	needle := strings.ToLower(string(m.filterText))
	rows := make([]int, 0, len(m.Items))
	pendingSep := -1
	for i, item := range m.Items {
		if item.Separator {
			if len(rows) > 0 && !m.Items[rows[len(rows)-1]].Separator {
				pendingSep = i
			}
			continue
		}
		clean, _, _ := ParseAmpersandString(item.Text)
		if !strings.Contains(strings.ToLower(clean), needle) {
			continue
		}
		if pendingSep >= 0 {
			rows = append(rows, pendingSep)
			pendingSep = -1
		}
		rows = append(rows, i)
	}
	return rows
}

func rowOfItem(rows []int, item int) int {
	for r, idx := range rows {
		if idx == item {
			return r
		}
	}
	return -1
}

func (m *VMenu) rowSelectable(rows []int, r int) bool {
	if r < 0 || r >= len(rows) {
		return false
	}
	return m.IsSelectable == nil || m.IsSelectable(rows[r])
}

// firstSelectableRow returns the first selectable row at or after from
// walking in dir, or -1.
func (m *VMenu) firstSelectableRow(rows []int, from, dir int) int {
	for r := from; r >= 0 && r < len(rows); r += dir {
		if m.rowSelectable(rows, r) {
			return r
		}
	}
	return -1
}

// steerSelection keeps SelectPos on a shown, selectable item: when the item
// it names is hidden it moves to the nearest shown one after it in menu
// order, else before it. With nothing selectable shown it is left alone;
// ProcessKey then withholds the keys that would act on it.
func (m *VMenu) steerSelection(rows []int) {
	if r := rowOfItem(rows, m.SelectPos); m.rowSelectable(rows, r) {
		m.ensureRowVisible(rows, r)
		return
	}
	target := -1
	for r, idx := range rows {
		if idx > m.SelectPos && m.rowSelectable(rows, r) {
			target = r
			break
		}
	}
	if target < 0 {
		for r := len(rows) - 1; r >= 0; r-- {
			if rows[r] < m.SelectPos && m.rowSelectable(rows, r) {
				target = r
				break
			}
		}
	}
	if target >= 0 {
		m.SelectPos = rows[target]
		m.ensureRowVisible(rows, target)
	} else {
		m.clampFilterTop(len(rows))
	}
}

func (m *VMenu) hasSelectableRow(rows []int) bool {
	return m.firstSelectableRow(rows, 0, 1) >= 0
}

func (m *VMenu) clampFilterTop(n int) {
	maxTop := n - m.ViewHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if m.filterTop > maxTop {
		m.filterTop = maxTop
	}
	if m.filterTop < 0 {
		m.filterTop = 0
	}
}

func (m *VMenu) ensureRowVisible(rows []int, r int) {
	if m.ViewHeight > 0 {
		if r < m.filterTop {
			m.filterTop = r
		} else if r >= m.filterTop+m.ViewHeight {
			m.filterTop = r - m.ViewHeight + 1
		}
	}
	m.clampFilterTop(len(rows))
}

// GetClickIndex returns the item under screen row my, or -1.
func (m *VMenu) GetClickIndex(my int) int {
	if !m.filtering() {
		return m.ScrollView.GetClickIndex(my)
	}
	relY := my - (m.Y1 + m.MarginTop)
	if relY < 0 || relY >= m.ViewHeight {
		return -1
	}
	rows := m.visibleRows()
	if r := m.filterTop + relY; r < len(rows) {
		return rows[r]
	}
	return -1
}

// DrawScrollBar draws the scrollbar over the rows that are actually shown.
func (m *VMenu) DrawScrollBar(scr *ScreenBuf) {
	if !m.filtering() {
		m.ScrollView.DrawScrollBar(scr)
		return
	}
	if m.ScrollBar == nil {
		return
	}
	n := len(m.visibleRows())
	if m.ShowScrollBar && m.ViewHeight > 0 && n > m.ViewHeight {
		m.ScrollBar.SetParams(m.filterTop, 0, n-m.ViewHeight)
		m.ScrollBar.Show(scr)
		return
	}
	// Nothing to scroll: park the bar so a click in its column is not taken
	// for a drag over the rows the filter hid.
	m.ScrollBar.SetParams(0, 0, 0)
}

// HandleKey moves the selection for the navigation keys, over the shown rows
// while the filter hides items.
func (m *VMenu) HandleKey(e *vtinput.InputEvent) bool {
	if !m.filtering() {
		return m.ScrollView.HandleKey(e)
	}
	if !e.KeyDown {
		return false
	}
	rows := m.visibleRows()
	old := m.SelectPos
	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		m.stepFiltered(rows, -1)
	case vtinput.VK_DOWN:
		m.stepFiltered(rows, 1)
	case vtinput.VK_PRIOR:
		m.scrollFilteredBy(rows, -max(m.ViewHeight, 1))
	case vtinput.VK_NEXT:
		m.scrollFilteredBy(rows, max(m.ViewHeight, 1))
	case vtinput.VK_HOME:
		if r := m.firstSelectableRow(rows, 0, 1); r >= 0 {
			m.SelectPos = rows[r]
			m.ensureRowVisible(rows, r)
		}
	case vtinput.VK_END:
		if r := m.firstSelectableRow(rows, len(rows)-1, -1); r >= 0 {
			m.SelectPos = rows[r]
			m.ensureRowVisible(rows, r)
		}
	default:
		return false
	}
	if m.SelectPos != old && m.OnSelect != nil {
		m.OnSelect(m.SelectPos)
	}
	return true
}

// stepFiltered is MoveSelection(dir) over the shown rows, wrapping when
// Wrap is set.
func (m *VMenu) stepFiltered(rows []int, dir int) {
	n := len(rows)
	if n == 0 {
		return
	}
	r := rowOfItem(rows, m.SelectPos)
	if r < 0 {
		if dir > 0 {
			r = -1
		} else {
			r = n
		}
	}
	for k := 0; k < n; k++ {
		r += dir
		if r < 0 || r >= n {
			if !m.Wrap {
				return
			}
			r = (r + n) % n
		}
		if m.rowSelectable(rows, r) {
			m.SelectPos = rows[r]
			m.ensureRowVisible(rows, r)
			return
		}
	}
}

// scrollFilteredBy is ScrollBy over the shown rows: the view and the
// selection move together and stop at the ends.
func (m *VMenu) scrollFilteredBy(rows []int, delta int) {
	n := len(rows)
	if n == 0 || delta == 0 {
		return
	}
	m.filterTop += delta
	m.clampFilterTop(n)

	r := rowOfItem(rows, m.SelectPos)
	if r < 0 {
		r = 0
	}
	r = min(max(r+delta, 0), n-1)
	dir := 1
	if delta < 0 {
		dir = -1
	}
	target := m.firstSelectableRow(rows, r, dir)
	if target < 0 {
		target = m.firstSelectableRow(rows, r, -dir)
	}
	if target >= 0 {
		m.SelectPos = rows[target]
		m.ensureRowVisible(rows, target)
	}
}

// rowOffset returns how many rows below the top of the view the item at
// index is drawn.
func (m *VMenu) rowOffset(index int) int {
	if !m.filtering() {
		return index - m.TopPos
	}
	return rowOfItem(m.visibleRows(), index) - m.filterTop
}

// displayTitle is the title with the filter string appended, the way far2l
// draws it: the menu title gives way first when the box is too narrow.
func (m *VMenu) displayTitle() string {
	if !m.filterOn || !m.filterSupported() {
		return m.title
	}
	tag := "[" + string(m.filterText) + "]"
	if m.filterLocked {
		tag = "<" + string(m.filterText) + ">"
	}
	if m.title == "" {
		return tag
	}
	room := (m.X2 - m.X1 + 1) - 4 - StringWidth(tag) - 1
	switch {
	case m.X2 <= m.X1 || room >= StringWidth(m.title):
		return m.title + " " + tag
	case room >= 4:
		return TruncateString(m.title, room, "…") + " " + tag
	default:
		return tag
	}
}

func menuKeyMods(e *vtinput.InputEvent) (ctrl, alt, shift bool) {
	ctrl = e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt = e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift = e.ControlKeyState&vtinput.ShiftPressed != 0
	return
}

// isFilterRune reports whether the key types a character into the filter:
// the same test Edit applies to text input.
func isFilterRune(e *vtinput.InputEvent) bool {
	ctrl, alt, _ := menuKeyMods(e)
	return e.Char != 0 && !ctrl && !alt && (unicode.IsGraphic(e.Char) || e.Char == ' ')
}

// processFilterKey gives the filter its keys ahead of the menu, as far2l's
// VMenu::ReadInput does. It reports whether the key was taken.
func (m *VMenu) processFilterKey(e *vtinput.InputEvent) bool {
	if !m.filterSupported() {
		return false
	}
	ctrl, alt, shift := menuKeyMods(e)
	vk := e.VirtualKeyCode

	if ctrl && alt && !shift && vk == vtinput.VK_F {
		m.setFilter(!m.filterOn)
		return true
	}
	if m.filterOn && ctrl && alt && !shift && vk == vtinput.VK_L {
		m.filterLocked = !m.filterLocked
		FrameManager.Redraw()
		return true
	}
	if m.filterLocked {
		return false
	}
	if !m.filterOn {
		if !m.FilterOnType || !isFilterRune(e) {
			return false
		}
		m.filterOn = true
	}

	switch {
	case vk == vtinput.VK_BACK && !ctrl && !alt && !shift:
		if n := len(m.filterText); n > 0 {
			m.filterText = m.filterText[:n-1]
			m.filterChanged()
		}
		return true
	case (vk == vtinput.VK_V && ctrl && !alt && !shift) || (vk == vtinput.VK_INSERT && shift && !ctrl && !alt):
		added := false
		for _, r := range GetClipboard() {
			if unicode.IsGraphic(r) || r == ' ' {
				m.filterText = append(m.filterText, r)
				added = true
			}
		}
		if added {
			m.filterChanged()
		}
		return true
	case isFilterRune(e):
		// far2l stops taking characters once nothing is left to narrow.
		if m.filtering() && !m.hasSelectableRow(m.visibleRows()) {
			return true
		}
		m.filterText = append(m.filterText, e.Char)
		m.filterChanged()
		return true
	}
	return false
}

// setFilter turns the filter on or off; either way it starts empty.
func (m *VMenu) setFilter(on bool) {
	wasFiltering := m.filtering()
	m.filterOn = on
	m.filterLocked = false
	m.filterText = nil
	m.filterTop = 0
	if wasFiltering {
		m.ScrollView.SetSelectPos(m.SelectPos)
	}
	FrameManager.Redraw()
}

// filterChanged re-applies the filter after its string changed. The
// selection stays on its item while that is shown, otherwise it goes to the
// first shown item, as far2l's SetSelectPos(0, 1) does.
func (m *VMenu) filterChanged() {
	old := m.SelectPos
	if !m.filtering() {
		m.ScrollView.SetSelectPos(m.SelectPos)
	} else {
		rows := m.visibleRows()
		r := rowOfItem(rows, m.SelectPos)
		if !m.rowSelectable(rows, r) {
			r = m.firstSelectableRow(rows, 0, 1)
		}
		if r >= 0 {
			m.SelectPos = rows[r]
			m.ensureRowVisible(rows, r)
		} else {
			m.filterTop = 0
		}
	}
	if m.SelectPos != old && m.OnSelect != nil {
		m.OnSelect(m.SelectPos)
	}
	FrameManager.Redraw()
}

// filterBlocksKey reports whether a key must not reach the menu because the
// filter hides every item it could act on: SelectPos then names a hidden
// item, and Enter or a consumer's Shift+Del would take that one. Esc, F10,
// F1 and Left still get through, so the menu can be left.
func (m *VMenu) filterBlocksKey(e *vtinput.InputEvent) bool {
	if !m.filtering() || m.hasSelectableRow(m.visibleRows()) {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10, vtinput.VK_F1, vtinput.VK_LEFT:
		return false
	}
	return true
}
