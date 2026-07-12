package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"unicode"
)

type visualCell struct {
	info       vtui.CharInfo
	byteOffset int // Offset in bytes from the start of the logical line
}

type lineFragment struct {
	cells           []visualCell
	startOffset     int // Absolute offset of the fragment start
	startByteInLine int // Byte in the logical line where the fragment starts
	endByteInLine   int // Byte where the fragment ends
}

var (
	LastEditorSearch        string
	LastEditorSearchCase    bool
	LastEditorSearchReverse bool
)

// EditorView is a text editor component.
type EditorView struct {
	vtui.BaseFrame
	topBar  *TopBar
	menuBar *vtui.MenuBar
	pt      *piecetable.PieceTable
	li      *piecetable.LineIndex
	engine  *textlayout.WrapEngine

	ScrollTopRow int // Индекс первой видимой ВИЗУАЛЬНОЙ строки
	ScrollLeft   int // Горизонтальный скролл (когда WordWrap=false)

	WordWrap         bool
	overtype         bool
	modified         bool
	CursorLine       int // Текущая логическая строка (для плагинов)
	CursorPos        int // Позиция в байтах (для плагинов)
	DesiredVisualCol int // Колонка, в которую мы хотим попасть при навигации Up/Down

	ShowWhitespaces bool
	selActive       bool
	selAnchorOffset int // Абсолютное смещение начала выделения
	editSession     int // Unique ID to fence background tasks

	pasting     bool
	saving      bool
	edited      bool
	pasteBuffer []rune
	asyncBuf    *AsyncBuffer
	indexCancel context.CancelFunc
	renderBytes []byte          // Reusable buffer for text data
	renderCells []vtui.CharInfo // Reusable buffer for row rendering

	vfs         vfs.VFS
	filePath    string
	file        vfs.ReadAtCloser
	scrollBar   *vtui.ScrollBar
	highlighter vtui.Highlighter
	lineStates  []any // Cache of highlighter states per logical line

	// Undo/Redo
	undoStack  []editorState
	redoStack  []editorState
	inGroup    bool
	lastOp     undoOpType
	cleanState piecetable.TableState // State of the file on disk

	// Autocomplete state
	acEnabled    bool
	acPrefix     string
	acMatches    []string
	acCurrentIdx int

	targetLine   int
	targetPos    int
	targetTopRow int
	targetLeft   int

	TabSize             int
	ExpandTabs          int
	AutoIndent          bool
	CursorBeyondEOL     bool
	CursorVirtualSpaces int
	UseEditorConfig     bool

	// OnClose, if set, fires once after the editor has been torn down.
	// Used by callers (e.g. the user menu's Ctrl+F4 handler) that want
	// to react to the file content once the user is done editing.
	OnClose func()
}

func (ev *EditorView) ApplyEditorConfig() {
	if !ev.UseEditorConfig || ev.vfs == nil || ev.filePath == "" {
		return
	}

	dir := ev.vfs.Dir(ev.filePath)
	filename := ev.vfs.Base(ev.filePath)

	configPath := ev.vfs.Join(dir, ".editorconfig")
	f, err := ev.vfs.Open(context.Background(), configPath)
	if err != nil {
		return
	}
	defer f.Close()

	data := make([]byte, 64*1024)
	n, _ := f.Read(context.Background(), data)
	content := string(data[:n])

	lines := strings.Split(content, "\n")
	inMatchingSection := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := line[1 : len(line)-1]
			matched, _ := filepath.Match(section, filename)
			if section == "*" || matched {
				inMatchingSection = true
			} else {
				inMatchingSection = false
			}
			continue
		}

		if !inMatchingSection {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])

			switch key {
			case "indent_style":
				if val == "space" {
					ev.ExpandTabs = 1
				} else if val == "tab" {
					ev.ExpandTabs = 0
				}
			case "indent_size", "tab_width":
				if size, err := strconv.Atoi(val); err == nil && size > 0 {
					ev.TabSize = size
					ev.engine.SetTabSize(size)
				}
			}
		}
	}
}

type undoOpType int

const (
	opNone undoOpType = iota
	opTyping
	opOther
)

type editorState struct {
	table piecetable.TableState
	line  int
	pos   int
}

func (ev *EditorView) Close() {
	if GlobalFileState != nil && ev.filePath != "" {
		GlobalFileState.SaveEditorState(ev.filePath, ev.CursorLine, ev.CursorPos, ev.ScrollTopRow, ev.ScrollLeft, ev.WordWrap)
	}
	if ev.indexCancel != nil {
		ev.indexCancel()
	}
	if ev.asyncBuf != nil {
		ev.asyncBuf.Close()
	}
	if ev.file != nil {
		ev.file.Close()
	}
	ev.BaseFrame.Close()
	if ev.OnClose != nil {
		ev.OnClose()
	}
}

func NewEditorView(pt *piecetable.PieceTable, v vfs.VFS, path string) *EditorView {
	li := piecetable.NewLineIndex()
	li.Rebuild(pt)
	ev := &EditorView{
		pt:              pt,
		li:              li,
		engine:          textlayout.NewWrapEngine(pt, li),
		vfs:             v,
		filePath:        path,
		WordWrap:        false,
		ShowWhitespaces: false,
		cleanState:      pt.GetState(),
		targetLine:      -1,
		targetPos:       -1,
		targetTopRow:    -1,
		targetLeft:      -1,
		TabSize:         AppConfig.EditorTabSize,
		ExpandTabs:      AppConfig.EditorExpandTabs,
		AutoIndent:      AppConfig.EditorAutoIndent,
		CursorBeyondEOL: AppConfig.EditorCursorBeyondEOL,
		UseEditorConfig: AppConfig.EditorUseEditorConfig,
	}
	if ev.TabSize <= 0 {
		ev.TabSize = 8
	}
	ev.engine.SetTabSize(ev.TabSize)
	ev.ApplyEditorConfig()
	// Determine if AC should be enabled for this file
	ev.acEnabled = false
	if AppConfig.EditorAutoComplete && path != "" {
		masks := strings.Split(AppConfig.EditorAutoCompleteMask, ";")
		fileName := strings.ToLower(filepath.Base(path))
		for _, mask := range masks {
			mask = strings.TrimSpace(mask)
			if mask == "" {
				continue
			}
			matched, _ := filepath.Match(strings.ToLower(mask), fileName)
			if matched {
				ev.acEnabled = true
				break
			}
		}
	}
	ev.highlighter = vtui.GetHighlighter(path, "")
	ev.scrollBar = vtui.NewScrollBar(0, 0, 0)
	ev.scrollBar.SetOwner(ev)
	ev.scrollBar.OnScroll = func(v int) {
		ev.ScrollTopRow = v
		vtui.FrameManager.Redraw()
	}
	ev.menuBar = vtui.NewMenuBar(nil)
	ev.menuBar.Items = []vtui.MenuBarItem{
		{Label: "&File", SubItems: []vtui.MenuItem{{Text: "&Save", Command: vtui.CmDefault}, {Text: "E&xit", Command: vtui.CmClose}}},
		{Label: "&Edit", SubItems: []vtui.MenuItem{{Text: "&Copy", Command: CmCopy}, {Text: "&Paste"}}},
		{Label: "&Search", SubItems: []vtui.MenuItem{{Text: "&Find", Command: CmSearch}}},
		{Label: "&Options", SubItems: []vtui.MenuItem{{Text: "&WordWrap"}}},
	}

	ev.topBar = NewTopBar(func() string {
		base := ""
		if ev.vfs != nil {
			base = ev.vfs.Base(ev.filePath)
		} else {
			base = filepath.Base(ev.filePath)
		}
		return fmt.Sprintf(" %s │ %d,%d ", base, ev.CursorLine+1, ev.CursorPos)
	})
	ev.topBar.SetVisible(true)
	ev.SetCanFocus(true)
	ev.SetFocus(true)
	return ev
}

// GetTopBar возвращает верхнюю панель для тестов
func (ev *EditorView) GetTopBar() *TopBar {
	return ev.topBar
}

// SetText replaces the entire content of the editor.
func (ev *EditorView) SetText(text string) {
	if ev.indexCancel != nil {
		ev.indexCancel()
		ev.indexCancel = nil
	}
	ev.edited = true
	ev.editSession++

	ev.pt = piecetable.New([]byte(text))
	ev.li.Rebuild(ev.pt)
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.engine.SetPointers(ev.pt, ev.li)
	ev.modified = true
}

func (ev *EditorView) clearCaches() {
	ev.engine.InvalidateCache()
}
func (ev *EditorView) saveUndo(op undoOpType) {
	if ev.inGroup {
		return
	}

	ev.redoStack = nil // Redo stack MUST be cleared on any new modification

	// If we are about to modify a selection, the "home" position for Undo
	// is the start of that selection.
	line, pos := ev.CursorLine, ev.CursorPos
	if ev.selActive {
		minOff, _ := ev.getSelectionRange()
		line = ev.li.GetLineAtOffset(minOff)
		pos = minOff - ev.li.GetLineOffset(line)
	}

	state := editorState{
		table: ev.pt.GetState(),
		line:  line,
		pos:   pos,
	}

	// Simple grouping for typing: don't push new state if we are just typing characters consecutively
	if op == opTyping && ev.lastOp == opTyping && len(ev.undoStack) > 0 {
		return
	}

	ev.undoStack = append(ev.undoStack, state)
	if len(ev.undoStack) > 1000 {
		ev.undoStack = ev.undoStack[1:]
	}
	ev.lastOp = op
	ev.modified = true // Mark as dirty, will be re-evaluated on Undo/Redo
}

func (ev *EditorView) Undo() {
	if len(ev.undoStack) == 0 {
		vtui.DebugLog("EDITOR: Undo called but stack is empty")
		return
	}

	if !ev.edited {
		ev.edited = true
		if ev.indexCancel != nil {
			ev.indexCancel()
		}
	}
	ev.editSession++

	// Save current state to redo stack
	ev.redoStack = append(ev.redoStack, editorState{
		table: ev.pt.GetState(),
		line:  ev.CursorLine,
		pos:   ev.CursorPos,
	})

	// Restore last state
	last := len(ev.undoStack) - 1
	state := ev.undoStack[last]
	ev.undoStack = ev.undoStack[:last]

	ev.pt.LoadState(state.table)
	ev.li.Rebuild(ev.pt)
	ev.CursorLine = state.line
	ev.CursorPos = state.pos

	ev.clearCaches()
	// Intelligent modified flag: if structure matches clean state, it's not modified
	ev.modified = !ev.pt.GetState().Equals(ev.cleanState)
	ev.lastOp = opNone
	ev.ensureCursorVisible()
	vtui.DebugLog("EDITOR: Executed Undo, remaining: %d, modified: %v", len(ev.undoStack), ev.modified)
}

func (ev *EditorView) Redo() {
	if len(ev.redoStack) == 0 {
		vtui.DebugLog("EDITOR: Redo called but stack is empty")
		return
	}

	if !ev.edited {
		ev.edited = true
		if ev.indexCancel != nil {
			ev.indexCancel()
		}
	}
	ev.editSession++

	// Save current state to undo stack
	ev.undoStack = append(ev.undoStack, editorState{
		table: ev.pt.GetState(),
		line:  ev.CursorLine,
		pos:   ev.CursorPos,
	})

	last := len(ev.redoStack) - 1
	state := ev.redoStack[last]
	ev.redoStack = ev.redoStack[:last]

	ev.pt.LoadState(state.table)
	ev.li.Rebuild(ev.pt)
	ev.CursorLine = state.line
	ev.CursorPos = state.pos

	ev.clearCaches()
	// Intelligent modified flag
	ev.modified = !ev.pt.GetState().Equals(ev.cleanState)
	ev.lastOp = opNone
	ev.ensureCursorVisible()
	vtui.DebugLog("EDITOR: Executed Redo, remaining: %d, modified: %v", len(ev.redoStack), ev.modified)
}
func (ev *EditorView) invalidateStates(fromLine int) {
	if fromLine < len(ev.lineStates) {
		ev.lineStates = ev.lineStates[:fromLine]
	}
}
func (ev *EditorView) ensureEngineWidth() {
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}
	if width < 1 {
		width = 1
	}
	ev.engine.SetWidth(width)
	ev.engine.ToggleWrap(ev.WordWrap)
}

func (ev *EditorView) updateDesiredVisualCol() {
	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	_, vCol := ev.engine.LogicalToVisual(curOffset)
	ev.DesiredVisualCol = vCol + ev.CursorVirtualSpaces
}

func (ev *EditorView) Show(scr *vtui.ScreenBuf) {
	ev.ScreenObject.Show(scr)
	if ev.topBar != nil {
		ev.topBar.Show(scr)
	}
	if ev.IsFocused() {
		if vtui.ManageCursorStyle {
			if ev.overtype {
				os.Stdout.WriteString("\x1b[1 q") // Blinking Block
			} else {
				os.Stdout.WriteString("\x1b[3 q") // Blinking Underline
			}
		}
	}
	ev.DisplayObject(scr)
}

func (ev *EditorView) DisplayObject(scr *vtui.ScreenBuf) {
	if !ev.IsVisible() || ev.pasting {
		return
	}

	ev.ensureEngineWidth()
	height := ev.Y2 - ev.Y1
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}

	bgAttr := vtui.Palette[ColEditorText]
	selAttr := vtui.Palette[vtui.ColDialogEditSelected]

	if ev.saving {
		scr.FillRect(ev.X1, ev.Y1+1, ev.X2, ev.Y2, ' ', bgAttr)
		scr.Write(ev.X1, ev.Y1+1, vtui.StringToCharInfo(" [ Saving... ] ", bgAttr))
		return
	}

	// Calculate crosshair parameters before usage
	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	curVRow, curVCol := ev.engine.LogicalToVisual(curOffset)

	crossVRow, crossVCol := -1, -1
	var crossAttr uint64
	if AppConfig.EditorCrosshair && ev.IsFocused() {
		crossVRow = curVRow
		crossVCol = curVCol + ev.CursorVirtualSpaces
		crossAttr = vtui.Palette[ColEditorCrosshair]
	}

	// Clear the entire editor text area
	scr.FillRect(ev.X1, ev.Y1+1, ev.X2, ev.Y2, ' ', bgAttr)

	if crossVRow != -1 {
		// Horizontal line
		cy := ev.Y1 + 1 + crossVRow - ev.ScrollTopRow
		if cy >= ev.Y1+1 && cy <= ev.Y2 {
			scr.FillRect(ev.X1, cy, ev.X1+width-1, cy, ' ', crossAttr)
		}
		// Vertical line
		cx := ev.X1 + crossVCol - ev.ScrollLeft
		if cx >= ev.X1 && cx < ev.X1+width {
			scr.FillRect(cx, ev.Y1+1, cx, ev.Y2, ' ', crossAttr)
		}
	}

	scr.PushClipRect(ev.X1, ev.Y1+1, ev.X1+width-1, ev.Y2)

	// 2. Отрисовка
	startLogLine, startFragIdx := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	rowsRendered := 0

	for logIdx := startLogLine; logIdx < ev.li.LineCount(); logIdx++ {
		lineStart := ev.li.GetLineOffset(logIdx)
		lineLen := 0
		if logIdx+1 < ev.li.LineCount() {
			lineLen = ev.li.GetLineOffset(logIdx+1) - lineStart
		} else {
			lineLen = ev.pt.Size() - lineStart
		}

		// Stateful Highlighting
		var lineSyntax []uint64
		if ev.highlighter != nil {
			// Ensure we have computed states up to this line
			for len(ev.lineStates) <= logIdx {
				currIdx := len(ev.lineStates)
				lStart := ev.li.GetLineOffset(currIdx)
				lEnd := ev.pt.Size()
				if currIdx+1 < ev.li.LineCount() {
					lEnd = ev.li.GetLineOffset(currIdx + 1)
				}
				// Prevent highlighter from crashing on huge binary lines
				if lEnd-lStart > 64*1024 {
					lEnd = lStart + 64*1024
				}

				var prevState any
				if currIdx > 0 {
					prevState = ev.lineStates[currIdx-1]
				}

				lineData, err := ev.pt.GetRange(lStart, lEnd-lStart)
				if err == piecetable.ErrLoading {
					break // Wait for data
				}

				attrs, nextState := ev.highlighter.Highlight(string(lineData), prevState, bgAttr)
				ev.lineStates = append(ev.lineStates, nextState)
				if currIdx == logIdx {
					lineSyntax = attrs
				}
			}
			if logIdx < len(ev.lineStates) && lineSyntax == nil {
				// State was already cached, but we need the actual attributes for the current visible line
				lStart := ev.li.GetLineOffset(logIdx)
				// Re-apply highlighter OOM protection for the rendering path
				highlightLen := lineLen
				if highlightLen > 64*1024 {
					highlightLen = 64 * 1024
				}
				lineData, _ := ev.pt.GetRange(lStart, highlightLen)
				var prevState any
				if logIdx > 0 {
					prevState = ev.lineStates[logIdx-1]
				}
				lineSyntax, _ = ev.highlighter.Highlight(string(lineData), prevState, bgAttr)
			}
		}

		frags := ev.engine.GetFragments(logIdx)
		baseVRow := ev.engine.GetRowOffset(logIdx)
		// vtui.DebugLog("EDITOR_RENDER: Line %d, Frags: %d, BaseVRow: %d", logIdx, len(frags), baseVRow)
		runesProcessedInLine := 0

		for fIdx, frag := range frags {
			if logIdx == startLogLine && fIdx < startFragIdx {
				// Пропускаем подсветку для фрагментов выше области видимости
				fragData, _ := ev.pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
				runesProcessedInLine += len([]rune(string(fragData)))
				continue
			}

			absVRow := baseVRow + fIdx
			currY := ev.Y1 + 1 + rowsRendered

			ev.renderBytes = ev.renderBytes[:0]
			var err error
			ev.renderBytes, err = ev.pt.AppendRange(ev.renderBytes, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)

			fragRuneCount := len([]rune(string(ev.renderBytes)))

			if err == piecetable.ErrLoading {
				scr.Write(ev.X1-ev.ScrollLeft, currY, vtui.StringToCharInfo(" [ Loading... ] ", bgAttr))
				runesProcessedInLine += fragRuneCount
				rowsRendered++
				if rowsRendered >= height {
					goto DoneRendering
				}
				continue
			}

			selMin, selMax := ev.getSelectionRange()

			// Вырезаем кусок атрибутов именно для этого фрагмента
			var fragSyntax []uint64
			if runesProcessedInLine < len(lineSyntax) {
				end := runesProcessedInLine + fragRuneCount
				if end > len(lineSyntax) {
					end = len(lineSyntax)
				}
				fragSyntax = lineSyntax[runesProcessedInLine:end]
			}
			runesProcessedInLine += fragRuneCount

			_, startVCol := ev.engine.LogicalToVisual(frag.ByteOffsetStart)
			isCrossRow := (absVRow == crossVRow)
			ev.renderCells = ev.fillCells(ev.renderCells, ev.renderBytes, bgAttr, selAttr, frag.ByteOffsetStart, ev.selActive, selMin, selMax, fragSyntax, startVCol, isCrossRow, crossVCol, crossAttr)

			scr.Write(ev.X1-ev.ScrollLeft, currY, ev.renderCells)

			if absVRow == curVRow {
				scr.SetCursorPos(ev.X1+curVCol+ev.CursorVirtualSpaces-ev.ScrollLeft, currY)
				scr.SetCursorVisible(true)
			}

			rowsRendered++
			if rowsRendered >= height {
				goto DoneRendering
			}
		}
	}

DoneRendering:
	// 3. Draw Autocomplete Ghost Text
	if ev.acEnabled && len(ev.acMatches) > 0 && ev.IsFocused() && !ev.pasting {
		match := ev.acMatches[ev.acCurrentIdx]
		if len(match) > len(ev.acPrefix) {
			tail := match[len(ev.acPrefix):]
			// Calculate exact visual position of cursor
			curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			vRow, vCol := ev.engine.LogicalToVisual(curOffset)

			drawY := ev.Y1 + 1 + vRow - ev.ScrollTopRow
			drawX := ev.X1 + vCol - ev.ScrollLeft

			// Draw if visible
			if drawY >= ev.Y1+1 && drawY <= ev.Y2 {
				// We use DimColor of the standard text to make it look like a ghost suggestion
				ghostAttr := vtui.DimColor(vtui.Palette[ColCommandLineUserScreen])
				// Ensure it doesn't leak out of the editor frame
				maxLen := ev.X2 - drawX
				if ev.scrollBar != nil {
					maxLen--
				}

				if maxLen > 0 {
					displayTail := tail
					if len([]rune(displayTail)) > maxLen {
						displayTail = string([]rune(displayTail)[:maxLen])
					}
					scr.Write(drawX, drawY, vtui.StringToCharInfo(displayTail, ghostAttr))
				}
			}
		}
	}

	scr.PopClipRect()

	if ev.scrollBar != nil {
		totalRows := ev.engine.GetTotalVisualRows()
		if totalRows > height {
			ev.scrollBar.SetParams(ev.ScrollTopRow, 0, totalRows-height)
			ev.scrollBar.Show(scr)
		}
	}
}

func (ev *EditorView) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.KeyEventType && e.KeyDown {
		if ev.targetLine != -1 {
			ev.targetLine = -1 // User took control, abort target jump
			ev.ensureCursorVisible()
		}
	}

	ev.ensureEngineWidth()
	if ev.saving {
		return true
	}
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	// 1. Processing Bracketed Paste (events arrive outside KeyDown)
	if e.Type == vtinput.PasteEventType {
		if e.PasteStart {
			ev.pasting = true
			ev.pasteBuffer = nil
		} else {
			ev.pasting = false
			if len(ev.pasteBuffer) > 0 {
				if ev.indexCancel != nil {
					ev.indexCancel()
					ev.indexCancel = nil
				}
				ev.edited = true

				ev.saveUndo(opOther)
				if ev.selActive {
					ev.inGroup = true
					ev.DeleteSelection()
					ev.inGroup = false
				}

				offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
				data := []byte(string(ev.pasteBuffer))
				ev.pt.Insert(offset, data)
				ev.li.UpdateAfterInsert(offset, data)
				ev.invalidateStates(ev.CursorLine)
				ev.engine.InvalidateFrom(ev.CursorLine)

				newOffset := offset + len(data)
				ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
				ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)
				ev.modified = true
				ev.updateDesiredVisualCol()
				ev.ensureCursorVisible()
			}
		}
		return true
	}

	// 2. Accumulating characters in paste mode
	if ev.pasting {
		if e.Type == vtinput.KeyEventType && e.KeyDown {
			if e.Char != 0 {
				// Handle system line breaks inside the paste
				if e.Char == '\r' {
					// Ignore \r to prevent double line breaks
				} else if e.Char == '\n' {
					ev.pasteBuffer = append(ev.pasteBuffer, '\n')
				} else {
					ev.pasteBuffer = append(ev.pasteBuffer, e.Char)
				}
			} else if e.VirtualKeyCode == vtinput.VK_RETURN {
				ev.pasteBuffer = append(ev.pasteBuffer, '\n')
			}
		}
		return true
	}

	// 3. Regular key processing
	if !e.KeyDown {
		return false
	}

	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	//alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	// --- Autocomplete Interception ---
	if ev.acEnabled && len(ev.acMatches) > 0 {
		if e.VirtualKeyCode == vtinput.VK_TAB {
			if (e.ControlKeyState & vtinput.ShiftPressed) != 0 {
				// Shift+Tab: cycle matches
				ev.acCurrentIdx = (ev.acCurrentIdx + 1) % len(ev.acMatches)
				vtui.FrameManager.Redraw()
				return true
			} else if (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed)) == 0 {
				// Tab: Apply completion
				match := ev.acMatches[ev.acCurrentIdx]
				tail := match[len(ev.acPrefix):]
				ev.acMatches = nil // Clear state

				ev.saveUndo(opTyping)
				ev.modified = true
				offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
				data := []byte(tail)
				ev.pt.Insert(offset, data)
				ev.li.UpdateAfterInsert(offset, data)
				ev.invalidateStates(ev.CursorLine)
				ev.engine.InvalidateFrom(ev.CursorLine)
				ev.CursorPos += len(data)
				ev.updateDesiredVisualCol()
				ev.ensureCursorVisible()
				return true
			}
		} else if e.VirtualKeyCode == vtinput.VK_ESCAPE {
			// Esc: Dismiss autocomplete
			ev.acMatches = nil
			vtui.FrameManager.Redraw()
			return true
		}

		// Any movement or non-character key clears the AC state
		if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN ||
			e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_RIGHT ||
			e.VirtualKeyCode == vtinput.VK_HOME || e.VirtualKeyCode == vtinput.VK_END ||
			e.VirtualKeyCode == vtinput.VK_PRIOR || e.VirtualKeyCode == vtinput.VK_NEXT ||
			e.VirtualKeyCode == vtinput.VK_RETURN {
			ev.acMatches = nil
		}
	}

	// Allow FrameManager to handle Ctrl+Tab for workspace switching
	if e.VirtualKeyCode == vtinput.VK_TAB && ctrl {
		return false
	}

	handleNav := func() {
		if shift {
			if !ev.selActive {
				ev.selActive = true
				ev.selAnchorOffset = ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			}
		} else {
			ev.selActive = false
		}
	}

	// Any key that can reach this point and is not a pure navigation key
	// should stop the background indexer to prevent index corruption.
	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_HOME, vtinput.VK_END,
		vtinput.VK_SHIFT, vtinput.VK_CONTROL, vtinput.VK_MENU:
		// ignore navigation and modifiers
	default:
		if !ev.edited {
			ev.edited = true
			ev.editSession++
			if ev.indexCancel != nil {
				ev.indexCancel()
			}
		}
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_Z:
		if ctrl {
			if shift {
				ev.Redo()
			} else {
				ev.Undo()
			}
			return true
		}

	case vtinput.VK_A:
		if ctrl {
			ev.selActive = true
			ev.selAnchorOffset = 0
			lastLine := ev.li.LineCount() - 1
			ev.CursorLine = lastLine
			ev.CursorPos = ev.getLineLength(lastLine)
			ev.ensureCursorVisible()
			return true
		}

	case vtinput.VK_F2:
		ev.SaveToFile(nil)
		return true

	case vtinput.VK_F3:
		ev.WordWrap = !ev.WordWrap
		ev.ScrollLeft = 0
		ev.clearCaches()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_F5:
		ev.ShowWhitespaces = !ev.ShowWhitespaces
		return true

	case vtinput.VK_F7:
		if shift && LastEditorSearch != "" {
			ev.Search(LastEditorSearch, LastEditorSearchCase, LastEditorSearchReverse, true)
		} else {
			vtui.FrameManager.EmitCommand(CmSearch, nil)
		}
		return true

	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		ev.tryClose()
		return true

	case vtinput.VK_C, vtinput.VK_INSERT:
		if ctrl && ev.selActive {
			ev.CopySelection()
			return true
		}
		if shift && !ctrl && e.VirtualKeyCode == vtinput.VK_INSERT {
			if text := vtui.GetClipboard(); text != "" {
				ev.PasteText(text)
			}
			return true
		}
		if !shift && !ctrl && !alt && e.VirtualKeyCode == vtinput.VK_INSERT {
			ev.overtype = !ev.overtype
			ev.ensureCursorVisible()
			return true
		}

	case vtinput.VK_V:
		if ctrl && !shift {
			if text := vtui.GetClipboard(); text != "" {
				ev.PasteText(text)
			}
			return true
		}

	case vtinput.VK_UP, vtinput.VK_E:
		if e.VirtualKeyCode == vtinput.VK_E && !ctrl {
			break
		}
		handleNav()
		curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		vRow, _ := ev.engine.LogicalToVisual(curOffset)
		if vRow > 0 {
			newOffset := ev.engine.VisualToLogical(vRow-1, ev.DesiredVisualCol)
			ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
			ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)

			lineLen := ev.getLineLength(ev.CursorLine)
			vtui.DebugLog("DEBUG_UP: TargetRow:%d DesiredCol:%d ResultPos:%d LineLen:%d", vRow-1, ev.DesiredVisualCol, ev.CursorPos, lineLen)
			if ev.CursorPos == lineLen && ev.CursorBeyondEOL {
				_, endVCol := ev.engine.LogicalToVisual(ev.li.GetLineOffset(ev.CursorLine) + lineLen)
				if ev.DesiredVisualCol > endVCol {
					ev.CursorVirtualSpaces = ev.DesiredVisualCol - endVCol
				} else {
					ev.CursorVirtualSpaces = 0
				}
				vtui.DebugLog("DEBUG_UP_VIRT: EndVCol:%d ResultVirt:%d", endVCol, ev.CursorVirtualSpaces)
			} else {
				ev.CursorVirtualSpaces = 0
			}
		}
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_DOWN, vtinput.VK_X:
		if e.VirtualKeyCode == vtinput.VK_X {
			if !ctrl {
				break
			}
			if ev.selActive {
				ev.CopySelection()
				ev.DeleteSelection()
				return true
			}
		}
		handleNav()
		curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		vRow, _ := ev.engine.LogicalToVisual(curOffset)
		newOffset := ev.engine.VisualToLogical(vRow+1, ev.DesiredVisualCol)
		ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
		ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)

		lineLen := ev.getLineLength(ev.CursorLine)
		if ev.CursorPos == lineLen && ev.CursorBeyondEOL {
			_, endVCol := ev.engine.LogicalToVisual(ev.li.GetLineOffset(ev.CursorLine) + lineLen)
			vtui.DebugLog("DEBUG_DOWN: TargetRow:%d DesiredCol:%d ResultPos:%d LineLen:%d EndVCol:%d", vRow+1, ev.DesiredVisualCol, ev.CursorPos, lineLen, endVCol)
			if ev.DesiredVisualCol > endVCol {
				ev.CursorVirtualSpaces = ev.DesiredVisualCol - endVCol
			} else {
				ev.CursorVirtualSpaces = 0
			}
			vtui.DebugLog("DEBUG_DOWN_VIRT: ResultVirt:%d", ev.CursorVirtualSpaces)
		} else {
			vtui.DebugLog("DEBUG_DOWN_NO_VIRT: TargetRow:%d ResultPos:%d LineLen:%d", vRow+1, ev.CursorPos, lineLen)
			ev.CursorVirtualSpaces = 0
		}
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_PRIOR: // PgUp
		handleNav()
		height := ev.Y2 - ev.Y1
		curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		vRow, _ := ev.engine.LogicalToVisual(curOffset)
		newVRow := vRow - height
		if newVRow < 0 {
			newVRow = 0
		}
		newOffset := ev.engine.VisualToLogical(newVRow, ev.DesiredVisualCol)
		ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
		ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)

		lineLen := ev.getLineLength(ev.CursorLine)
		if ev.CursorPos == lineLen && ev.CursorBeyondEOL {
			_, endVCol := ev.engine.LogicalToVisual(ev.li.GetLineOffset(ev.CursorLine) + lineLen)
			if ev.DesiredVisualCol > endVCol {
				ev.CursorVirtualSpaces = ev.DesiredVisualCol - endVCol
			} else {
				ev.CursorVirtualSpaces = 0
			}
		} else {
			ev.CursorVirtualSpaces = 0
		}
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_NEXT: // PgDn
		handleNav()
		height := ev.Y2 - ev.Y1
		curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		vRow, _ := ev.engine.LogicalToVisual(curOffset)
		newVRow := vRow + height
		totalVRows := ev.engine.GetTotalVisualRows()
		if newVRow >= totalVRows {
			newVRow = totalVRows - 1
		}
		newOffset := ev.engine.VisualToLogical(newVRow, ev.DesiredVisualCol)
		ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
		ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)

		lineLen := ev.getLineLength(ev.CursorLine)
		if ev.CursorPos == lineLen && ev.CursorBeyondEOL {
			_, endVCol := ev.engine.LogicalToVisual(ev.li.GetLineOffset(ev.CursorLine) + lineLen)
			if ev.DesiredVisualCol > endVCol {
				ev.CursorVirtualSpaces = ev.DesiredVisualCol - endVCol
			} else {
				ev.CursorVirtualSpaces = 0
			}
		} else {
			ev.CursorVirtualSpaces = 0
		}
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_LEFT, vtinput.VK_S:
		isAlias := e.VirtualKeyCode == vtinput.VK_S
		if isAlias && !ctrl {
			break
		}
		handleNav()
		if ev.CursorVirtualSpaces > 0 {
			ev.CursorVirtualSpaces--
			ev.updateDesiredVisualCol()
			ev.ensureCursorVisible()
			return true
		}

		// Jump by word only if it's the real Left arrow + Ctrl
		if ctrl && !isAlias {
			if ev.CursorPos > 0 {
				runes := ev.getLogicalLineRunes(ev.CursorLine)
				currRuneIdx := 0
				byteAcc := 0
				for i, r := range runes {
					if byteAcc >= ev.CursorPos {
						currRuneIdx = i
						break
					}
					byteAcc += utf8.RuneLen(r)
					if i == len(runes)-1 {
						currRuneIdx = len(runes)
					}
				}

				if currRuneIdx > 0 {
					lineStart := ev.li.GetLineOffset(ev.CursorLine)
					startVRow, _ := ev.engine.LogicalToVisual(lineStart + ev.CursorPos)

					currRuneIdx--
					ev.CursorPos = 0
					for i := 0; i < currRuneIdx; i++ {
						ev.CursorPos += utf8.RuneLen(runes[i])
					}
					if shift {
						handleNav()
					}

					vRow, _ := ev.engine.LogicalToVisual(lineStart + ev.CursorPos)
					if vRow == startVRow {
						for currRuneIdx > 0 {
							prev, curr := runes[currRuneIdx-1], runes[currRuneIdx]
							pCat, cCat := getCharCategory(prev), getCharCategory(curr)
							if (shift && pCat != catSpace && cCat == catSpace) ||
								(pCat == catSpace && cCat == catWord) ||
								(pCat == catSpace && cCat == catDivider) ||
								(pCat == catDivider && cCat == catWord) {
								break
							}
							currRuneIdx--
							ev.CursorPos = 0
							for i := 0; i < currRuneIdx; i++ {
								ev.CursorPos += utf8.RuneLen(runes[i])
							}
							if shift {
								handleNav()
							}
							vRow, _ = ev.engine.LogicalToVisual(lineStart + ev.CursorPos)
							if vRow != startVRow {
								break
							}
						}
					}
				}
			} else if ev.CursorLine > 0 {
				ev.CursorLine--
				ev.CursorPos = ev.getLineLength(ev.CursorLine)
			}
		} else {
			if ev.CursorPos > 0 {
				lineStart := ev.li.GetLineOffset(ev.CursorLine)
				data, _ := ev.pt.GetRange(lineStart, ev.CursorPos)
				if data != nil && len(data) > 0 {
					_, size := utf8.DecodeLastRune(data)
					ev.CursorPos -= size
				} else {
					ev.CursorPos--
				}
			} else if ev.CursorLine > 0 {
				ev.CursorLine--
				ev.CursorPos = ev.getLineLength(ev.CursorLine)
			}
		}
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_RIGHT, vtinput.VK_D:
		isAlias := e.VirtualKeyCode == vtinput.VK_D
		if isAlias && !ctrl {
			break
		}
		handleNav()
		lineLen := ev.getLineLength(ev.CursorLine)
		// Jump by word only if it's the real Right arrow + Ctrl
		if ctrl && !isAlias {
			ev.CursorVirtualSpaces = 0
			if ev.CursorPos < lineLen {
				runes := ev.getLogicalLineRunes(ev.CursorLine)
				currRuneIdx := len(runes)
				byteAcc := 0
				for i, r := range runes {
					if byteAcc >= ev.CursorPos {
						currRuneIdx = i
						break
					}
					byteAcc += utf8.RuneLen(r)
				}

				if currRuneIdx < len(runes) {
					lineStart := ev.li.GetLineOffset(ev.CursorLine)
					startVRow, _ := ev.engine.LogicalToVisual(lineStart + ev.CursorPos)

					currRuneIdx++
					ev.CursorPos = 0
					for i := 0; i < currRuneIdx; i++ {
						ev.CursorPos += utf8.RuneLen(runes[i])
					}
					if shift {
						handleNav()
					}

					vRow, _ := ev.engine.LogicalToVisual(lineStart + ev.CursorPos)
					if vRow == startVRow {
						for currRuneIdx < len(runes) {
							prev, curr := runes[currRuneIdx-1], runes[currRuneIdx]
							pCat, cCat := getCharCategory(prev), getCharCategory(curr)
							stop := false
							if shift && pCat != catSpace && cCat == catSpace {
								stop = true
							}
							if pCat == catWord && cCat == catDivider {
								stop = true
							}
							if pCat == catSpace && cCat == catWord {
								stop = true
							}
							if pCat == catSpace && cCat == catDivider {
								stop = true
							}
							if pCat == catDivider && cCat == catWord {
								stop = true
							}
							if pCat == catDivider && cCat == catDivider && prev != curr {
								stop = true
							}

							if stop {
								break
							}

							currRuneIdx++
							ev.CursorPos = 0
							for i := 0; i < currRuneIdx; i++ {
								ev.CursorPos += utf8.RuneLen(runes[i])
							}
							if shift {
								handleNav()
							}

							vRow, _ = ev.engine.LogicalToVisual(lineStart + ev.CursorPos)
							if vRow != startVRow {
								// Revert to the end of the previous visual line
								currRuneIdx--
								ev.CursorPos = 0
								for i := 0; i < currRuneIdx; i++ {
									ev.CursorPos += utf8.RuneLen(runes[i])
								}
								if shift {
									handleNav()
								}
								break
							}
						}
					}
				}
			} else if ev.CursorLine < ev.li.LineCount()-1 {
				ev.CursorLine++
				ev.CursorPos = 0
			}
		} else {
			if ev.CursorPos < lineLen {
				lineStart := ev.li.GetLineOffset(ev.CursorLine)
				peekLen := 4
				if lineLen-ev.CursorPos < 4 {
					peekLen = lineLen - ev.CursorPos
				}
				data, _ := ev.pt.GetRange(lineStart+ev.CursorPos, peekLen)
				if data != nil && len(data) > 0 {
					_, size := utf8.DecodeRune(data)
					ev.CursorPos += size
				} else {
					ev.CursorPos++
				}
			} else if ev.CursorLine < ev.li.LineCount()-1 {
				if ev.CursorBeyondEOL {
					ev.CursorVirtualSpaces++
				} else {
					ev.CursorLine++
					ev.CursorPos = 0
					ev.CursorVirtualSpaces = 0
				}
			} else if ev.CursorBeyondEOL {
				ev.CursorVirtualSpaces++
			}
		}
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_HOME:
		handleNav()
		if ctrl {
			ev.CursorLine = 0
		}
		ev.CursorPos = 0
		ev.CursorVirtualSpaces = 0
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_END:
		handleNav()
		if ctrl {
			ev.CursorLine = ev.li.LineCount() - 1
		}
		ev.CursorPos = ev.getLineLength(ev.CursorLine)
		ev.CursorVirtualSpaces = 0
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_BACK:
		if ev.selActive {
			ev.DeleteSelection()
		} else {
			ev.saveUndo(opOther)
			ev.modified = true
			offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			if offset > 0 {
				if ev.CursorPos == 0 {
					// Merge with the previous line (remove line break)
					prevLen := ev.getLineLength(ev.CursorLine - 1)
					delLen := 1
					// Check for CRLF (\r\n)
					if offset >= 2 {
						prefix, _ := ev.pt.GetRange(offset-2, 2)
						if len(prefix) == 2 && prefix[0] == '\r' && prefix[1] == '\n' {
							delLen = 2
						}
					}

					ev.pt.Delete(offset-delLen, delLen)
					ev.li.UpdateAfterDelete(offset-delLen, delLen)
					ev.invalidateStates(ev.CursorLine - 1)
					ev.engine.InvalidateFrom(ev.CursorLine - 1)
					ev.CursorLine--
					ev.CursorPos = prevLen
				} else {
					// Remove the UTF-8 character before the cursor
					lineStart := ev.li.GetLineOffset(ev.CursorLine)
					lineData, _ := ev.pt.GetRange(lineStart, ev.CursorPos)
					size := 1
					if lineData != nil {
						r, rsize := utf8.DecodeLastRune(lineData)
						if r != utf8.RuneError {
							size = rsize
						}
					}

					ev.pt.Delete(offset-size, size)
					ev.li.UpdateAfterDelete(offset-size, size)
					ev.invalidateStates(ev.CursorLine)
					ev.engine.InvalidateFrom(ev.CursorLine)
					ev.CursorPos -= size
				}
			}
		}
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_DELETE:
		if ev.selActive {
			if shift {
				ev.CopySelection()
			}
			ev.DeleteSelection()
		} else {
			ev.saveUndo(opOther)
			ev.modified = true
			offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			if offset < ev.pt.Size() {
				// Remove the UTF-8 character under the cursor
				peekLen := 4
				if ev.pt.Size()-offset < 4 {
					peekLen = ev.pt.Size() - offset
				}
				data, _ := ev.pt.GetRange(offset, peekLen)
				size := 1
				if data != nil {
					r, rsize := utf8.DecodeRune(data)
					if r != utf8.RuneError {
						size = rsize
					}
				}

				ev.pt.Delete(offset, size)
				ev.li.UpdateAfterDelete(offset, size)
				ev.invalidateStates(ev.CursorLine)
				ev.engine.InvalidateFrom(ev.CursorLine)
			}
		}
		ev.ensureCursorVisible()
		return true

	case vtinput.VK_RETURN:
		ev.saveUndo(opOther)
		if ev.selActive {
			ev.inGroup = true
			ev.DeleteSelection()
			ev.inGroup = false
		}
		ev.modified = true
		offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		var indent []byte
		if ev.AutoIndent {
			lineRunes := ev.getLogicalLineRunes(ev.CursorLine)
			for _, r := range lineRunes {
				if r == ' ' || r == '\t' {
					indent = append(indent, []byte(string(r))...)
				} else {
					break
				}
			}
		}

		if ev.CursorVirtualSpaces > 0 {
			spaces := []byte(strings.Repeat(" ", ev.CursorVirtualSpaces))
			ev.pt.Insert(offset, spaces)
			ev.li.UpdateAfterInsert(offset, spaces)
			offset += ev.CursorVirtualSpaces
			ev.CursorVirtualSpaces = 0
		}

		ev.pt.Insert(offset, []byte("\n"))
		ev.li.UpdateAfterInsert(offset, []byte("\n"))
		ev.engine.InvalidateFrom(ev.CursorLine)
		ev.CursorLine++
		ev.CursorPos = 0
		ev.DesiredVisualCol = 0

		if len(indent) > 0 {
			offset = ev.li.GetLineOffset(ev.CursorLine)
			ev.pt.Insert(offset, indent)
			ev.li.UpdateAfterInsert(offset, indent)
			ev.CursorPos += len(indent)
			ev.updateDesiredVisualCol()
		}

		ev.ensureCursorVisible()
		return true

	case vtinput.VK_TAB:
		if !shift && !ctrl && !alt {
			ev.saveUndo(opTyping)
			if ev.selActive {
				ev.inGroup = true
				ev.DeleteSelection()
				ev.inGroup = false
			}
			ev.modified = true
			offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos

			var data []byte
			if ev.ExpandTabs > 0 {
				_, vCol := ev.engine.LogicalToVisual(offset)
				vCol += ev.CursorVirtualSpaces
				spaces := ev.TabSize - (vCol % ev.TabSize)
				vtui.DebugLog("DEBUG_TAB_EXPAND: Offset:%d BaseVCol:%d Virt:%d ResultSpaces:%d (TabSize:%d)", offset, vCol-ev.CursorVirtualSpaces, ev.CursorVirtualSpaces, spaces, ev.TabSize)
				data = []byte(strings.Repeat(" ", spaces))
			} else {
				data = []byte("\t")
			}

			if ev.CursorVirtualSpaces > 0 {
				virtSpaces := []byte(strings.Repeat(" ", ev.CursorVirtualSpaces))
				ev.pt.Insert(offset, virtSpaces)
				ev.li.UpdateAfterInsert(offset, virtSpaces)
				offset += ev.CursorVirtualSpaces
				ev.CursorPos += ev.CursorVirtualSpaces
				ev.CursorVirtualSpaces = 0
			}

			ev.pt.Insert(offset, data)
			ev.li.UpdateAfterInsert(offset, data)
			ev.invalidateStates(ev.CursorLine)
			ev.engine.InvalidateFrom(ev.CursorLine)
			ev.CursorPos += len(data)
			ev.updateDesiredVisualCol()
			ev.ensureCursorVisible()

			if ev.acEnabled {
				ev.updateAutocomplete()
			}
			return true
		}
	}

	if e.Char != 0 && ctrl == false {
		ev.saveUndo(opTyping)
		if ev.selActive {
			ev.inGroup = true
			ev.DeleteSelection()
			ev.inGroup = false
		}
		ev.modified = true
		offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		if ev.CursorVirtualSpaces > 0 {
			spaces := []byte(strings.Repeat(" ", ev.CursorVirtualSpaces))
			ev.pt.Insert(offset, spaces)
			ev.li.UpdateAfterInsert(offset, spaces)
			offset += ev.CursorVirtualSpaces
			ev.CursorPos += ev.CursorVirtualSpaces
			ev.CursorVirtualSpaces = 0
		}

		data := []byte(string(e.Char))

		if ev.overtype {
			lineLen := ev.getLineLength(ev.CursorLine)
			if ev.CursorPos < lineLen {
				peekLen := 4
				if lineLen-ev.CursorPos < 4 {
					peekLen = lineLen - ev.CursorPos
				}
				oldData, _ := ev.pt.GetRange(offset, peekLen)
				size := 1
				if oldData != nil && len(oldData) > 0 {
					_, rsize := utf8.DecodeRune(oldData)
					if rsize > 0 {
						size = rsize
					}
				}
				ev.pt.Delete(offset, size)
				ev.li.UpdateAfterDelete(offset, size)
			}
		}

		ev.pt.Insert(offset, data)
		ev.li.UpdateAfterInsert(offset, data)
		ev.invalidateStates(ev.CursorLine)
		ev.engine.InvalidateFrom(ev.CursorLine)
		ev.CursorPos += len(data)
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()

		if ev.acEnabled {
			ev.updateAutocomplete()
		}
		return true
	}

	return false
}

func (ev *EditorView) fillCells(target []vtui.CharInfo, data []byte, defaultAttr, selAttr uint64, offset int, selActive bool, selMin, selMax int, syntax []uint64, startVisualCol int, isCrossRow bool, crossVCol int, crossAttr uint64) []vtui.CharInfo {
	target = target[:0]
	currByte := 0
	charIdx := 0
	visualCol := startVisualCol
	tabSize := ev.TabSize
	if tabSize <= 0 {
		tabSize = 8
	}

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		displayRune, w := vtui.SanitizeRune(r)
		if r == '\t' {
			w = tabSize - (visualCol % tabSize)
			displayRune = ' '
			if ev.ShowWhitespaces {
				displayRune = '→'
			}
		} else if r == ' ' && ev.ShowWhitespaces {
			displayRune = '·'
		} else if r < 0x20 || r == 0x7F {
			if !ev.ShowWhitespaces {
				displayRune = ' '
			}
		}

		attr := defaultAttr
		if charIdx < len(syntax) {
			attr = syntax[charIdx]
		}

		// Horizontal crosshair line applies to the entire character in the active row
		if isCrossRow && crossAttr != 0 {
			if crossAttr&vtui.IsBgRGB != 0 {
				attr = vtui.SetRGBBack(attr, vtui.GetRGBBack(crossAttr))
			} else {
				attr = vtui.SetIndexBack(attr, vtui.GetIndexBack(crossAttr))
			}
		}

		if selActive {
			absPos := offset + currByte
			if absPos >= selMin && absPos < selMax {
				attr = selAttr
			}
		}
		charIdx++
		currByte += size

		if w > 0 {
			charVal := uint64(displayRune)
			for j := 0; j < w; j++ {
				cellAttr := attr
				// Vertical crosshair line: apply ONLY to the specific cell index
				if !isCrossRow && (visualCol+j == crossVCol) && crossAttr != 0 {
					if crossAttr&vtui.IsBgRGB != 0 {
						cellAttr = vtui.SetRGBBack(cellAttr, vtui.GetRGBBack(crossAttr))
					} else {
						cellAttr = vtui.SetIndexBack(cellAttr, vtui.GetIndexBack(crossAttr))
					}
				}
				target = append(target, vtui.CharInfo{Char: charVal, Attributes: cellAttr})
				charVal = uint64(vtui.WideCharFiller)
				if r == '\t' {
					charVal = ' '
				}
			}
			visualCol += w
		}
	}
	return target
}

func (ev *EditorView) ensureCursorVisible() {
	if ev.targetLine != -1 {
		return // Skip clamping and scrolling while waiting for the target line to be indexed
	}

	// Safety constraints for binary files or corrupted indices
	if ev.CursorLine < 0 {
		ev.CursorLine = 0
	}
	if ev.CursorLine >= ev.li.LineCount() {
		ev.CursorLine = ev.li.LineCount() - 1
	}

	lineLen := ev.getLineLength(ev.CursorLine)
	if ev.CursorPos < 0 {
		ev.CursorPos = 0
	}
	if ev.CursorPos > lineLen {
		ev.CursorPos = lineLen
	}

	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	vRow, vCol := ev.engine.LogicalToVisual(curOffset)
	vCol += ev.CursorVirtualSpaces

	width := ev.X2 - ev.X1 + 1
	height := ev.Y2 - ev.Y1

	if ev.scrollBar != nil {
		width--
	}
	if width <= 0 || height <= 0 {
		return
	}

	// 1. Вертикальный скролл
	if vRow < ev.ScrollTopRow {
		ev.ScrollTopRow = vRow
	} else if vRow >= ev.ScrollTopRow+height {
		ev.ScrollTopRow = vRow - height + 1
	}

	// 2. Горизонтальный скролл (только если WordWrap выключен)
	if !ev.WordWrap {
		if vCol < ev.ScrollLeft {
			ev.ScrollLeft = vCol
		} else if vCol >= ev.ScrollLeft+width {
			ev.ScrollLeft = vCol - width + 1
		}
		if ev.ScrollLeft < 0 {
			ev.ScrollLeft = 0
		}
	} else {
		ev.ScrollLeft = 0
	}
}

func (ev *EditorView) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	if e.ButtonState != 0 && ev.targetLine != -1 {
		ev.targetLine = -1
		ev.ensureCursorVisible()
	}

	if ev.scrollBar != nil && ev.scrollBar.ProcessMouse(e) {
		return true
	}

	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 {
			ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
		} else {
			ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
		}
		return true
	}
	return false
}

func (ev *EditorView) SetPosition(x1, y1, x2, y2 int) {
	ev.ScreenObject.SetPosition(x1, y1, x2, y2)
	if ev.topBar != nil {
		ev.topBar.SetPosition(x1, y1, x2, y1)
	}
	if ev.menuBar != nil {
		ev.menuBar.SetPosition(x1, 0, x2, 0)
	}
	if ev.scrollBar != nil {
		ev.scrollBar.SetPosition(x2, y1+1, x2, y2)
		ev.scrollBar.PgStep = y2 - y1
	}
	ev.ensureEngineWidth()
	ev.ensureCursorVisible()
}

func (ev *EditorView) ResizeConsole(w, h int) {
	// Редактор в f4 занимает всё пространство до KeyBar (h-1)
	ev.SetPosition(0, 0, w-1, h-2)
}

func (ev *EditorView) GetMenuBar() *vtui.MenuBar { return ev.menuBar }

func (ev *EditorView) StartIndexing() {
	if ev.asyncBuf == nil {
		return
	}
	if ev.indexCancel != nil {
		ev.indexCancel()
	}

	ev.editSession++
	sessionID := ev.editSession

	ctx, cancel := context.WithCancel(context.Background())
	ev.indexCancel = cancel

	go func() {
		absPos := 0
		chunkSize := 64 * 1024
		buf := ev.asyncBuf
		li := ev.li
		maxSize := buf.Size()

		// BATCHING OPTIMIZATION: Collect offsets and update UI in larger chunks
		// to reduce main thread overhead and prevent "redraw storms".
		pendingOffsets := make([]int, 0, 1000)

		for absPos < maxSize {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if ev.IsDone() {
				return
			}

			data, err := buf.Read(absPos, chunkSize)
			if err == piecetable.ErrLoading {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if err != nil {
				break
			}

			for i, b := range data {
				if b == '\n' {
					pendingOffsets = append(pendingOffsets, absPos+i+1)
				}
			}

			absPos += len(data)

			// Update UI if we have enough lines or reached EOF
			if len(pendingOffsets) >= 500 || absPos >= maxSize {
				currentBatch := pendingOffsets
				pendingOffsets = make([]int, 0, 1000)

				vtui.FrameManager.PostTask(func() {
					if ctx.Err() != nil || ev.edited || ev.editSession != sessionID {
						return
					}
					// Incremental update: we only need to invalidate visual cache
					// from the line that was previously the "last" one.
					lastLineBefore := li.LineCount() - 1
					li.AppendOffsets(currentBatch, maxSize)

					if ev.targetLine != -1 && (li.LineCount() > ev.targetLine || absPos >= maxSize) {
						ev.CursorLine = ev.targetLine
						if ev.CursorLine >= li.LineCount() {
							ev.CursorLine = li.LineCount() - 1
						}
						if ev.CursorLine < 0 {
							ev.CursorLine = 0
						}
						ev.CursorPos = ev.targetPos
						ev.ScrollTopRow = ev.targetTopRow
						ev.ScrollLeft = ev.targetLeft
						if ev.ScrollLeft < 0 {
							ev.ScrollLeft = 0
						}
						ev.targetLine = -1
						ev.ensureCursorVisible()
						ev.updateDesiredVisualCol()
					}

					ev.engine.InvalidateFrom(lastLineBefore)
					vtui.FrameManager.Redraw()
				})
			}
		}

		vtui.FrameManager.PostTask(func() {
			if ctx.Err() == nil && !ev.edited && ev.editSession == sessionID {
				if ev.targetLine != -1 {
					ev.CursorLine = ev.targetLine
					if ev.CursorLine >= li.LineCount() {
						ev.CursorLine = li.LineCount() - 1
					}
					if ev.CursorLine < 0 {
						ev.CursorLine = 0
					}
					ev.CursorPos = ev.targetPos
					ev.ScrollTopRow = ev.targetTopRow
					ev.ScrollLeft = ev.targetLeft
					if ev.ScrollLeft < 0 {
						ev.ScrollLeft = 0
					}
					ev.targetLine = -1
					ev.ensureCursorVisible()
					ev.updateDesiredVisualCol()
					vtui.FrameManager.Redraw()
				}
			}
		})
		vtui.DebugLog("INDEXER: Finished for %s", ev.filePath)
	}()
}

func (ev *EditorView) HandleCommand(cmd int, args any) bool {
	if cmd == vtui.CmClose {
		ev.tryClose()
		return true
	}
	if cmd == CmSearch {
		ev.showSearchDialog()
		return true
	}
	return ev.BaseFrame.HandleCommand(cmd, args)
}

func (ev *EditorView) tryClose() {
	if !ev.modified {
		ev.Close()
		return
	}

	msg := "The file has been modified.\nDo you want to save it?"
	dlg := vtui.ShowMessage(" Confirm ", msg, []string{"&Save", "&Don't Save", "Cancel"})
	dlg.OnResult = func(code int) {
		switch code {
		case 0: // Save
			ev.SaveToFile(func() {
				ev.Close()
			})
		case 1: // Don't save
			ev.Close()
		}
	}
}

func (ev *EditorView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			Msg("KeyBar.EditorF1"), Msg("KeyBar.EditorF2"), Msg("KeyBar.EditorF3"),
			"", Msg("KeyBar.EditorF5"), "", Msg("KeyBar.EditorF7"), "", "", Msg("KeyBar.EditorF10"),
		},
	}
}
func (ev *EditorView) showSearchDialog() {
	dlgW, dlgH := 50, 11
	dlg := vtui.NewCenteredDialog(dlgW, dlgH, Msg("Viewer.SearchTitle"))
	dlg.ShowClose = true

	lblPrompt := vtui.NewLabel(0, 0, "Search for:", nil)
	editPattern := vtui.NewEdit(0, 0, 30, LastEditorSearch)
	editPattern.SelectAll()
	lblPrompt.FocusLink = editPattern
	dlg.SetFocusedItem(editPattern)

	chkCase := vtui.NewCheckbox(0, 0, Msg("Search.CaseSensitive"), false)
	chkCase.State = 0
	if LastEditorSearchCase {
		chkCase.State = 1
	}

	chkReverse := vtui.NewCheckbox(0, 0, Msg("Search.Reverse"), false)
	chkReverse.State = 0
	if LastEditorSearchReverse {
		chkReverse.State = 1
	}

	btnFind := vtui.NewButton(0, 0, "&Find")
	btnFind.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, "Cancel")

	dlg.AddItem(lblPrompt)
	dlg.AddItem(editPattern)
	dlg.AddItem(chkCase)
	dlg.AddItem(chkReverse)
	dlg.AddItem(btnFind)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, dlgW-4, dlgH-4)
	vbox.Add(lblPrompt, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editPattern, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(chkCase, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkReverse, vtui.Margins{}, vtui.AlignLeft)

	hbox := vtui.NewHBoxLayout(0, 0, dlgW-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnFind, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnFind.OnClick = func() {
		LastEditorSearch = editPattern.GetText()
		LastEditorSearchCase = chkCase.State == 1
		LastEditorSearchReverse = chkReverse.State == 1
		SaveSession()
		dlg.Close()
		ev.Search(LastEditorSearch, LastEditorSearchCase, LastEditorSearchReverse, false)
	}
	btnCancel.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}
func (ev *EditorView) getLogicalLineRunes(line int) []rune {
	lineStart := ev.li.GetLineOffset(line)
	lineLen := ev.getLineLength(line)
	// Prevent OOM on huge binary lines for word navigation
	const maxRuneFetch = 32 * 1024
	if lineLen > maxRuneFetch {
		lineLen = maxRuneFetch
	}
	lineData, _ := ev.pt.GetRange(lineStart, lineLen)
	return []rune(string(lineData))
}
func (ev *EditorView) getLineLength(line int) int {
	if line < 0 || line >= ev.li.LineCount() {
		return 0
	}
	start := ev.li.GetLineOffset(line)
	size := ev.pt.Size()
	end := size
	if line+1 < ev.li.LineCount() {
		end = ev.li.GetLineOffset(line + 1)
	}

	totalLen := end - start
	if totalLen <= 0 {
		return 0
	}

	// Use a small buffer to check just the end of the line for line breaks
	// to avoid loading massive binary lines entirely.
	checkLen := 2
	if totalLen < checkLen {
		checkLen = totalLen
	}

	data, err := ev.pt.GetRange(start+totalLen-checkLen, checkLen)
	if err != nil || len(data) == 0 {
		return totalLen
	}

	// Safely decrease length if there are line breaks at the end.
	// We work with the end of the returned buffer.
	if data[len(data)-1] == '\n' {
		totalLen--
		if len(data) > 1 && data[len(data)-2] == '\r' {
			totalLen--
		}
	}
	return totalLen
}

func (ev *EditorView) SaveToFile(afterSave func()) {
	if ev.filePath == "" || ev.vfs == nil || ev.saving {
		return
	}

	ev.saving = true
	ev.edited = true
	vtui.DebugLog("EDITOR: Saving %s...", ev.filePath)

	// Stop indexing to prevent async reads on closed buffers
	if ev.indexCancel != nil {
		ev.indexCancel()
		ev.indexCancel = nil
	}

	// Capture visible offset for preloading before we destroy the current engine
	visStart := ev.engine.VisualToLogical(ev.ScrollTopRow, 0)

	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		// To preserve original file ownership, permissions and xattrs (crucial for root-owned files),
		// we write directly to the original file instead of using atomic rename via temp file.
		oldAsync := ev.asyncBuf
		oldFile := ev.file
		//needsBufferRecovery := oldAsync != nil && ev.pt.GetOriginalBuffer() == oldAsync

		// Capture original metadata to restore it after atomic rename
		originalStat, statErr := ev.vfs.Stat(ctx.Context, ev.filePath)

		tempPath := ev.filePath + ".f4tmp"
		f, err := ev.vfs.Create(ctx.Context, tempPath)
		if err != nil {
			ctx.RunOnUI(func() {
				ev.saving = false
				vtui.DebugLog("EDITOR: Failed to create temp file for saving: %v", err)
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to create temporary file:\n%v", err), []string{"&Ok"})
			})
			return
		}

		// Streaming write loop with retry logic for async loading.
		// We use GetRange instead of ForEachRange to safely handle ErrLoading without closure mess.
		var saveErr error
		curr := 0
		total := ev.pt.Size()
		for curr < total {
			if ctx.Err() != nil {
				saveErr = ctx.Err()
				break
			}
			take := 256 * 1024 // 256KB chunks
			if curr+take > total {
				take = total - curr
			}
			data, errRange := ev.pt.GetRange(curr, take)
			if errRange == piecetable.ErrLoading {
				time.Sleep(20 * time.Millisecond)
				continue
			}
			if errRange != nil {
				saveErr = errRange
				break
			}
			if _, errWrite := f.Write(data); errWrite != nil {
				saveErr = errWrite
				break
			}
			curr += len(data)
		}
		f.Close()

		if saveErr != nil {
			ev.vfs.Remove(ctx.Context, tempPath)
			ctx.RunOnUI(func() {
				ev.saving = false
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to save data:\n%v", saveErr), []string{"&Ok"})
			})
			return
		}

		// Success: finalize the save atomically.
		// Close reading handles to the OLD file so it can be replaced by Rename.
		if oldAsync != nil {
			oldAsync.Close()
		}
		if oldFile != nil {
			oldFile.Close()
		}

		if err := ev.vfs.Rename(ctx.Context, tempPath, ev.filePath); err != nil {
			ctx.RunOnUI(func() {
				ev.saving = false
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to finalize save (rename failed):\n%v", err), []string{"&Ok"})
			})
			return
		}

		// Restore original metadata (owner, group, perms, times)
		if statErr == nil {
			ev.vfs.SetAttributes(ctx.Context, ev.filePath, originalStat)
		}

		newFile, err := ev.vfs.Open(ctx.Context, ev.filePath)
		var newPt *piecetable.PieceTable
		var newEngine *textlayout.WrapEngine
		var newBuf *AsyncBuffer

		if err == nil {
			newBuf = NewAsyncBuffer(ctx.Context, newFile)
			newPt = piecetable.NewWithBuffer(newBuf)
			// Reuse the existing LineIndex since the logical content is identical
			newEngine = textlayout.NewWrapEngine(newPt, ev.li)
		}

		// PRELOAD CACHE TO PREVENT SCREEN FLICKER
		// This MUST be outside RunOnUI to prevent blocking the main thread for 500ms.
		for i := 0; i < 50; i++ { // max 500ms
			if ctx.Err() != nil {
				break
			}
			_, e := newBuf.Read(visStart, 4096)
			if e != piecetable.ErrLoading {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}

		ctx.RunOnUI(func() {
			ev.saving = false
			if err == nil {
				vtui.DebugLog("EDITOR: Successfully saved %s (%d bytes)", ev.filePath, ev.pt.Size())
			}
			vtui.FrameManager.Broadcast(CmFileChanged, nil)

			if err == nil {
				ev.modified = false
				if afterSave != nil {
					afterSave()
				}
				ev.file = newFile
				ev.asyncBuf = newBuf
				ev.pt = newPt
				ev.cleanState = newPt.GetState()
				ev.engine = newEngine
				ev.editSession++
				ev.ensureEngineWidth()
				ev.edited = false
			}
		})
	})
}

func (ev *EditorView) getSelectionRange() (int, int) {
	if !ev.selActive {
		return 0, 0
	}
	cursorOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	min, max := ev.selAnchorOffset, cursorOffset
	if min > max {
		min, max = max, min
	}
	return min, max
}

func (ev *EditorView) CopySelection() {
	min, max := ev.getSelectionRange()
	if max > min {
		data, _ := ev.pt.GetRange(min, max-min)
		if data != nil {
			vtui.SetClipboard(string(data))
			vtui.DebugLog("EDITOR: Copied %d bytes to clipboard", max-min)
		}
	}
}

func (ev *EditorView) PasteText(text string) {
	if !ev.edited {
		ev.edited = true
		if ev.indexCancel != nil {
			ev.indexCancel()
		}
	}
	ev.editSession++

	ev.saveUndo(opOther)
	ev.inGroup = true
	if ev.selActive {
		ev.DeleteSelection()
	}
	ev.inGroup = false

	offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	data := []byte(text)
	ev.pt.Insert(offset, data)
	ev.li.UpdateAfterInsert(offset, data)
	ev.invalidateStates(ev.CursorLine)
	ev.engine.InvalidateFrom(ev.CursorLine)

	newOffset := offset + len(data)
	ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
	ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)
	ev.modified = true
	ev.updateDesiredVisualCol()
	ev.ensureCursorVisible()
}
func (ev *EditorView) DeleteSelection() {
	min, max := ev.getSelectionRange()
	// Safety clamp to prevent panic on stale selection ranges
	if min < 0 {
		min = 0
	}
	if max > ev.pt.Size() {
		max = ev.pt.Size()
	}
	if max > min {
		if !ev.edited {
			ev.edited = true
			if ev.indexCancel != nil {
				ev.indexCancel()
			}
		}
		ev.editSession++

		ev.saveUndo(opOther)

		ev.modified = true
		ev.pt.Delete(min, max-min)
		// Incremental update
		ev.li.UpdateAfterDelete(min, max-min)
		ev.clearCaches()
		ev.selActive = false
		// Update cursor position to the start of the former selection
		ev.CursorLine = ev.li.GetLineAtOffset(min)
		ev.CursorPos = min - ev.li.GetLineOffset(ev.CursorLine)
	}
}
func (ev *EditorView) GetType() vtui.FrameType { return vtui.TypeUser + 2 }
func (ev *EditorView) IsBusy() bool            { return ev.pasting || ev.saving }
func (ev *EditorView) GetTitle() string {
	if ev.filePath != "" {
		return "Edit: " + filepath.Base(ev.filePath)
	}
	return "Editor"
}
func (ev *EditorView) Search(pattern string, caseSensitive, reverse, next bool) {
	if pattern == "" {
		return
	}
	if LastEditorSearch != pattern || LastEditorSearchCase != caseSensitive || LastEditorSearchReverse != reverse {
		LastEditorSearch = pattern
		LastEditorSearchCase = caseSensitive
		LastEditorSearchReverse = reverse
		SaveSession()
	}

	vtui.DebugLog("EDITOR_SEARCH: Starting for %q (sensitive=%v, reverse=%v, next=%v). CursorPos=%d",
		pattern, caseSensitive, reverse, next, ev.li.GetLineOffset(ev.CursorLine)+ev.CursorPos)

	title := " Searching... "
	msg := fmt.Sprintf("Looking for: %s", pattern)

	vtui.FrameManager.PostTask(func() {
		dlg := vtui.NewCenteredDialog(50, 8, title)
		lbl := vtui.NewLabel(0, 0, msg, nil)
		dlg.AddItem(lbl)
		btnCancel := vtui.NewButton(0, 0, "&Cancel")
		dlg.AddItem(btnCancel)

		vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 8-4)
		vbox.Add(lbl, vtui.Margins{}, vtui.AlignCenter)
		vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
		vbox.Apply()

		vtui.FrameManager.AddScreenHeadless(dlg)

		_ = vtui.RunAsync(func(ctx *vtui.TaskContext) {
			btnCancel.OnClick = func() { ctx.Cancel(); dlg.Close() }

			startOff := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			foundOffset := -1
			totalSize := ev.pt.Size()
			chunkSize := 256 * 1024

			match := func(data string) int {
				searchData, searchPat := data, pattern
				if !caseSensitive {
					searchData, searchPat = strings.ToLower(data), strings.ToLower(pattern)
				}
				res := -1
				if reverse {
					res = strings.LastIndex(searchData, searchPat)
				} else {
					res = strings.Index(searchData, searchPat)
				}
				if res != -1 {
					vtui.DebugLog("EDITOR_SEARCH: Internal match found at relative index %d in chunk (chunk size %d)", res, len(data))
				}
				return res
			}

			if !reverse {
				currOff := startOff
				if next {
					currOff++
				}

				vtui.DebugLog("EDITOR_SEARCH: Running forward search from offset %d", currOff)

				for currOff < totalSize {
					if ctx.Err() != nil {
						return
					}
					percent := 0
					if totalSize > 0 {
						percent = int((currOff * 100) / totalSize)
					}
					if totalSize > 0 {
						ctx.RunOnUI(func() { dlg.SetProgress(percent) })
					}

					readSize := chunkSize
					if currOff+readSize > totalSize {
						readSize = totalSize - currOff
					}

					vtui.DebugLog("EDITOR_SEARCH: Reading segment at %d, size %d", currOff, readSize)
					data, err := ev.pt.GetRange(currOff, readSize)
					if err == piecetable.ErrLoading {
						time.Sleep(20 * time.Millisecond)
						continue
					}
					if len(data) == 0 {
						break
					}

					idx := match(string(data))
					if idx != -1 {
						foundOffset = currOff + idx
						break
					}
					advance := len(data) - len(pattern)
					if advance <= 0 {
						advance = 1
					}
					currOff += advance
					if len(data) < chunkSize {
						break
					}
				}
			} else {
				currOff := startOff
				if next {
					currOff--
				}

				vtui.DebugLog("EDITOR_SEARCH: Running backward search from offset %d", currOff)

				for currOff >= 0 {
					if ctx.Err() != nil {
						return
					}
					if totalSize > 0 {
						percent := int(((totalSize - currOff) * 100) / totalSize)
						ctx.RunOnUI(func() { dlg.SetProgress(percent) })
					}

					readStart := currOff - chunkSize
					if readStart < 0 {
						readStart = 0
					}
					readSize := currOff - readStart

					vtui.DebugLog("EDITOR_SEARCH: Reading segment at %d, size %d", readStart, readSize)
					data, err := ev.pt.GetRange(readStart, readSize)
					if err == piecetable.ErrLoading {
						time.Sleep(20 * time.Millisecond)
						continue
					}
					if len(data) == 0 {
						break
					}

					idx := match(string(data))
					if idx != -1 {
						foundOffset = readStart + idx
						break
					}
					if readStart == 0 {
						break
					}
					currOff = readStart + len(pattern) - 1
					if currOff >= readStart+readSize {
						currOff = readStart
					}
				}
			}

			ctx.RunOnUI(func() {
				dlg.Close()
				if foundOffset != -1 {
					vtui.DebugLog("EDITOR_SEARCH: UI update: Found pattern at offset %d. Updating cursor and selection.", foundOffset)
					ev.selActive = true
					ev.selAnchorOffset = foundOffset

					endFound := foundOffset + len(pattern)
					ev.CursorLine = ev.li.GetLineAtOffset(endFound)
					ev.CursorPos = endFound - ev.li.GetLineOffset(ev.CursorLine)

					ev.updateDesiredVisualCol()
					ev.ensureCursorVisible()
					vtui.FrameManager.Redraw()
				} else if ctx.Err() == nil {
					vtui.DebugLog("EDITOR_SEARCH: UI update: Pattern NOT FOUND.")
					vtui.ShowMessage(" Search ", "Pattern not found.", []string{"&Ok"})
				}
			})
		})
	})
}

const (
	catSpace = iota
	catDivider
	catWord
)

func getCharCategory(r rune) int {
	if r == ' ' || r == '\t' {
		return catSpace
	}
	if strings.ContainsRune("~!%^&*()+|{}:\"<>?`-=\\[];',./", r) {
		return catDivider
	}
	return catWord
}

// updateAutocomplete scans nearby lines for words matching the current prefix.
func (ev *EditorView) updateAutocomplete() {
	ev.acMatches = nil
	if ev.CursorPos == 0 {
		return
	}

	lineLen := ev.getLineLength(ev.CursorLine)
	lineStart := ev.li.GetLineOffset(ev.CursorLine)

	// Disable if we are in the middle of a word (peek at the character under cursor)
	if ev.CursorPos < lineLen {
		dataUnder, _ := ev.pt.GetRange(lineStart+ev.CursorPos, 4)
		if len(dataUnder) > 0 {
			r, _ := utf8.DecodeRune(dataUnder)
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
				return
			}
		}
	}

	lineData, _ := ev.pt.GetRange(lineStart, ev.CursorPos)
	if len(lineData) == 0 {
		return
	}

	runes := []rune(string(lineData))

	// Find prefix by going backwards until non-word character
	prefixStart := len(runes)
	for i := len(runes) - 1; i >= 0; i-- {
		r := runes[i]
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			break
		}
		prefixStart = i
	}

	if prefixStart == len(runes) {
		return // No word char before cursor
	}

	ev.acPrefix = string(runes[prefixStart:])

	// Minimum prefix length to trigger suggestions (like in far2l)
	if len([]rune(ev.acPrefix)) < 2 {
		return
	}

	// Scan lines around cursor
	maxDelta := 256
	startL := ev.CursorLine - maxDelta
	if startL < 0 {
		startL = 0
	}
	endL := ev.CursorLine + maxDelta
	if endL >= ev.li.LineCount() {
		endL = ev.li.LineCount() - 1
	}

	seen := make(map[string]bool)
	var currentLineMatches []string
	var otherLineMatches []string

	// Fast word extractor
	extractWords := func(lineIdx int) {
		lStart := ev.li.GetLineOffset(lineIdx)
		lLen := ev.getLineLength(lineIdx)
		// Limit to 512 bytes per line to avoid lag on minified files
		if lLen > 512 {
			lLen = 512
		}

		data, _ := ev.pt.GetRange(lStart, lLen)
		if len(data) == 0 {
			return
		}

		lRunes := []rune(string(data))
		wordStart := -1

		for i, r := range lRunes {
			isWord := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
			if isWord {
				if wordStart == -1 {
					wordStart = i
				}
			} else {
				if wordStart != -1 {
					word := string(lRunes[wordStart:i])
					if strings.HasPrefix(word, ev.acPrefix) && word != ev.acPrefix && !seen[word] {
						seen[word] = true
						if lineIdx == ev.CursorLine {
							currentLineMatches = append(currentLineMatches, word)
						} else {
							otherLineMatches = append(otherLineMatches, word)
						}
					}
					wordStart = -1
				}
			}
		}
		// Check tail
		if wordStart != -1 {
			word := string(lRunes[wordStart:])
			if strings.HasPrefix(word, ev.acPrefix) && word != ev.acPrefix && !seen[word] {
				seen[word] = true
				if lineIdx == ev.CursorLine {
					currentLineMatches = append(currentLineMatches, word)
				} else {
					otherLineMatches = append(otherLineMatches, word)
				}
			}
		}
	}

	// Prioritize current line, then others
	extractWords(ev.CursorLine)
	for i := startL; i <= endL; i++ {
		if i != ev.CursorLine {
			extractWords(i)
		}
	}

	if len(currentLineMatches) > 0 || len(otherLineMatches) > 0 {
		ev.acMatches = append(currentLineMatches, otherLineMatches...)
		ev.acCurrentIdx = 0
	}
}
