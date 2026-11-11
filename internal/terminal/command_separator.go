package terminal

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// Bit 6 is unused by vtui's color and style encoding. Keeping the marker in
// the first cell lets it travel with the row through scroll, resize and reflow.
const commandSeparatorAttr uint64 = 1 << 6

// ExpectCommandEcho arms the next submitted command before the PTY can echo it.
func (tv *TerminalView) ExpectCommandEcho() {
	tv.mu.Lock()
	tv.commandEchoPending = true
	tv.mu.Unlock()
}

func (tv *TerminalView) CancelCommandEcho() {
	tv.mu.Lock()
	tv.commandEchoPending = false
	tv.mu.Unlock()
}

func (tv *TerminalView) markCommandEchoLocked() {
	tv.commandEchoPending = false
	if !tv.hasCommandEcho {
		tv.hasCommandEcho = true
		return
	}
	var previous []vtui.CharInfo
	if tv.CursorY > 0 {
		previous = tv.Lines[tv.CursorY-1]
	} else if len(tv.GridHistory) > 0 {
		previous = tv.GridHistory[len(tv.GridHistory)-1]
	}
	if len(previous) > 0 && isCommandSeparatorBlank(previous) {
		previous[0].Attributes |= commandSeparatorAttr
		return
	}
	tv.Lines[tv.CursorY][0].Attributes |= commandSeparatorAttr
}

func isLogTrailingBlank(cell vtui.CharInfo) bool {
	cell.Attributes &^= commandSeparatorAttr
	return isTrailingBlank(cell)
}

func isCommandSeparatorBlank(cells []vtui.CharInfo) bool {
	for _, cell := range cells {
		if !isLogTrailingBlank(cell) {
			return false
		}
	}
	return true
}

func commandSeparatorInCells(cells []vtui.CharInfo) string {
	if len(cells) == 0 || cells[0].Attributes&commandSeparatorAttr == 0 {
		return ""
	}
	if isCommandSeparatorBlank(cells) {
		return "center"
	}
	return "top"
}

func terminalCellPresentation(cells []vtui.CharInfo, width int) ([]extui.RunModel, string) {
	separator := commandSeparatorInCells(cells)
	if separator == "center" {
		return nil, separator
	}
	cells = terminalTrimmedCells(cells, width)
	if separator != "" && len(cells) > 0 {
		cells = append([]vtui.CharInfo{}, cells...)
		cells[0].Attributes &^= commandSeparatorAttr
	}
	return semantic.RunsFromCells(cells), separator
}
