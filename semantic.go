package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// SemanticNode экспортирует PanelsFrame в семантическое дерево ShellModel
func (pf *PanelsFrame) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	shell := extui.ShellModel{
		ID:             vtui.SemanticID(pf),
		Title:          strings.TrimSpace(pf.GetTitle()),
		Mode:           "panels",
		ActivePanel:    pf.activeIdx,
		ShowPanels:     pf.showPanels,
		ShowLeftPanel:  pf.showLeftPanel,
		ShowRightPanel: pf.showRightPanel,
		ShowKeyBar:     pf.showKeyBar,
		TerminalBusy:   pf.isPtyBusy(),
		TerminalActive: !pf.showPanels,
	}
	if !pf.showPanels {
		shell.Mode = "terminal"
	}

	for i, panel := range pf.panels {
		if fsp, ok := panel.(*FileSystemPanel); ok {
			shell.Panels = append(shell.Panels, fsp.semanticPanelModel(ctx, i, i == pf.activeIdx))
		}
	}

	if pf.cmdLine != nil {
		shell.CommandLine = pf.cmdLine.semanticModel(ctx)
	}
	if pf.termView != nil {
		shell.Terminal = pf.termView.semanticModel(ctx)
	}
	if MacroMgr != nil && MacroMgr.Recording {
		shell.MacroRecording = true
	}
	if reason := pf.semanticGridFallbackReason(); reason != "" {
		shell.Fallback = true
		shell.FallbackReason = reason
	}

	return shell.ToMap()
}

func (pf *PanelsFrame) semanticGridFallbackReason() string {
	for _, panel := range pf.altPanels {
		if panel != nil {
			return "native model unavailable for " + panel.Kind() + " panel"
		}
	}
	if pf.showPanels && (!pf.showLeftPanel || !pf.showRightPanel) {
		return "native model unavailable for hidden panel layout"
	}
	if pf.showPanels && (pf.widthDecrement != 0 || pf.leftHeightDecrement != 0 || pf.rightHeightDecrement != 0) {
		return "native model unavailable for resized panel layout"
	}
	return ""
}

// HandleSemanticAction обрабатывает нативные GUI-действия для ViewerView
func (vv *ViewerView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(vv) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "viewer.scroll":
		offset := int64(semanticInt(action["offset"]))
		if offset < 0 {
			offset = 0
		}
		if offset > vv.backend.Size() {
			offset = vv.backend.Size()
		}
		if vv.HexMode {
			offset &= ^int64(0xF)
		} else {
			offset = vv.backend.FindLineStart(offset)
		}
		vv.TopOffset = offset
		return true
	case "control.focus":
		vv.SetFocus(true)
		return true
	}
	return false
}

// HandleSemanticAction глобально маршрутизирует семантические действия из внешнего GUI
func HandleSemanticAction(action map[string]any) bool {
	if action == nil {
		return false
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return vtui.FrameManager.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	if semanticString(action["action"]) == "menu_bar_activate" || semanticString(action["action"]) == "menuBar.activate" {
		if mb := vtui.FrameManager.GetActiveMenuBar(); mb != nil {
			idx := semanticInt(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				mb.Active = true
				mb.ActivateSubMenu(idx)
				vtui.FrameManager.Redraw()
				return true
			}
		}
	}

	activeIdx := vtui.FrameManager.ActiveIdx
	frames := vtui.FrameManager.GetActiveFrames(activeIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		if h, ok := frames[i].(vtui.SemanticActionHandler); ok && h.HandleSemanticAction(action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}

	target := semanticString(action["target"])
	if target == "" {
		return false
	}
	for i := len(frames) - 1; i >= 0; i-- {
		if handleSemanticFrameAction(frames[i], target, action) {
			vtui.FrameManager.Redraw()
			return true
		}
	}
	return false
}

func handleSemanticFrameAction(frame vtui.Frame, target string, action map[string]any) bool {
	if vtui.SemanticID(frame) == target {
		switch semanticString(action["action"]) {
		case "close", "dialog.close", "window.close":
			frame.Close()
			return true
		case "menu_activate", "menu.activate":
			if menu, ok := frame.(*vtui.VMenu); ok {
				idx := semanticInt(action["index"])
				if idx >= 0 && idx < len(menu.Items) && !menu.Items[idx].Separator {
					menu.SetSelectPos(idx)
					return menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
				}
			}
		}
	}
	if c, ok := frame.(vtui.Container); ok {
		return handleSemanticChildrenAction(c.GetChildren(), target, action)
	}
	return false
}

func handleSemanticChildrenAction(children []vtui.UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if vtui.SemanticID(child) == target {
			return handleSemanticElementAction(child, action)
		}
		if c, ok := child.(vtui.Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func handleSemanticElementAction(el vtui.UIElement, action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "focus", "control.focus":
		el.SetFocus(true)
		return true
	case "activate", "control.activate":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN, InputSource: "qt_semantic"})
	case "toggle", "control.toggle":
		return el.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE, Char: ' ', InputSource: "qt_semantic"})
	case "set_text", "control.setText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.SetText(semanticString(action["text"]))
			if edit.OnTextChange != nil {
				edit.OnTextChange(edit.GetText())
			}
			return true
		}
	case "insert_text", "control.insertText":
		if edit, ok := el.(*vtui.Edit); ok {
			edit.InsertString(semanticString(action["text"]))
			return true
		}
	case "select", "control.select":
		idx := semanticInt(action["index"])
		switch w := el.(type) {
		case *vtui.RadioGroup:
			if idx >= 0 && idx < len(w.Items) {
				w.SetData(idx)
				return true
			}
		case *vtui.ListBox:
			if idx >= 0 && idx < len(w.Items) {
				w.SetSelectPos(idx)
				return true
			}
		case *vtui.ComboBox:
			if idx >= 0 && idx < len(w.Menu.Items) {
				w.Menu.SetSelectPos(idx)
				w.Edit.SetText(w.Menu.Items[idx].Text)
				return true
			}
		}
	}
	return false
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
		closeActiveAutocompleteMenus()
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

func closeActiveAutocompleteMenus() {
	if vtui.FrameManager == nil {
		return
	}
	for _, frame := range vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx) {
		if _, ok := frame.(*vtui.AutoCompleteMenu); ok {
			frame.Close()
		}
	}
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

func (fp *FileSystemPanel) semanticPanelModel(ctx *vtui.SemanticContext, side int, active bool) extui.PanelModel {
	var entries []extui.FileEntryModel
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
		entries = append(entries, extui.FileEntryModel{
			Index:          i,
			Name:           entry.Name,
			Size:           entry.Size,
			SizeText:       semanticFileSize(entry),
			IsDir:          entry.IsDir,
			IsUp:           entry.Name == "..",
			IsHidden:       entry.IsHidden,
			IsExecutable:   entry.IsExecutable,
			IsCached:       entry.IsCached,
			Selected:       entry.Selected,
			SizeCalculated: entry.SizeCalculated,
			MTime:          entry.MTime.Format("2006-01-02 15:04"),
			Mode:           entry.Mode,
		})
	}

	return extui.PanelModel{
		ID:            vtui.SemanticID(fp),
		Side:          side,
		Active:        active,
		Path:          fp.vfs.GetPath(),
		Title:         fp.frame.GetTitle(),
		ViewMode:      viewModeName(fp.effectiveViewMode()),
		SortMode:      sortModeName(fp.sortMode),
		SortReverse:   fp.sortReverse,
		Cursor:        fp.GetCursorIndex(),
		Top:           fp.table.TopPos,
		Loading:       fp.isLoading,
		FastFind:      fp.fastFindMode,
		FastFindText:  fp.fastFindStr,
		SelectedCount: selectedCount,
		SelectedSize:  selectedSize,
		TotalCount:    len(fp.entries),
		TotalSize:     totalSize,
		Entries:       entries,
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
	case ViewModeBrief:
		return "brief"
	case ViewModeDetailed:
		return "detailed"
	case ViewModeWide:
		return "wide"
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

func (cl *CommandLine) semanticModel(ctx *vtui.SemanticContext) *extui.CommandLineModel {
	return &extui.CommandLineModel{
		ID:         vtui.SemanticID(cl),
		Visible:    cl.IsVisible(),
		Focused:    cl.IsFocused(),
		Prompt:     cl.Prompt,
		PromptRuns: semanticRunsFromCells(cl.RichPrompt),
		Text:       cl.Edit.GetText(),
		Empty:      cl.IsEmpty(),
	}
}

func (tv *TerminalView) semanticModel(ctx *vtui.SemanticContext) *extui.TerminalModel {
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

	var rows []extui.TextRowModel
	for y := 0; y < tv.Height && y < len(buf); y++ {
		drawY := y + offset
		if tv.UseAltScreen {
			drawY = y
		}
		if drawY < 0 || drawY >= tv.Height {
			continue
		}
		rows = append(rows, extui.TextRowModel{
			Index: drawY,
			Runs:  semanticRunsFromCells(buf[y]),
		})
	}

	return &extui.TerminalModel{
		ID:        vtui.SemanticID(tv),
		Title:     tv.Title,
		Visible:   tv.IsVisible(),
		Focused:   tv.IsFocused(),
		AltScreen: tv.UseAltScreen,
		Busy:      tv.Muted,
		CursorX:   tv.CursorX,
		CursorY:   tv.CursorY + offset,
		Rows:      rows,
	}
}

func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	rows := vv.semanticRows()
	mode := "text"
	if vv.HexMode {
		mode = "hex"
	}

	surface := extui.SurfaceModel{
		ID:        vtui.SemanticID(vv),
		Kind:      "viewer",
		Title:     vv.GetTitle(),
		Path:      vv.path,
		BaseName:  semanticBaseName(vv.vfs, vv.path),
		Mode:      mode,
		HexMode:   vv.HexMode,
		WrapMode:  vv.WrapMode,
		Busy:      vv.Busy,
		TopOffset: vv.TopOffset,
		Size:      vv.backend.Size(),
		Rows:      rows,
	}
	return surface.ToMap()
}

func (vv *ViewerView) semanticRows() []extui.TextRowModel {
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
		return []extui.TextRowModel{{Index: 0, Text: " [ Loading... ] "}}
	}
	var rows []extui.TextRowModel
	if vv.HexMode {
		currOffset := vv.TopOffset &^ 0xF
		for y := 0; y < contentHeight && currOffset < vv.backend.Size(); y++ {
			data, err := vv.backend.ReadAt(currOffset, 16)
			if err != nil && err != piecetable.ErrLoading {
				break
			}
			rows = append(rows, extui.TextRowModel{
				Index:  y,
				Offset: currOffset,
				Text:   semanticHexLine(currOffset, data),
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
			rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: " [ Loading... ] "})
			break
		}
		if err != nil || len(data) == 0 {
			break
		}
		lineLen, textLen := semanticViewerLineLen(data, width, vv.WrapMode)
		rows = append(rows, extui.TextRowModel{Index: y, Offset: currOffset, Text: string(data[:textLen])})
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
	tabSize := 8
	if AppConfig.EditorTabSize > 0 {
		tabSize = AppConfig.EditorTabSize
	}
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
		rw := 1
		if r == '\t' {
			rw = tabSize - (visualWidth % tabSize)
		} else {
			rw = runewidth.RuneWidth(r)
			if rw <= 0 {
				rw = 1
			}
		}
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
	rows := ev.semanticRows()

	surface := extui.SurfaceModel{
		ID:           vtui.SemanticID(ev),
		Kind:         "editor",
		Title:        ev.GetTitle(),
		Path:         ev.filePath,
		BaseName:     semanticBaseName(ev.vfs, ev.filePath),
		Busy:         ev.IsBusy(),
		Dirty:        ev.modified,
		Saving:       ev.saving,
		WordWrap:     ev.WordWrap,
		Overtype:     ev.overtype,
		CursorLine:   ev.CursorLine,
		CursorPos:    ev.CursorPos,
		ScrollTop:    ev.ScrollTopRow,
		ScrollLeft:   ev.ScrollLeft,
		Selection:    ev.selActive,
		Rows:         rows,
		Autocomplete: ev.semanticAutocomplete(),
	}
	return surface.ToMap()
}

// GetText возвращает текущий текст редактора из PieceTable
func (ev *EditorView) GetText() string {
	if ev.pt == nil {
		return ""
	}
	return ev.pt.String()
}

// HandleSemanticAction обрабатывает нативные GUI-действия для EditorView
func (ev *EditorView) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if vtui.SemanticID(ev) != target {
		return false
	}

	switch semanticString(action["action"]) {
	case "editor.setText":
		text := semanticString(action["text"])
		ev.SetText(text)
		return true
	case "editor.insertText":
		text := semanticString(action["text"])
		ev.PasteText(text)
		return true
	case "editor.deleteSelection":
		ev.DeleteSelection()
		return true
	case "editor.undo":
		ev.Undo()
		return true
	case "editor.redo":
		ev.Redo()
		return true
	case "editor.save":
		ev.SaveToFile(nil)
		return true
	case "editor.search":
		pattern := semanticString(action["pattern"])
		caseSensitive := semanticBool(action["case"])
		reverse := semanticBool(action["reverse"])
		next := semanticBool(action["next"])
		ev.Search(pattern, caseSensitive, reverse, false, false, next)
		return true
	case "control.focus":
		ev.SetFocus(true)
		return true
	}
	return false
}

func (ev *EditorView) semanticRows() []extui.TextRowModel {
	if ev.pt == nil || ev.li == nil || ev.engine == nil {
		return nil
	}
	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	if height <= 0 {
		return nil
	}
	startLogLine, startFragIdx := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	var rows []extui.TextRowModel
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
			rows = append(rows, extui.TextRowModel{
				Index:       len(rows),
				VisualRow:   baseVRow + fIdx,
				LogicalLine: logIdx,
				Offset:      int64(frag.ByteOffsetStart),
				Text:        text,
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

func semanticRunsFromCells(cells []vtui.CharInfo) []extui.RunModel {
	if len(cells) == 0 {
		return nil
	}
	var runs []extui.RunModel
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, extui.RunModel{
			Text: b.String(),
			Attr: attr,
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

func semanticBool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(int); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return false
}
