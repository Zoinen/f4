package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := pf.GetPosition()
	node := map[string]any{
		"id":             vtui.SemanticID(pf),
		"kind":           "panels",
		"title":          strings.TrimSpace(pf.GetTitle()),
		"type":           int(pf.GetType()),
		"x":              x1,
		"y":              y1,
		"w":              x2 - x1 + 1,
		"h":              y2 - y1 + 1,
		"activePanel":    pf.activeIdx,
		"showPanels":     pf.showPanels,
		"showKeyBar":     pf.showKeyBar,
		"terminalBusy":   pf.isPtyBusy(),
		"terminalActive": !pf.showPanels,
	}

	panels := make([]map[string]any, 0, len(pf.panels))
	for i, panel := range pf.panels {
		if sp, ok := panel.(interface {
			SemanticPanelNode(ctx *vtui.SemanticContext, side int, active bool) map[string]any
		}); ok {
			panels = append(panels, sp.SemanticPanelNode(ctx, i, i == pf.activeIdx))
		}
	}
	node["panels"] = panels
	if pf.cmdLine != nil {
		node["commandLine"] = pf.cmdLine.SemanticNode(ctx)
	}
	if pf.termView != nil {
		node["terminal"] = pf.termView.SemanticNode(ctx)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		node["macroRecording"] = true
	}
	return node
}

func (pf *PanelsFrame) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "activate_panel", "panel.activate":
		side := semanticInt(action["side"])
		if side >= 0 && side < len(pf.panels) {
			pf.activeIdx = side
			pf.lastKey = 0
			return true
		}
	case "panel_cursor", "panel.cursor":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.SetCursorIndex(semanticInt(action["index"]))
			return true
		}
	case "panel_open", "panel.open":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			idx := semanticInt(action["index"])
			pf.setActivePanelForAction(action)
			fsp.SetCursorIndex(idx)
			return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
		}
	case "panel_toggle_selection", "panel.toggleSelection":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ToggleSelection(semanticInt(action["index"]))
			return true
		}
	case "panel_refresh", "panel.refresh":
		if fsp := pf.panelForSemanticAction(action); fsp != nil {
			fsp.ReadDirectory()
			return true
		}
	case "submit_command", "command.submit":
		if text := semanticString(action["text"]); text != "" && pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(text)
		}
		return pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "set_command_text", "command.setText":
		if pf.cmdLine != nil {
			pf.cmdLine.Edit.SetText(semanticString(action["text"]))
			return true
		}
	case "emit_command", "command.emit":
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	return false
}

func (pf *PanelsFrame) setActivePanelForAction(action map[string]any) {
	side := semanticInt(action["side"])
	if side >= 0 && side < len(pf.panels) {
		pf.activeIdx = side
	}
}

func (pf *PanelsFrame) panelForSemanticAction(action map[string]any) *FileSystemPanel {
	side := semanticInt(action["side"])
	if side < 0 || side >= len(pf.panels) {
		side = pf.activeIdx
	}
	if fsp, ok := pf.panels[side].(*FileSystemPanel); ok {
		return fsp
	}
	return nil
}

func (fp *FileSystemPanel) SemanticPanelNode(ctx *vtui.SemanticContext, side int, active bool) map[string]any {
	x1, y1, x2, y2 := fp.GetPosition()
	entries := make([]map[string]any, 0, len(fp.entries))
	selectedCount := 0
	var selectedSize int64
	var totalSize int64
	for i, entry := range fp.entries {
		if !entry.IsDir {
			totalSize += entry.Size
		}
		if entry.Selected {
			selectedCount++
			selectedSize += entry.Size
		}
		entries = append(entries, map[string]any{
			"index":          i,
			"name":           entry.Name,
			"size":           entry.Size,
			"sizeText":       semanticFileSize(entry),
			"isDir":          entry.IsDir,
			"isUp":           entry.Name == "..",
			"isHidden":       entry.IsHidden,
			"isExecutable":   entry.IsExecutable,
			"isCached":       entry.IsCached,
			"selected":       entry.Selected,
			"sizeCalculated": entry.SizeCalculated,
			"mtime":          entry.MTime.Format("2006-01-02 15:04"),
			"mode":           entry.Mode,
		})
	}

	return map[string]any{
		"id":            vtui.SemanticID(fp),
		"kind":          "filePanel",
		"side":          side,
		"active":        active,
		"x":             x1,
		"y":             y1,
		"w":             x2 - x1 + 1,
		"h":             y2 - y1 + 1,
		"path":          fp.vfs.GetPath(),
		"title":         fp.frame.GetTitle(),
		"viewMode":      int(fp.viewMode),
		"viewModeName":  viewModeName(fp.viewMode),
		"sortMode":      int(fp.sortMode),
		"sortModeName":  sortModeName(fp.sortMode),
		"sortReverse":   fp.sortReverse,
		"cursor":        fp.GetCursorIndex(),
		"top":           fp.table.TopPos,
		"loading":       fp.isLoading,
		"fastFind":      fp.fastFindMode,
		"fastFindText":  fp.fastFindStr,
		"selectedCount": selectedCount,
		"selectedSize":  selectedSize,
		"totalCount":    len(fp.entries),
		"totalSize":     totalSize,
		"entries":       entries,
	}
}

func semanticFileSize(entry *fileEntry) string {
	if entry.IsDir {
		if entry.SizeCalculated {
			return formatIntWithSpaces(entry.Size)
		}
		if entry.Name == ".." {
			return Msg("Panel.UpDir")
		}
		return ""
	}
	return formatIntWithSpaces(entry.Size)
}

func viewModeName(mode ViewMode) string {
	switch mode {
	case ViewModeDetailed:
		return "detailed"
	default:
		return "medium"
	}
}

func sortModeName(mode SortMode) string {
	switch mode {
	case SortExt:
		return "extension"
	case SortTime:
		return "time"
	case SortSize:
		return "size"
	case SortUnsorted:
		return "unsorted"
	default:
		return "name"
	}
}

func (cl *CommandLine) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := cl.GetPosition()
	return map[string]any{
		"id":         vtui.SemanticID(cl),
		"kind":       "commandLine",
		"x":          x1,
		"y":          y1,
		"w":          x2 - x1 + 1,
		"h":          y2 - y1 + 1,
		"visible":    cl.IsVisible(),
		"focused":    cl.IsFocused(),
		"prompt":     cl.Prompt,
		"promptRuns": semanticRunsFromCells(cl.RichPrompt),
		"text":       cl.Edit.GetText(),
		"empty":      cl.IsEmpty(),
	}
}

func (tv *TerminalView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := tv.GetPosition()
	tv.mu.Lock()
	defer tv.mu.Unlock()

	buf := tv.Lines
	if tv.UseAltScreen {
		buf = tv.AltLines
	}
	offset := 0
	if !tv.UseAltScreen {
		lowestRow := 0
		for y := tv.Height - 1; y >= 0; y-- {
			if tv.rowHasText(y) {
				lowestRow = y
				break
			}
		}
		if tv.CursorY > lowestRow {
			lowestRow = tv.CursorY
		}
		if lowestRow < tv.Height-1 {
			offset = (tv.Height - 1) - lowestRow
		}
	}

	rows := make([]map[string]any, 0, tv.Height)
	for y := 0; y < tv.Height && y < len(buf); y++ {
		drawY := y + offset
		if tv.UseAltScreen {
			drawY = y
		}
		if drawY < 0 || drawY >= tv.Height {
			continue
		}
		rows = append(rows, map[string]any{
			"index": drawY,
			"runs":  semanticRunsFromCells(buf[y]),
		})
	}

	return map[string]any{
		"id":        vtui.SemanticID(tv),
		"kind":      "terminal",
		"x":         x1,
		"y":         y1,
		"w":         x2 - x1 + 1,
		"h":         y2 - y1 + 1,
		"visible":   tv.IsVisible(),
		"focused":   tv.IsFocused(),
		"title":     tv.Title,
		"altScreen": tv.UseAltScreen,
		"busy":      tv.Muted,
		"cursorX":   tv.CursorX,
		"cursorY":   tv.CursorY + offset,
		"rows":      rows,
	}
}

func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := vv.GetPosition()
	rows := vv.semanticRows()
	mode := "text"
	if vv.HexMode {
		mode = "hex"
	}
	return map[string]any{
		"id":        vtui.SemanticID(vv),
		"kind":      "viewer",
		"title":     vv.GetTitle(),
		"path":      vv.path,
		"baseName":  semanticBaseName(vv.vfs, vv.path),
		"x":         x1,
		"y":         y1,
		"w":         x2 - x1 + 1,
		"h":         y2 - y1 + 1,
		"mode":      mode,
		"hexMode":   vv.HexMode,
		"wrapMode":  vv.WrapMode,
		"busy":      vv.Busy,
		"topOffset": vv.TopOffset,
		"size":      vv.backend.Size(),
		"rows":      rows,
	}
}

func (vv *ViewerView) semanticRows() []map[string]any {
	if vv.backend == nil {
		return nil
	}
	width := vv.X2 - vv.X1 + 1
	if vv.scrollBar != nil {
		width--
	}
	contentHeight := vv.Y2 - vv.Y1
	if width <= 0 || contentHeight <= 0 {
		return nil
	}
	if vv.Busy {
		return []map[string]any{{"index": 0, "text": " [ Loading... ] "}}
	}
	rows := make([]map[string]any, 0, contentHeight)
	if vv.HexMode {
		currOffset := vv.TopOffset &^ 0xF
		for y := 0; y < contentHeight && currOffset < vv.backend.Size(); y++ {
			data, err := vv.backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			rows = append(rows, map[string]any{
				"index":  y,
				"offset": currOffset,
				"text":   semanticHexLine(currOffset, data),
			})
			currOffset += 16
		}
		return rows
	}

	currOffset := vv.TopOffset
	for y := 0; y < contentHeight; y++ {
		if currOffset >= vv.backend.Size() {
			break
		}
		data, err := vv.backend.ReadAt(currOffset, width*4)
		if err == piecetable.ErrLoading {
			rows = append(rows, map[string]any{"index": y, "offset": currOffset, "text": " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen := semanticViewerLineLen(data, width, vv.WrapMode)
		rows = append(rows, map[string]any{"index": y, "offset": currOffset, "text": string(data[:textLen])})
		if lineLen <= 0 {
			break
		}
		currOffset += int64(lineLen)
	}
	return rows
}

func semanticHexLine(offset int64, data []byte) string {
	hexPart := ""
	asciiPart := ""
	for i := 0; i < 16; i++ {
		if i < len(data) {
			hexPart += fmt.Sprintf("%02X ", data[i])
			r := rune(data[i])
			if r < 32 || r > 126 {
				r = '.'
			}
			asciiPart += string(r)
		} else {
			hexPart += "   "
		}
		if i == 7 {
			hexPart += " "
		}
	}
	return fmt.Sprintf("%010X: %s | %s", offset, hexPart, asciiPart)
}

func semanticViewerLineLen(data []byte, width int, wrap bool) (lineLen int, textLen int) {
	visualWidth := 0
	for lineLen < len(data) {
		r, size := utf8.DecodeRune(data[lineLen:])
		if r == '\n' {
			lineLen += size
			return lineLen, textLen
		}
		if r == '\r' {
			lineLen += size
			continue
		}
		rw := runewidth.RuneWidth(r)
		if wrap && visualWidth+rw > width {
			return lineLen, textLen
		}
		visualWidth += rw
		lineLen += size
		textLen = lineLen
		if !wrap && visualWidth >= width {
			return lineLen, textLen
		}
	}
	return lineLen, textLen
}

func (ev *EditorView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	x1, y1, x2, y2 := ev.GetPosition()
	rows := ev.semanticRows()
	return map[string]any{
		"id":           vtui.SemanticID(ev),
		"kind":         "editor",
		"title":        ev.GetTitle(),
		"path":         ev.filePath,
		"baseName":     semanticBaseName(ev.vfs, ev.filePath),
		"x":            x1,
		"y":            y1,
		"w":            x2 - x1 + 1,
		"h":            y2 - y1 + 1,
		"dirty":        ev.modified,
		"saving":       ev.saving,
		"wordWrap":     ev.WordWrap,
		"overtype":     ev.overtype,
		"cursorLine":   ev.CursorLine,
		"cursorPos":    ev.CursorPos,
		"scrollTop":    ev.ScrollTopRow,
		"scrollLeft":   ev.ScrollLeft,
		"selection":    ev.selActive,
		"rows":         rows,
		"autocomplete": ev.semanticAutocomplete(),
	}
}

func (ev *EditorView) semanticRows() []map[string]any {
	if ev.pt == nil || ev.li == nil || ev.engine == nil {
		return nil
	}
	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return nil
	}
	startLogLine, startFragIdx := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	rows := make([]map[string]any, 0, height)
	for logIdx := startLogLine; logIdx < ev.li.LineCount() && len(rows) < height; logIdx++ {
		frags := ev.engine.GetFragments(logIdx)
		baseVRow := ev.engine.GetRowOffset(logIdx)
		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				continue
			}
			data, err := ev.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			text := string(data)
			if err == piecetable.ErrLoading {
				text = " [ Loading... ] "
			} else if err != nil {
				text = ""
			}
			rows = append(rows, map[string]any{
				"index":       len(rows),
				"visualRow":   baseVRow + fIdx,
				"logicalLine": logIdx,
				"offset":      frag.ByteOffsetStart,
				"text":        text,
			})
			if len(rows) >= height {
				break
			}
		}
	}
	return rows
}

func (ev *EditorView) semanticAutocomplete() map[string]any {
	if !ev.acEnabled || len(ev.acMatches) == 0 || ev.acCurrentIdx < 0 || ev.acCurrentIdx >= len(ev.acMatches) {
		return nil
	}
	match := ev.acMatches[ev.acCurrentIdx]
	if len(match) <= len(ev.acPrefix) {
		return nil
	}
	return map[string]any{
		"prefix": ev.acPrefix,
		"tail":   match[len(ev.acPrefix):],
		"index":  ev.acCurrentIdx,
	}
}

func semanticRunsFromCells(cells []vtui.CharInfo) []map[string]any {
	if len(cells) == 0 {
		return nil
	}
	runs := make([]map[string]any, 0, 8)
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, map[string]any{
			"text": b.String(),
			"attr": attr,
		})
		b.Reset()
	}
	for _, cell := range cells {
		if cell.Char == vtui.WideCharFiller {
			continue
		}
		ch := cellRune(cell.Char)
		if !haveRun {
			attr = cell.Attributes
			haveRun = true
		} else if cell.Attributes != attr {
			flush()
			attr = cell.Attributes
			haveRun = true
		}
		b.WriteRune(ch)
	}
	flush()
	return runs
}

func cellRune(ch uint64) rune {
	if ch == 0 || ch > utf8.MaxRune || (ch >= 0xD800 && ch <= 0xDFFF) {
		return ' '
	}
	return rune(ch)
}

func semanticBaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}

func semanticString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func semanticInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}
