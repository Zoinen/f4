package panel

// The wheel ramp itself -- how a fast spin queues the lines a panel then
// scrolls on its own -- lives in internal/wheel, shared with the editor,
// the viewer and every other view that scrolls under the pointer. This file
// keeps the one piece only a panel has: moving its cursor and its scroll
// position over its entries.

// wheelScrollBy moves the cursor and the scroll position by step entries,
// positive down the list, and reports whether anything moved so the coast
// stops at an end of the list instead of spinning in place.
func (fp *FileSystemPanel) wheelScrollBy(step int) bool {
	if len(fp.Entries) == 0 || step == 0 {
		fp.Refresh()
		return false
	}
	beforeIdx := fp.GetCursorIndex()
	beforeTop := fp.Table.TopPos

	direction := 1
	if step < 0 {
		direction = -1
	}

	if fp.GroupBy != GroupNone {
		target := fp.nearestDisplayEntry(fp.displayOfEntry(beforeIdx)+step, direction)
		fp.setPanelScrollTop(beforeTop + step)
		fp.SetCursorIndex(target)
		fp.Refresh()
		return fp.GetCursorIndex() != beforeIdx || fp.Table.TopPos != beforeTop
	}

	H := fp.Table.ViewHeight
	if H <= 0 {
		H = 1
	}
	newIdx := beforeIdx + step
	if newIdx < 0 {
		newIdx = 0
	}
	if newIdx >= len(fp.Entries) {
		newIdx = len(fp.Entries) - 1
	}
	newTop := beforeTop + step
	maxTop := len(fp.Entries) - fp.gridColumnCount()*H
	if maxTop < 0 {
		maxTop = 0
	}
	if newTop < 0 {
		newTop = 0
	}
	if newTop > maxTop {
		newTop = maxTop
	}

	fp.Table.TopPos = newTop
	fp.SetCursorIndex(newIdx)
	fp.Refresh()
	return newIdx != beforeIdx || newTop != beforeTop
}
