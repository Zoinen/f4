package terminal

import (
	"bytes"
	"fmt"

	"github.com/unxed/vtui"
)

// GetAllLogANSIBytes returns the terminal log with its cell colours encoded as
// SGR sequences. The text viewer can then render the snapshot in ANSI mode.
func (tv *TerminalView) GetAllLogANSIBytes() []byte {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	return tv.terminalLogANSILocked(0).data
}

// terminalLogANSISnapshot freezes colours, text and the native viewport in
// one critical section, including output still backed by terminal cells.
func (tv *TerminalView) terminalLogANSISnapshot() terminalLogSnapshot {
	tv.mu.Lock()
	defer tv.mu.Unlock()

	plain := tv.terminalLogSnapshotLocked()
	return tv.terminalLogANSILocked(plain.viewportOffset)
}

// terminalLogANSILocked requires tv.mu. plainOffset addresses the plain log;
// map only that boundary into the ANSI stream, after any preceding SGR bytes.
func (tv *TerminalView) terminalLogANSILocked(plainOffset int64) terminalLogSnapshot {
	var out bytes.Buffer
	var plainWritten int64
	viewportOffset := int64(-1)
	writeText := func(text string) {
		if viewportOffset < 0 && plainOffset >= plainWritten && plainOffset < plainWritten+int64(len(text)) {
			viewportOffset = int64(out.Len()) + plainOffset - plainWritten
		}
		out.WriteString(text)
		plainWritten += int64(len(text))
	}
	hist, _ := tv.Pt.Bytes()
	attr := DefaultTermAttr
	writeStyle := func(next uint64) {
		if next == attr {
			return
		}
		if next == DefaultTermAttr {
			out.WriteString("\x1b[0m")
		} else {
			out.WriteString(ansiLogStyle(next, &tv.Palette))
		}
		attr = next
	}
	styleIndex := 0
	applyStyles := func(offset int) {
		for styleIndex < len(tv.styles) && tv.styles[styleIndex].Offset <= offset {
			writeStyle(tv.styles[styleIndex].Attr)
			styleIndex++
		}
	}
	for i, b := range hist {
		applyStyles(i)
		if plainWritten == plainOffset {
			viewportOffset = int64(out.Len())
		}
		out.WriteByte(b)
		plainWritten++
	}
	applyStyles(len(hist))

	writeLine := func(line []vtui.CharInfo, last int) {
		for i := 0; i <= last; i++ {
			cell := line[i]
			if isWrapPad(cell) {
				continue
			}
			writeStyle(cell.Attributes)
			writeText(vtui.CellString(cell.Char))
		}
	}
	for i, line := range tv.GridHistory {
		last := len(line) - 1
		for last >= 0 && isTrailingBlank(line[last]) {
			last--
		}
		writeLine(line, last)
		if !tv.GridHistoryWrap[i] {
			writeText("\n")
		}
	}

	if !tv.UseAltScreen {
		lastValidRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lastValidRow = y
				break
			}
		}
		if tv.CursorY > lastValidRow {
			lastValidRow = tv.CursorY
		}
		firstValidRow := 0
		if len(tv.GridHistory) == 0 && tv.Pt.Size() == 0 {
			for y := 0; y <= lastValidRow; y++ {
				if tv.rowHasText(y) || y == tv.CursorY {
					firstValidRow = y
					break
				}
			}
		}
		for y := firstValidRow; y <= lastValidRow && y < tv.Height; y++ {
			line := tv.Lines[y]
			last := len(line) - 1
			for last >= 0 && isTrailingBlank(line[last]) {
				last--
			}
			writeLine(line, last)
			if !tv.WrapFlags[y] && y < lastValidRow {
				writeText("\n")
			}
		}
	}
	if attr != DefaultTermAttr {
		out.WriteString("\x1b[0m")
	}
	if viewportOffset < 0 {
		viewportOffset = int64(out.Len())
	}
	return terminalLogSnapshot{data: out.Bytes(), viewportOffset: viewportOffset}
}

func ansiLogStyle(attr uint64, palette *[256]uint32) string {
	params := []string{"0"}
	if attr&vtui.ForegroundIntensity != 0 {
		params = append(params, "1")
	}
	if attr&vtui.ForegroundDim != 0 {
		params = append(params, "2")
	}
	if attr&vtui.CommonLvbUnderscore != 0 {
		params = append(params, "4")
	}
	if attr&vtui.CommonLvbReverse != 0 {
		params = append(params, "7")
	}
	if attr&vtui.CommonLvbStrikeout != 0 {
		params = append(params, "9")
	}
	addColor := func(code string, rgb uint32, useDefault bool) {
		if useDefault {
			params = append(params, code)
			return
		}
		params = append(params, code, "2", fmt.Sprint((rgb>>16)&0xff), fmt.Sprint((rgb>>8)&0xff), fmt.Sprint(rgb&0xff))
	}
	fg := vtui.GetRGBFore(attr)
	if attr&vtui.IsFgRGB == 0 {
		fg = palette[vtui.GetIndexFore(attr)]
	}
	addColor("38", fg, attr&vtui.ForegroundDefault != 0)

	bg := vtui.GetRGBBack(attr)
	if attr&vtui.IsBgRGB == 0 {
		bg = palette[vtui.GetIndexBack(attr)]
	}
	addColor("48", bg, attr&vtui.BackgroundDefault != 0)
	return "\x1b[" + joinANSIParams(params) + "m"
}

func joinANSIParams(params []string) string {
	if len(params) == 0 {
		return "0"
	}
	var out bytes.Buffer
	for i, param := range params {
		if i > 0 {
			out.WriteByte(';')
		}
		out.WriteString(param)
	}
	return out.String()
}
