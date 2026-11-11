package terminal

import (
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

// Bit 6 is unused by vtui's color and style encoding. Keeping the marker in
// the first cell lets it travel with the row through scroll, resize and reflow.
const commandSeparatorAttr uint64 = 1 << 6

// An indexed background has no RGB payload. Use one of those bits to tell
// a display-only spacer apart from an actual blank line emitted by the shell.
const commandSeparatorSpacerAttr = commandSeparatorAttr | 1<<63

const commandSeparatorSpacer = "spacer"

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

func (tv *TerminalView) spaceCleanCommandEchoLocked() {
	if !tv.commandEchoPending || !tv.hasCommandEcho || tv.UseAltScreen {
		return
	}
	// The cursor may be on a wrapped prompt. Leave the whole prompt intact
	// and make room above it without moving the cursor or the rows below it.
	start := tv.CursorY
	for start > 0 && tv.WrapFlags[start-1] {
		start--
	}
	var previous []vtui.CharInfo
	if start > 0 {
		previous = tv.Lines[start-1]
	} else if len(tv.GridHistory) > 0 {
		previous = tv.GridHistory[len(tv.GridHistory)-1]
	}
	if len(previous) > 0 && isCommandSeparatorBlank(previous) {
		previous[0].Attributes |= commandSeparatorAttr
	} else {
		switch start {
		case 0:
			spacer := blankRow(tv.Width)
			spacer[0].Attributes |= commandSeparatorSpacerAttr
			tv.pushCellsToGridHistory(spacer, false)
		case 1:
			tv.pushRowToGridHistory(0)
			tv.Lines[0] = blankRow(tv.Width)
			tv.Lines[0][0].Attributes |= commandSeparatorSpacerAttr
		default:
			tv.scrollUp(0, start-1, 1)
			tv.Lines[start-1][0].Attributes |= commandSeparatorSpacerAttr
		}
	}
	tv.commandEchoPending = false
}

func isLogTrailingBlank(cell vtui.CharInfo) bool {
	if cell.Attributes == DefaultTermAttr|commandSeparatorSpacerAttr {
		cell.Attributes = DefaultTermAttr
	} else {
		cell.Attributes &^= commandSeparatorAttr
	}
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
		if cells[0].Attributes == DefaultTermAttr|commandSeparatorSpacerAttr {
			return commandSeparatorSpacer
		}
		return "center"
	}
	return "top"
}

func terminalCellPresentation(cells []vtui.CharInfo, width int) ([]extui.RunModel, string) {
	separator := commandSeparatorInCells(cells)
	if separator == "center" || separator == commandSeparatorSpacer {
		return nil, "center"
	}
	cells = terminalTrimmedCells(cells, width)
	if separator != "" && len(cells) > 0 {
		cells = append([]vtui.CharInfo{}, cells...)
		cells[0].Attributes &^= commandSeparatorAttr
	}
	return semantic.RunsFromCells(cells), separator
}
