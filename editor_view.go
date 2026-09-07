package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/charlievieth/strcase"
	"github.com/coregx/coregex"
	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)
import "golang.org/x/arch/x86/x86asm"

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
	LastEditorSearch          string
	LastEditorReplace         string
	LastEditorSearchCase      bool
	LastEditorSearchReverse   bool
	LastEditorSearchRegexp    bool
	LastEditorSearchWholeWord bool
	LastEditorSearchHex       bool
)
var GlobalLastClipboardWasRectangular bool

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
	// Acknowledges semantic GUI scroll/window requests, including clamped
	// no-ops at the beginning/end of the document.
	semanticWindowGeneration        uint64
	semanticWindowRequestGeneration uint64
	semanticPendingScroll           bool
	semanticPendingTop              int
	semanticPendingGeneration       uint64
	semanticExtentTaskPending       bool
	semanticExtentTaskRevision      uint64
	semanticExtentKnown             bool
	semanticLoadError               string
	// nativeViewportRows is the number of complete rows left after the QML
	// document header and shared native chrome have been laid out. A zero value
	// keeps the terminal geometry authoritative.
	nativeViewportRows      int
	nativeViewportColumns   int
	nativeViewportRevision  uint64
	semanticLayoutRevision  uint64
	layoutWidth             int
	layoutWordWrap          bool
	layoutTabSize           int
	reflow                  *editorReflow
	reflowTaskPending       bool
	semanticPointerActive   bool
	semanticPointerRevision uint64
	// The Qt semantic surface slides a bounded three-viewport window through
	// the document. Keep final styled rows for the current overlap so an edge
	// scroll or selection endpoint move only repaints the rows whose pixels
	// actually changed.
	semanticStyledRows             map[int]semanticEditorStyledRowCacheEntry
	semanticStyledRowsContext      semanticEditorStyledRowsContext
	semanticStyledRowsContextValid bool
	semanticStyledRowsRendered     uint64

	WordWrap         bool
	HexMode          bool
	DecodeMode       bool
	HexTopOffset     int
	HexNibble        int // 0 = high nibble, 1 = low nibble
	DisasmMode       int // 16, 32, or 64
	overtype         bool
	modified         bool
	CursorLine       int // Текущая логическая строка (для плагинов)
	CursorPos        int // Позиция в байтах (для плагинов)
	DesiredVisualCol int // Колонка, в которую мы хотим попасть при навигации Up/Down

	ShowWhitespaces  bool
	selActive        bool
	selAnchorOffset  int // Абсолютное смещение начала выделения
	rectSelActive    bool
	rectSelStartLine int
	rectSelStartCol  int
	editSession      int // Unique ID to fence background tasks

	pasting     bool
	saving      bool
	edited      bool
	pasteBuffer []rune
	asyncBuf    *AsyncBuffer
	indexCancel context.CancelFunc
	// indexing is true from StartIndexing until that run ends, by completion
	// or by cancellation. indexCancel cannot answer the question: it is left
	// set after a normal finish, so it reads as "indexing forever".
	indexing    bool
	renderBytes []byte          // Reusable buffer for text data
	renderCells []vtui.CharInfo // Reusable buffer for row rendering

	vfs         vfs.VFS
	filePath    string
	file        vfs.ReadAtCloser
	scrollBar   *vtui.ScrollBar
	highlighter vtui.Highlighter
	lineStates  []any // Cache of highlighter states per logical line

	// Highlighting arrives late and all at once; these ease it in.
	syntaxFadeStart time.Time
	fadeBuf         []uint64

	// Undo/Redo
	undoStack  []editorState
	redoStack  []editorState
	inGroup    bool
	lastOp     undoOpType
	cleanState piecetable.TableState // State of the file on disk
	// unsavedBaseline keeps a create-new buffer dirty until its first
	// successful save, even when edit + Undo restores the supplied content.
	unsavedBaseline bool
	// createNewTarget forbids replacing a path that appeared after this
	// editor was opened. Plugin-generated reports use this for their first
	// save; normal editors continue to replace the file they opened.
	createNewTarget bool

	// Autocomplete state
	acEnabled    bool
	acPrefix     string
	acMatches    []string
	acCurrentIdx int

	targetLine   int
	targetPos    int
	targetTopRow int
	targetLeft   int
	Codepage     int
	// DisplayTitle overrides the temporary filename in frame and top-bar
	// titles. It is used by internal editor round-trip workflows.
	DisplayTitle string

	TabSize             int
	ExpandTabs          int
	AutoIndent          bool
	CursorBeyondEOL     bool
	CursorVirtualSpaces int
	UseEditorConfig     bool

	highlighting        bool
	highlightCancel     context.CancelFunc
	highlightGeneration uint64

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
	ev.semanticPendingScroll = false
	ev.semanticPointerActive = false
	ev.editSession++
	if GlobalFileState != nil && ev.filePath != "" {
		GlobalFileState.SaveEditorStateAsync(FileStateKey(ev.vfs, ev.filePath), ev.CursorLine, ev.CursorPos, ev.ScrollTopRow, ev.ScrollLeft, ev.WordWrap)
	}
	ev.cancelHighlighting()
	if ev.indexCancel != nil {
		ev.indexCancel()
	}
	if ev.asyncBuf != nil {
		ev.asyncBuf.Close()
	}
	if ev.file != nil {
		ev.file.Close()
	}
	if closer, ok := ev.highlighter.(io.Closer); ok {
		closer.Close()
	}
	ev.BaseFrame.Close()
	if ev.OnClose != nil {
		ev.OnClose()
	}
}

func NewEditorView(pt *piecetable.PieceTable, v vfs.VFS, path string) *EditorView {
	return newEditorView(pt, v, path, true)
}

func newEditorView(pt *piecetable.PieceTable, v vfs.VFS, path string, useEditorConfig bool) *EditorView {
	li := piecetable.NewLineIndex()
	indexSource := pt
	if buf, ok := pt.GetOriginalBuffer().(*AsyncBuffer); ok {
		// The first block is already available when opening a streamed file.
		// ForEachRange requests a megabyte at a time; using it here discards
		// that ready prefix when the rest returns ErrLoading, leaving a blank
		// first window until the background indexer publishes a batch.
		prefix, _ := pt.GetRange(0, min(pt.Size(), buf.chunkSize))
		indexSource = piecetable.New(prefix)
	}
	li.Rebuild(indexSource)
	ev := &EditorView{
		pt:                  pt,
		li:                  li,
		engine:              textlayout.NewWrapEngine(pt, li),
		vfs:                 v,
		filePath:            path,
		WordWrap:            false,
		ShowWhitespaces:     false,
		cleanState:          pt.GetState(),
		targetLine:          -1,
		targetPos:           -1,
		targetTopRow:        -1,
		targetLeft:          -1,
		TabSize:             AppConfig.EditorTabSize,
		ExpandTabs:          AppConfig.EditorExpandTabs,
		AutoIndent:          AppConfig.EditorAutoIndent,
		CursorBeyondEOL:     AppConfig.EditorCursorBeyondEOL,
		UseEditorConfig:     useEditorConfig && AppConfig.EditorUseEditorConfig,
		Codepage:            65001,
		semanticExtentKnown: indexSource.Size() == pt.Size(),
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
	switch {
	case strings.EqualFold(AppConfig.EditorHighlighter, "None"):
		ev.highlighter = nil
	case strings.EqualFold(AppConfig.EditorHighlighter, "Colorer") && SchemasExist():
		firstLine := ""
		if probeLen := pt.Size(); probeLen > 0 {
			if probeLen > 1024 {
				probeLen = 1024
			}
			b, _ := pt.GetRange(0, probeLen)
			firstLine = string(b)
			if idx := strings.IndexAny(firstLine, "\r\n"); idx >= 0 {
				firstLine = firstLine[:idx]
			}
		}
		ev.highlighter = newColorerHighlighter(ev, filepath.Base(path), firstLine, vtui.GetHighlighter(path, ""))
	default:
		ev.highlighter = vtui.GetHighlighter(path, "")
	}
	vtui.DebugLog("EDITOR_INIT: Path=%q, Highlighter=%T", path, ev.highlighter)
	ev.scrollBar = vtui.NewScrollBar(0, 0, 0)
	ev.scrollBar.SetOwner(ev)
	ev.scrollBar.OnScroll = func(v int) {
		if ev.HexMode || ev.DecodeMode {
			oldOffset := ev.HexTopOffset
			if ev.HexMode {
				ev.HexTopOffset = v &^ 0xF
			} else {
				ev.HexTopOffset = v
			}
			if ev.HexTopOffset != oldOffset {
				ev.fenceSemanticNavigation()
			}
			vtui.FrameManager.Redraw()
			return
		}
		if ev.ScrollTopRow != v {
			ev.fenceSemanticNavigation()
		}
		ev.ScrollTopRow = v
		height := ev.viewportHeight()
		if height > 0 {
			startLogLine, _ := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
			endLogLine, _ := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow + height - 1)
			if ev.CursorLine < startLogLine {
				ev.CursorLine = startLogLine
				ev.CursorPos = 0
				ev.updateDesiredVisualCol()
			} else if ev.CursorLine > endLogLine {
				ev.CursorLine = endLogLine
				ev.CursorPos = 0
				ev.updateDesiredVisualCol()
			}
		}
		vtui.FrameManager.Redraw()
	}
	ev.menuBar = vtui.NewMenuBar(nil)

	ev.topBar = NewTopBar(
		func() string {
			base := ""
			if ev.DisplayTitle != "" {
				base = ev.DisplayTitle
			} else if ev.vfs != nil {
				base = ev.vfs.Base(ev.filePath)
			} else {
				base = filepath.Base(ev.filePath)
			}
			return " " + base
		},
		func() string {
			cpName := vfs.DisplayCodepageName(ev.Codepage)
			if ev.DecodeMode {
				absPos := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
				modeBits := ev.DisasmMode
				if modeBits == 0 {
					modeBits = 64
				}
				return fmt.Sprintf(" %s │ Dec:%d │ 0x%08X     ", cpName, modeBits, absPos)
			} else if ev.HexMode {
				absPos := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
				return fmt.Sprintf(" %s │ Hex │ 0x%08X     ", cpName, absPos)
			}
			return fmt.Sprintf(" %s │ %d,%d     ", cpName, ev.CursorLine+1, ev.CursorPos)
		},
	)
	ev.topBar.ColorIdx = ColEditorStatus
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
	ev.invalidateDocumentMapping()
	ev.cancelHighlighting()

	ev.pt = piecetable.New([]byte(text))
	ev.li.Rebuild(ev.pt)
	ev.CursorLine = 0
	ev.CursorPos = 0
	ev.engine.SetPointers(ev.pt, ev.li)
	ev.semanticExtentKnown = true
	ev.modified = true
}

func (ev *EditorView) clearCaches() {
	ev.invalidateDocumentMapping()
	ev.cancelHighlighting()
	ev.engine.InvalidateCache()
	// Undo, redo and a reload replace the text wholesale. Colorer caches
	// colours by line number and has no way to notice that on its own.
	if ch, ok := ev.highlighter.(*ColorerHighlighter); ok {
		ch.DropFrom(0)
	}
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
	ev.modified = ev.unsavedBaseline || !ev.pt.GetState().Equals(ev.cleanState)
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
	ev.modified = ev.unsavedBaseline || !ev.pt.GetState().Equals(ev.cleanState)
	ev.lastOp = opNone
	ev.ensureCursorVisible()
	vtui.DebugLog("EDITOR: Executed Redo, remaining: %d, modified: %v", len(ev.redoStack), ev.modified)
}
func (ev *EditorView) invalidateStates(fromLine int) {
	ev.cancelHighlighting()
	if fromLine < len(ev.lineStates) {
		ev.lineStates = ev.lineStates[:fromLine]
	}
	if ch, ok := ev.highlighter.(*ColorerHighlighter); ok {
		ch.DropFrom(fromLine)
	}
}

// lineTextForHighlight returns one logical line as the highlighters see it:
// the terminator included, and a long binary line cut short so no parser is
// ever handed a megabyte. Not ok means the text is not available yet — the
// piece table is still loading it — and the caller should leave the line plain
// and come back on the next frame.
func (ev *EditorView) lineTextForHighlight(idx int) (string, bool) {
	if idx < 0 || idx >= ev.li.LineCount() {
		return "", false
	}
	start := ev.li.GetLineOffset(idx)
	end := ev.pt.Size()
	if idx+1 < ev.li.LineCount() {
		end = ev.li.GetLineOffset(idx + 1)
	}
	if end-start > 64*1024 {
		end = start + 64*1024
	}
	if end <= start {
		return "", true
	}
	data, err := ev.pt.GetRange(start, end-start)
	if err != nil {
		return "", false
	}
	return string(data), true
}

// Background highlighting runs in slices on the UI thread: highlighters are
// not thread safe and the Colorer session is a pooled external resource. What
// a slice may cost is therefore a wall-clock budget, not a line count. The
// same 2500 lines are a few milliseconds of Chroma and roughly a hundred of
// Colorer, so a fixed count is either a wasted frame or a visible freeze,
// depending on which highlighter happens to be selected.
const (
	// hlSliceBudget is the longest stall one slice may put on the UI thread.
	hlSliceBudget = 4 * time.Millisecond
	// hlClockStride is how many lines pass between two clock readings inside
	// a slice. Reading the clock per line is measurable next to a fast
	// highlighter; eight lines keep the overshoot small even for a slow one.
	hlClockStride = 8
	// Share of wall-clock time, in percent, the walker may occupy.
	hlDutyIndexing = 10 // line index still building: it owns the machine
	hlDutyVisible  = 50 // viewport is waiting for colours, the user sees it
	hlDutyAhead    = 25 // walking past the viewport, nobody is looking yet
	// Bounds for the gap between two slices.
	hlIdleMin = 1 * time.Millisecond
	hlIdleMax = 100 * time.Millisecond
	// A slice that highlighted nothing is waiting for the piece table to
	// load. Retry, but give up after a while and let the next render restart
	// the walker instead of polling the task queue forever.
	hlStallIdle      = 10 * time.Millisecond
	hlMaxStallSlices = 100
)

// highlightSlicePlan is what one slice did and when the next one may start.
// It crosses from the UI thread back to the walker goroutine, which is why
// every view field the decision depends on is read inside the slice itself.
type highlightSlicePlan struct {
	done     bool
	indexing bool
	lines    int
	work     time.Duration
	idle     time.Duration
}

// highlightDuty reports the share of wall-clock time the walker may spend on
// the UI thread.
//
// While the line index is still being built the walker yields to it. The
// scroll bar, Ctrl+End and any restore of a saved position all wait for the
// index, and on a 38MB log that wait is what the user actually notices;
// colours arriving a few seconds later are not.
func highlightDuty(behindViewport, indexing bool) int {
	switch {
	case indexing:
		return hlDutyIndexing
	case behindViewport:
		return hlDutyVisible
	default:
		return hlDutyAhead
	}
}

// highlightIdleGap turns the time a slice spent into the pause that keeps the
// walker at the requested duty cycle.
func highlightIdleGap(work time.Duration, duty int) time.Duration {
	if duty >= 100 {
		return 0
	}
	if duty <= 0 {
		return hlIdleMax
	}
	idle := work * time.Duration(100-duty) / time.Duration(duty)
	if idle < hlIdleMin {
		idle = hlIdleMin
	}
	if idle > hlIdleMax {
		idle = hlIdleMax
	}
	return idle
}

// usesStateChain reports whether a highlighter's state can be carried in
// ev.lineStates at all.
//
// Colorer's cannot. What it returns is the line number; the parser state lives
// inside the wasm session as a cache that only moves forward. Walking the file
// for it therefore builds nothing, and it is not free: every line handed to
// the session is copied into its line source and grows the parse cache beside
// it, and the walk leaves the session parked ahead of the viewport, so the
// next frame has to rewind it to draw. The session belongs to whoever is
// drawing. See HIGHLIGHT.md, phase 5.
func usesStateChain(h vtui.Highlighter) bool {
	if h == nil {
		return false
	}
	_, isColorer := h.(*ColorerHighlighter)
	return !isColorer
}

// highlightSlice extends the state chain for at most hlSliceBudget and
// reports what it managed. It runs on the UI thread, so the budget is exactly
// the stall the user can feel; how many lines fit into it is left to the
// highlighter.
func (ev *EditorView) highlightSlice(bgAttr uint64) highlightSlicePlan {
	plan := highlightSlicePlan{indexing: ev.indexing}

	// The highlighter can be replaced under a running walker: the Colorer
	// session finishes loading in the background and takes the place of the
	// fallback it started with.
	if !usesStateChain(ev.highlighter) {
		plan.done = true
		return plan
	}

	lineCount := ev.li.LineCount()
	cIdx := len(ev.lineStates)
	if cIdx >= lineCount {
		plan.done = !ev.indexing
		return plan
	}

	// Sampled here, on the thread that owns these fields. The previous
	// version read the viewport from the walker goroutine while the UI thread
	// was writing it, and GetLogLineAtVisualRow updates the wrap engine's
	// caches as a side effect.
	startLogLine, _ := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	behind := cIdx < startLogLine

	start := time.Now()
	for cIdx < lineCount {
		lStart := ev.li.GetLineOffset(cIdx)
		lineLen := ev.getLineLength(cIdx)
		if lineLen > 64*1024 {
			lineLen = 64 * 1024
		}

		var prevState any
		if cIdx > 0 {
			prevState = ev.lineStates[cIdx-1]
		}

		lineData, err := ev.pt.GetRange(lStart, lineLen)
		if err == piecetable.ErrLoading {
			break
		}

		_, nextState := ev.highlighter.Highlight(string(lineData), prevState, bgAttr)
		ev.lineStates = append(ev.lineStates, nextState)
		cIdx++
		plan.lines++

		if plan.lines%hlClockStride == 0 && time.Since(start) >= hlSliceBudget {
			break
		}
	}
	plan.work = time.Since(start)
	plan.done = cIdx >= ev.li.LineCount() && !ev.indexing

	if plan.lines == 0 {
		plan.idle = hlStallIdle
	} else {
		plan.idle = highlightIdleGap(plan.work, highlightDuty(behind, ev.indexing))
	}

	vStart, _ := ev.engine.GetLogLineAtVisualRow(ev.ScrollTopRow)
	vEnd := vStart + (ev.Y2 - ev.Y1) + 1
	if cIdx >= vStart && cIdx-plan.lines <= vEnd+400 {
		vtui.FrameManager.Redraw()
	}
	return plan
}

func (ev *EditorView) cancelHighlighting() {
	if ev.highlightCancel != nil {
		ev.highlightCancel()
		ev.highlightCancel = nil
	}
	ev.highlightGeneration++
	ev.highlighting = false
}

func (ev *EditorView) startHighlighting() {
	if ev.highlighter == nil || ev.highlighting {
		return
	}
	if !usesStateChain(ev.highlighter) {
		return
	}
	if len(ev.lineStates) >= ev.li.LineCount() {
		return
	}

	ev.highlighting = true
	sessionID := ev.editSession
	ev.highlightGeneration++
	generation := ev.highlightGeneration
	bgAttr := ColorerEditorBaseAttr(vtui.Palette[ColEditorText])

	ctx, cancel := context.WithCancel(context.Background())
	ev.highlightCancel = cancel

	go func() {
		defer func() {
			vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
				if generation == ev.highlightGeneration && !ev.IsDone() {
					ev.highlighting = false
				}
				return false
			})
		}()

		startedAt := time.Now()
		walked := 0
		stalls := 0
		var uiTime time.Duration

		defer func() {
			vtui.DebugLog("EDITOR: Highlight walker stopped: %d lines, %v on the UI thread, %v wall clock",
				walked, uiTime, time.Since(startedAt))
		}()

		plans := make(chan highlightSlicePlan, 1)

		for {
			if ctx.Err() != nil {
				return
			}

			vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
				plan := highlightSlicePlan{done: true}
				defer func() { plans <- plan }()

				if ctx.Err() != nil || generation != ev.highlightGeneration || ev.IsDone() {
					return false
				}
				if ev.editSession != sessionID || ev.highlighter == nil {
					cancel() // Self-terminate stale worker loop
					return false
				}
				plan = ev.highlightSlice(bgAttr)
				return false // highlightSlice explicitly redraws changed visible rows.
			})

			var plan highlightSlicePlan
			select {
			case <-ctx.Done():
				return
			case plan = <-plans:
			}

			walked += plan.lines
			uiTime += plan.work

			if plan.lines == 0 {
				stalls++
			} else {
				stalls = 0
			}

			if plan.done || (stalls >= hlMaxStallSlices && !plan.indexing) {
				return
			}
			if ctx.Err() != nil {
				return
			}

			time.Sleep(plan.idle)
		}
	}()
}
func (ev *EditorView) ensureEngineWidth() bool {
	width := max(1, ev.viewportWidth())
	changed := ev.layoutWidth != width || ev.layoutWordWrap != ev.WordWrap || ev.layoutTabSize != ev.TabSize
	if ev.reflow != nil && ev.reflow.session != ev.editSession {
		ev.reflow = nil
	}
	if changed && ev.reflow == nil && ev.layoutWidth > 0 && (ev.layoutWordWrap || ev.WordWrap) {
		ev.reflow = &editorReflow{top: ev.ScrollTopRow, session: ev.editSession}
	}
	if pending := ev.reflow; pending != nil && !pending.anchorReady {
		mapping := ev.engine.AdvanceToVisualRow(pending.top, editorMappingWorkBytes)
		if !mapping.Ready {
			ev.continueEditorMapping(mapping.Progress, mapping.Err)
			return false
		}
		pending.anchor, pending.anchorReady = mapping.Offset, true
	}
	ev.engine.SetWidth(width)
	ev.engine.ToggleWrap(ev.WordWrap)
	ev.engine.SetTabSize(ev.TabSize)
	if changed {
		ev.layoutWidth, ev.layoutWordWrap, ev.layoutTabSize = width, ev.WordWrap, ev.TabSize
		ev.invalidateDocumentMapping()
	}
	if pending := ev.reflow; pending != nil {
		mapping := ev.engine.AdvanceToOffset(pending.anchor, editorMappingWorkBytes)
		if !mapping.Ready {
			ev.continueEditorMapping(mapping.Progress, mapping.Err)
			return false
		}
		ev.ScrollTopRow = mapping.Row
		ev.reflow = nil
	}
	return true
}

func (ev *EditorView) updateDesiredVisualCol() {
	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	_, vCol := ev.engine.LogicalToVisual(curOffset)
	ev.DesiredVisualCol = vCol + ev.CursorVirtualSpaces
}
func (ev *EditorView) renderHex(scr *vtui.ScreenBuf, width, contentHeight int) {
	bgAttr := ColorerEditorBaseAttr(vtui.Palette[ColEditorText])
	offAttr := vtui.Palette[ColEditorStatus]
	currOffset := ev.HexTopOffset
	absPos := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos

	for y := 0; y < contentHeight; y++ {
		if currOffset > ev.pt.Size() {
			break
		}
		if currOffset == ev.pt.Size() && currOffset != 0 {
			if absPos != currOffset {
				break
			}
		}

		take := 16
		if currOffset+take > ev.pt.Size() {
			take = ev.pt.Size() - currOffset
		}

		var data []byte
		if take > 0 {
			var err error
			data, err = ev.pt.GetRange(currOffset, take)
			if err == piecetable.ErrLoading {
				scr.Write(ev.X1, ev.Y1+1+y, vtui.StringToCharInfo(" [ Loading... ] ", bgAttr))
				break
			}
		}

		line := fmt.Sprintf("%010X: ", currOffset)
		scr.Write(ev.X1, ev.Y1+1+y, vtui.StringToCharInfo(line, offAttr))

		// Hex part
		for i := 0; i < 16; i++ {
			cx := ev.X1 + 12 + i*3
			if i >= 8 {
				cx++
			}
			if i < len(data) {
				hexStr := fmt.Sprintf("%02X", data[i])
				scr.Write(cx, ev.Y1+1+y, vtui.StringToCharInfo(hexStr, bgAttr))
			}

			// Cursor
			if ev.IsFocused() && currOffset+i == absPos {
				scr.SetCursorPos(cx+ev.HexNibble, ev.Y1+1+y)
				scr.SetCursorVisible(true)
				scr.SetCursorShape(vtui.CursorShapeUnderline)
			}
		}

		// EOF Cursor
		if ev.IsFocused() && currOffset+len(data) == absPos && len(data) < 16 {
			cx := ev.X1 + 12 + len(data)*3
			if len(data) >= 8 {
				cx++
			}
			scr.SetCursorPos(cx+ev.HexNibble, ev.Y1+1+y)
			scr.SetCursorVisible(true)
			scr.SetCursorShape(vtui.CursorShapeUnderline)
		}

		// ASCII part
		asciiStartX := ev.X1 + 12 + 50
		scr.Write(asciiStartX-2, ev.Y1+1+y, vtui.StringToCharInfo("│ ", offAttr))
		for i := 0; i < len(data); i++ {
			r := rune(data[i])
			if r < 32 || r > 126 {
				r = '.'
			}
			cellAttr := bgAttr
			if ev.IsFocused() && currOffset+i == absPos {
				cellAttr = vtui.Palette[vtui.ColDialogEditSelected]
			}
			scr.Write(asciiStartX+i, ev.Y1+1+y, []vtui.CharInfo{{Char: uint64(r), Attributes: cellAttr}})
		}
		currOffset += 16
	}
}
func (ev *EditorView) renderDecode(scr *vtui.ScreenBuf, width, contentHeight int) {
	bgAttr := ColorerEditorBaseAttr(vtui.Palette[ColEditorText])
	offAttr := vtui.Palette[ColEditorStatus]
	currOffset := ev.HexTopOffset
	absPos := int(ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos)

	for y := 0; y < contentHeight; y++ {
		if currOffset >= ev.pt.Size() {
			break
		}
		if currOffset == ev.pt.Size() && currOffset != 0 {
			if absPos != currOffset {
				break
			}
		}

		take := 15
		if currOffset+take > ev.pt.Size() {
			take = ev.pt.Size() - currOffset
		}

		var data []byte
		if take > 0 {
			var err error
			data, err = ev.pt.GetRange(currOffset, take)
			if err == piecetable.ErrLoading {
				scr.Write(ev.X1, ev.Y1+1+y, vtui.StringToCharInfo(" [ Loading... ] ", bgAttr))
				break
			}
		}

		if len(data) == 0 && currOffset < ev.pt.Size() {
			break
		}

		if ev.DisasmMode == 0 {
			header, _ := ev.pt.GetRange(0, 1024)
			ev.DisasmMode = detectX86Mode(header)
		}

		instLen := 1
		asmStr := ""
		if len(data) > 0 {
			inst, err := x86asm.Decode(data, ev.DisasmMode)
			asmStr = fmt.Sprintf("db 0x%02X", data[0])
			if err == nil {
				instLen = inst.Len
				asmStr = x86asm.IntelSyntax(inst, uint64(currOffset), nil)
			}
		}

		line := fmt.Sprintf("%010X: ", currOffset)

		cellAttr := bgAttr
		if ev.IsFocused() && absPos >= currOffset && absPos < currOffset+instLen {
			cellAttr = vtui.Palette[vtui.ColDialogEditSelected]
		}

		scr.Write(ev.X1, ev.Y1+1+y, vtui.StringToCharInfo(line, offAttr))

		hexStr := ""
		for i := 0; i < instLen; i++ {
			if i < len(data) {
				hexStr += fmt.Sprintf("%02X", data[i])
				if i < instLen-1 {
					hexStr += " "
				}
			}
		}
		scr.Write(ev.X1+12, ev.Y1+1+y, vtui.StringToCharInfo(fmt.Sprintf("%-24s", hexStr), cellAttr))
		scr.Write(ev.X1+38, ev.Y1+1+y, vtui.StringToCharInfo(asmStr, cellAttr))
		scr.FillRect(ev.X1+38+len(asmStr), ev.Y1+1+y, ev.X2, ev.Y1+1+y, ' ', cellAttr)

		if ev.IsFocused() && absPos >= currOffset && absPos < currOffset+instLen {
			byteOffset := absPos - currOffset
			cx := ev.X1 + 12 + byteOffset*3
			scr.SetCursorPos(cx+ev.HexNibble, ev.Y1+1+y)
			scr.SetCursorVisible(true)
			scr.SetCursorShape(vtui.CursorShapeUnderline)
		}

		currOffset += instLen
	}
}

func detectX86Mode(data []byte) int {
	if len(data) >= 6 && bytes.HasPrefix(data, []byte("\x7fELF")) {
		if data[4] == 1 {
			return 32
		}
		return 64
	}
	if len(data) >= 0x40 && bytes.HasPrefix(data, []byte("MZ")) {
		peOff := int(data[0x3C]) | (int(data[0x3D]) << 8) | (int(data[0x3E]) << 16) | (int(data[0x3F]) << 24)
		if peOff > 0 && peOff+6 <= len(data) && bytes.Equal(data[peOff:peOff+4], []byte("PE\x00\x00")) {
			machine := uint16(data[peOff+4]) | (uint16(data[peOff+5]) << 8)
			if machine == 0x014C { // IMAGE_FILE_MACHINE_I386
				return 32
			}
			if machine == 0x8664 { // IMAGE_FILE_MACHINE_AMD64
				return 64
			}
		}
	}
	return 64 // Default
}
func hexCharToByte(c rune) byte {
	if c >= '0' && c <= '9' {
		return byte(c - '0')
	}
	if c >= 'a' && c <= 'f' {
		return byte(c - 'a' + 10)
	}
	if c >= 'A' && c <= 'F' {
		return byte(c - 'A' + 10)
	}
	return 0
}

func (ev *EditorView) processKeyHex(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	absPos := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	size := ev.pt.Size()

	syncCursor := func() {
		if absPos < 0 {
			absPos = 0
		}
		if absPos > size {
			absPos = size
		}
		ev.CursorLine = ev.li.GetLineAtOffset(absPos)
		ev.CursorPos = absPos - ev.li.GetLineOffset(ev.CursorLine)
		ev.ensureCursorVisible()
		vtui.FrameManager.Redraw()
	}

	shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if ev.HexNibble == 1 {
			ev.HexNibble = 0
		} else if absPos > 0 {
			absPos--
			ev.HexNibble = 1
		}
		syncCursor()
		return true
	case vtinput.VK_RIGHT:
		if absPos == size {
			return true
		}
		if ev.HexNibble == 0 {
			ev.HexNibble = 1
		} else if absPos < size {
			absPos++
			ev.HexNibble = 0
		}
		syncCursor()
		return true
	case vtinput.VK_UP:
		if ev.DecodeMode {
			absPos -= 1
		} else {
			absPos -= 16
		}
		if absPos < 0 {
			absPos = 0
		}
		syncCursor()
		return true
	case vtinput.VK_DOWN:
		if ev.DecodeMode {
			data, _ := ev.pt.GetRange(absPos, 15)
			if len(data) > 0 {
				if ev.DisasmMode == 0 {
					header, _ := ev.pt.GetRange(0, 1024)
					ev.DisasmMode = detectX86Mode(header)
				}
				inst, err := x86asm.Decode(data, ev.DisasmMode)
				if err == nil {
					absPos += int(inst.Len)
				} else {
					absPos += 1
				}
			}
		} else {
			absPos += 16
		}
		if absPos > size {
			absPos = size
		}
		syncCursor()
		return true
	case vtinput.VK_HOME:
		if ctrl {
			absPos = 0
		} else {
			absPos = absPos &^ 0xF
		}
		ev.HexNibble = 0
		syncCursor()
		return true
	case vtinput.VK_END:
		if ctrl {
			absPos = size
		} else {
			absPos = (absPos &^ 0xF) + 15
			if absPos > size {
				absPos = size
			}
		}
		ev.HexNibble = 0
		syncCursor()
		return true
	case vtinput.VK_PRIOR:
		height := ev.viewportHeight()
		absPos -= 16 * height
		syncCursor()
		return true
	case vtinput.VK_NEXT:
		height := ev.viewportHeight()
		absPos += 16 * height
		syncCursor()
		return true
	case vtinput.VK_DELETE:
		if !shift && !ctrl && absPos < size {
			ev.noteBufferEdit()
			ev.saveUndo(opOther)
			ev.modified = true
			ev.pt.Delete(absPos, 1)
			ev.li.UpdateAfterDelete(absPos, 1)
			ev.clearCaches()
			size--
			syncCursor()
		}
		return true
	case vtinput.VK_BACK:
		if absPos > 0 {
			ev.noteBufferEdit()
			ev.saveUndo(opOther)
			ev.modified = true
			ev.pt.Delete(absPos-1, 1)
			ev.li.UpdateAfterDelete(absPos-1, 1)
			ev.clearCaches()
			absPos--
			size--
			syncCursor()
		}
		return true
	}

	if (e.Char >= '0' && e.Char <= '9') || (e.Char >= 'a' && e.Char <= 'f') || (e.Char >= 'A' && e.Char <= 'F') {
		val := hexCharToByte(e.Char)
		ev.noteBufferEdit()
		ev.saveUndo(opTyping)
		ev.modified = true

		if absPos == size {
			b := byte(0)
			if ev.HexNibble == 0 {
				b = val << 4
			} else {
				b = val
			}
			ev.pt.Insert(absPos, []byte{b})
			ev.li.UpdateAfterInsert(absPos, []byte{b})
			size++
		} else {
			data, _ := ev.pt.GetRange(absPos, 1)
			b := byte(0)
			if len(data) > 0 {
				b = data[0]
			}
			if ev.HexNibble == 0 {
				b = (b & 0x0F) | (val << 4)
			} else {
				b = (b & 0xF0) | val
			}
			ev.pt.Delete(absPos, 1)
			ev.li.UpdateAfterDelete(absPos, 1)
			ev.pt.Insert(absPos, []byte{b})
			ev.li.UpdateAfterInsert(absPos, []byte{b})
		}
		ev.clearCaches()

		if ev.HexNibble == 0 {
			ev.HexNibble = 1
		} else {
			ev.HexNibble = 0
			absPos++
		}
		syncCursor()
		return true
	}

	return false
}

func (ev *EditorView) Show(scr *vtui.ScreenBuf) {
	ev.ScreenObject.Show(scr)
	if ev.topBar != nil {
		ev.topBar.Show(scr)
	}
	ev.DisplayObject(scr)
}

func (ev *EditorView) DisplayObject(scr *vtui.ScreenBuf) {
	if !ev.IsVisible() || ev.pasting {
		return
	}

	if !ev.ensureEngineWidth() || nativeDocumentCellPaintOwned(scr) {
		return
	}
	height := ev.Y2 - ev.Y1
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}

	bgAttr := ColorerEditorBaseAttr(vtui.Palette[ColEditorText])
	selAttr := vtui.Palette[vtui.ColDialogEditSelected]

	if ev.saving {
		scr.FillRect(ev.X1, ev.Y1+1, ev.X2, ev.Y2, ' ', bgAttr)
		scr.Write(ev.X1, ev.Y1+1, vtui.StringToCharInfo(" [ Saving... ] ", bgAttr))
		return
	}

	// A saved position deep in the file is restored only once the background
	// indexer has reached that line. Painting the top of the file until then
	// means the whole screen jumps the moment it does, which reads as a
	// flicker and undoes the point of easing the colours in. An empty area
	// costs the same wait and stays quiet.
	if ev.targetLine != -1 {
		scr.FillRect(ev.X1, ev.Y1+1, ev.X2, ev.Y2, ' ', bgAttr)
		msg := " [ Loading... ] "
		cx := ev.X1 + (width-len(msg))/2
		cy := ev.Y1 + 1 + height/2
		if cx >= ev.X1 && cy <= ev.Y2 {
			scr.Write(cx, cy, vtui.StringToCharInfo(msg, bgAttr))
		}
		return
	}

	// Calculate crosshair parameters before usage
	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	curVRow, curVCol := ev.engine.LogicalToVisual(curOffset)

	crossVRow, crossVCol := -1, -1
	var horzCrossAttr, vertCrossAttr uint64
	if showHorz, showVert, hAttr, vAttr := EditorCrossAttrs(); ev.IsFocused() {
		if showHorz {
			crossVRow = curVRow
			horzCrossAttr = hAttr
		}
		if showVert {
			crossVCol = curVCol + ev.CursorVirtualSpaces
			vertCrossAttr = vAttr
		}
	}

	// Clear the entire editor text area
	scr.FillRect(ev.X1, ev.Y1+1, ev.X2, ev.Y2, ' ', bgAttr)

	// Horizontal line
	if crossVRow != -1 {
		cy := ev.Y1 + 1 + crossVRow - ev.ScrollTopRow
		if cy >= ev.Y1+1 && cy <= ev.Y2 {
			scr.FillRect(ev.X1, cy, ev.X1+width-1, cy, ' ', horzCrossAttr)
		}
	}

	// Vertical line
	if crossVCol != -1 {
		cx := ev.X1 + crossVCol - ev.ScrollLeft
		if cx >= ev.X1 && cx < ev.X1+width {
			scr.FillRect(cx, ev.Y1+1, cx, ev.Y2, ' ', vertCrossAttr)
		}
	}

	if ev.HexMode {
		ev.renderHex(scr, width, height-1)
		if ev.scrollBar != nil && ev.pt.Size() > 0 {
			maxOffset := int(ev.pt.Size())
			contentHeight := ev.Y2 - ev.Y1
			if contentHeight > 0 {
				lastLineOffset := int((ev.pt.Size() - 1) &^ 0xF)
				maxOffset = lastLineOffset - (contentHeight-1)*16
				if maxOffset < 0 {
					maxOffset = 0
				}
			}
			ev.scrollBar.SetParams(ev.HexTopOffset, 0, maxOffset)
			ev.scrollBar.Show(scr)
		}
		return
	}

	if ev.HexMode {
		ev.renderHex(scr, width, height-1)
		if ev.scrollBar != nil && ev.pt.Size() > 0 {
			maxOffset := int(ev.pt.Size())
			contentHeight := ev.Y2 - ev.Y1
			if contentHeight > 0 {
				lastLineOffset := int((ev.pt.Size() - 1) &^ 0xF)
				maxOffset = lastLineOffset - (contentHeight-1)*16
				if maxOffset < 0 {
					maxOffset = 0
				}
			}
			ev.scrollBar.SetParams(ev.HexTopOffset, 0, maxOffset)
			ev.scrollBar.Show(scr)
		}
		return
	}

	if ev.DecodeMode {
		ev.renderDecode(scr, width, height-1)
		if ev.scrollBar != nil && ev.pt.Size() > 0 {
			ev.scrollBar.SetParams(ev.HexTopOffset, 0, ev.pt.Size())
			ev.scrollBar.Show(scr)
		}
		return
	} else if ev.HexMode {
		ev.renderHex(scr, width, height-1)
		if ev.scrollBar != nil && ev.pt.Size() > 0 {
			maxOffset := ev.pt.Size()
			contentHeight := ev.Y2 - ev.Y1
			if contentHeight > 0 {
				lastLineOffset := (ev.pt.Size() - 1) &^ 0xF
				maxOffset = lastLineOffset - (contentHeight-1)*16
				if maxOffset < 0 {
					maxOffset = 0
				}
			}
			ev.scrollBar.SetParams(ev.HexTopOffset, 0, maxOffset)
			ev.scrollBar.Show(scr)
		}
		return
	}

	scr.PushClipRect(ev.X1, ev.Y1+1, ev.X1+width-1, ev.Y2)

	style := editorTextProjectionStyle{
		background: bgAttr, selected: selAttr,
		paintStreamSelection: true,
		cursorRow:            curVRow, cursorColumn: curVCol,
		crossRow: crossVRow, crossColumn: crossVCol,
		horizontalCross: horzCrossAttr, verticalCross: vertCrossAttr,
	}
	ev.projectTextRows(ev.ScrollTopRow, width, height, style, func(row editorProjectedTextRow) {
		currY := ev.Y1 + 1 + row.visualRow - ev.ScrollTopRow
		scr.Write(ev.X1, currY, row.cells)
		if row.visualRow == curVRow {
			scr.SetCursorPos(ev.X1+curVCol+ev.CursorVirtualSpaces-ev.ScrollLeft, currY)
			scr.SetCursorVisible(true)
			if ev.overtype {
				scr.SetCursorShape(vtui.CursorShapeBlock)
			} else {
				scr.SetCursorShape(vtui.CursorShapeUnderline)
			}
		}
	})

	scr.PopClipRect()

	if ev.scrollBar != nil && ev.nativeViewportColumns == 0 {
		totalRows, complete := ev.engine.KnownVisualRows()
		if complete && totalRows > height {
			ev.scrollBar.SetParams(ev.ScrollTopRow, 0, totalRows-height)
			ev.scrollBar.Show(scr)
		}
		if !complete {
			ev.scheduleNativeVisualExtent()
		}
	}
}

// VetoActionKey reports modal input states in which the editor must see
// the key before the global hotkey dispatcher. While autocomplete is
// active, the keys it consumes (Tab/Esc) or uses to dismiss itself
// (navigation, Enter) belong to the editor's own ProcessKey.
func (ev *EditorView) VetoActionKey(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}
	if !ev.acEnabled || len(ev.acMatches) == 0 {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_TAB, vtinput.VK_ESCAPE, vtinput.VK_RETURN,
		vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT:
		return true
	}
	return false
}

// ProcessKey wraps the editor's own key handling so that a restore still
// waiting for the background index is abandoned only when the key actually did
// something. Over FISH+ the index needs seconds to reach a position saved deep
// in a file, and a key pressed inside that window used to throw the position
// away even when it moved nothing: Up at the top of a file left the user at the
// top of the file, which is precisely where they did not want to be.
type editorCursorPatchGuard struct {
	eligible                  bool
	editSession               int
	scrollTop, scrollLeft     int
	windowGeneration          uint64
	windowRequestGeneration   uint64
	layoutRevision            uint64
	extentKnown               bool
	wordWrap, overtype        bool
	modified, pasting, saving bool
	selection, rectSelection  bool
	hexMode, decodeMode       bool
	disasmMode                int
	showWhitespaces           bool
	cursorBeyondEOL           bool
	targetLine                int
	autocompleteEnabled       bool
	autocompleteMatchCount    int
}

func (ev *EditorView) editorCursorPatchGuard(e *vtinput.InputEvent) editorCursorPatchGuard {
	guard := editorCursorPatchGuard{}
	if ev == nil || e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown ||
		ev.pt == nil || ev.li == nil || ev.engine == nil {
		return guard
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT:
	default:
		return guard
	}
	if e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
		return guard
	}
	return ev.editorCursorStateGuard()
}

// editorCursorStateGuard proves that an input can publish only caret/status
// state. Regular stream selection is a bounded overlay in the native client,
// so its anchor/focus can travel with the caret scalars. Rectangular selection
// still belongs to the styled row projection.
func (ev *EditorView) editorCursorStateGuard() editorCursorPatchGuard {
	guard := editorCursorPatchGuard{}
	if ev == nil || ev.pt == nil || ev.li == nil || ev.engine == nil {
		return guard
	}
	showHorzCross, showVertCross, _, _ := EditorCrossAttrs()
	if showHorzCross || showVertCross || ev.rectSelActive ||
		len(ev.acMatches) != 0 || ev.pasting || ev.saving || ev.targetLine != -1 ||
		ev.HexMode || ev.DecodeMode || ev.DisasmMode != 0 {
		return guard
	}
	guard = editorCursorPatchGuard{
		eligible:                true,
		editSession:             ev.editSession,
		scrollTop:               ev.ScrollTopRow,
		scrollLeft:              ev.ScrollLeft,
		windowGeneration:        ev.semanticWindowGeneration,
		windowRequestGeneration: ev.semanticWindowRequestGeneration,
		layoutRevision:          ev.semanticLayoutRevision,
		extentKnown:             ev.semanticExtentKnown,
		wordWrap:                ev.WordWrap,
		overtype:                ev.overtype,
		modified:                ev.modified,
		pasting:                 ev.pasting,
		saving:                  ev.saving,
		selection:               ev.selActive,
		rectSelection:           ev.rectSelActive,
		hexMode:                 ev.HexMode,
		decodeMode:              ev.DecodeMode,
		disasmMode:              ev.DisasmMode,
		showWhitespaces:         ev.ShowWhitespaces,
		cursorBeyondEOL:         ev.CursorBeyondEOL,
		targetLine:              ev.targetLine,
		autocompleteEnabled:     ev.acEnabled,
		autocompleteMatchCount:  len(ev.acMatches),
	}
	return guard
}

func (guard editorCursorPatchGuard) canPublish(ev *EditorView, handled bool) bool {
	if !guard.eligible || !handled || ev == nil {
		return false
	}
	showHorzCross, showVertCross, _, _ := EditorCrossAttrs()
	return !showHorzCross && !showVertCross &&
		guard.editSession == ev.editSession &&
		guard.scrollTop == ev.ScrollTopRow && guard.scrollLeft == ev.ScrollLeft &&
		guard.windowGeneration == ev.semanticWindowGeneration &&
		guard.windowRequestGeneration == ev.semanticWindowRequestGeneration &&
		guard.layoutRevision == ev.semanticLayoutRevision &&
		guard.extentKnown == ev.semanticExtentKnown &&
		guard.wordWrap == ev.WordWrap && guard.overtype == ev.overtype &&
		guard.modified == ev.modified && guard.pasting == ev.pasting &&
		guard.saving == ev.saving && guard.rectSelection == ev.rectSelActive &&
		!ev.rectSelActive && guard.hexMode == ev.HexMode &&
		guard.decodeMode == ev.DecodeMode && guard.disasmMode == ev.DisasmMode &&
		guard.showWhitespaces == ev.ShowWhitespaces &&
		guard.cursorBeyondEOL == ev.CursorBeyondEOL &&
		guard.targetLine == ev.targetLine && ev.targetLine == -1 &&
		guard.autocompleteEnabled == ev.acEnabled &&
		guard.autocompleteMatchCount == len(ev.acMatches) && len(ev.acMatches) == 0
}

func (ev *EditorView) ProcessKey(e *vtinput.InputEvent) bool {
	if editorNavigationKey(e) {
		ev.fenceSemanticNavigation()
	}
	// Take the publication snapshot after fencing. The fence intentionally
	// advances the window generation, and that generation belongs to this
	// keyboard move rather than making the scalar cursor patch ineligible.
	guard := ev.editorCursorPatchGuard(e)
	var handled bool
	if ev.targetLine == -1 {
		handled = ev.processKeyInner(e)
	} else {
		line, pos, edited := ev.CursorLine, ev.CursorPos, ev.edited
		handled = ev.processKeyInner(e)
		if ev.targetLine != -1 && (ev.CursorLine != line || ev.CursorPos != pos || ev.edited != edited) {
			ev.targetLine = -1 // User took control, abort target jump
			ev.ensureCursorVisible()
		}
	}
	if guard.canPublish(ev, handled) {
		ev.queueSemanticCursorState()
	}
	return handled
}

func editorNavigationKey(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT:
		return true
	case vtinput.VK_E, vtinput.VK_X:
		// Ctrl+E/Ctrl+X are the editor's alternate vertical movement keys.
		return e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	default:
		return false
	}
}

// fenceSemanticNavigation makes a direct keyboard move newer than every
// native scroll intent that could still be in flight. Qt may still have a
// replaceable destination or an acknowledgement queued; advancing the
// generation and dropping that destination prevents it from moving the
// viewport back over the caret (or the active Shift selection).
func (ev *EditorView) fenceSemanticNavigation() {
	// A normal keyboard move does not need a new window identity when there is
	// no native destination in flight. Keeping the installed generation in that
	// case lets the cursor-only fast path publish its scalar patch against the
	// scene that is already on screen. Only an outstanding/replacable native
	// request needs to be retired with a successor generation.
	nativeRequestOutstanding := ev.semanticPendingScroll ||
		ev.semanticPendingGeneration != 0 ||
		ev.semanticWindowRequestGeneration != ev.semanticWindowGeneration
	generation := max(ev.semanticWindowGeneration,
		ev.semanticWindowRequestGeneration, ev.semanticPendingGeneration)
	if nativeRequestOutstanding && generation != ^uint64(0) {
		generation++
	}
	if nativeRequestOutstanding {
		ev.semanticWindowGeneration = generation
		ev.semanticWindowRequestGeneration = generation
	}
	ev.semanticPendingScroll = false
	ev.semanticPendingTop = ev.ScrollTopRow
	ev.semanticPendingGeneration = 0
	ev.semanticPointerActive = false
}

func (ev *EditorView) processKeyInner(e *vtinput.InputEvent) bool {

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
				ev.noteBufferEdit()

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

	if ev.HexMode || ev.DecodeMode {
		if ev.processKeyHex(e) {
			return true
		}
		// Prevent fallthrough to text editing keys
		switch e.VirtualKeyCode {
		case vtinput.VK_F1, vtinput.VK_F2, vtinput.VK_F3, vtinput.VK_F4, vtinput.VK_F5, vtinput.VK_F6, vtinput.VK_F7, vtinput.VK_F8, vtinput.VK_F9, vtinput.VK_F10, vtinput.VK_F11, vtinput.VK_F12:
			return false // Let global hotkeys handle it
		}
		if MacroMgr.LookupHotkey(e) {
			return true
		}
		return true // Consume all other keys in Hex mode so they don't insert text
	}

	if ev.HexMode {
		if ev.processKeyHex(e) {
			return true
		}
		// Prevent fallthrough to text editing keys
		switch e.VirtualKeyCode {
		case vtinput.VK_F1, vtinput.VK_F2, vtinput.VK_F3, vtinput.VK_F4, vtinput.VK_F5, vtinput.VK_F6, vtinput.VK_F7, vtinput.VK_F8, vtinput.VK_F9, vtinput.VK_F10, vtinput.VK_F11, vtinput.VK_F12:
			return false // Let global hotkeys handle it
		}
		if MacroMgr.LookupHotkey(e) {
			return true
		}
		return true // Consume all other keys in Hex mode so they don't insert text
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
				if tail == "" {
					return true
				}

				ev.noteBufferEdit()
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
		if alt {
			ev.selActive = false
			if !ev.rectSelActive {
				ev.rectSelActive = true
				ev.rectSelStartLine = ev.CursorLine
				ev.rectSelStartCol = ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
			}
		} else if shift {
			ev.rectSelActive = false
			if !ev.selActive {
				ev.selActive = true
				ev.selAnchorOffset = ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			}
		} else {
			ev.selActive = false
			ev.rectSelActive = false
		}
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_E:
		if e.VirtualKeyCode == vtinput.VK_E && !ctrl {
			break
		}
		// Ctrl+Up scrolls the text under the cursor, which keeps its row
		// on the screen (far2l editor.cpp, Editor::ScrollUp). The Ctrl+E
		// alias stays a plain cursor movement.
		if ctrl && !shift && !alt && e.VirtualKeyCode == vtinput.VK_UP {
			ev.scrollViewBy(-1)
			return true
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
			// Ctrl+X is delegated to the Cut action when text is
			// selected; without a selection it falls through and acts
			// as the classic Ctrl+E/Ctrl+X down-movement alias.
			if ev.selActive || ev.rectSelActive {
				RunAction("Editor.Cut")
				return true
			}
		}
		// Ctrl+Down scrolls the text under the cursor, which keeps its row
		// on the screen (far2l editor.cpp, Editor::ScrollDown). The Ctrl+X
		// alias stays a plain cursor movement.
		if ctrl && !shift && !alt && e.VirtualKeyCode == vtinput.VK_DOWN {
			ev.scrollViewBy(1)
			return true
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
		height := ev.viewportHeight()
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
		height := ev.viewportHeight()
		curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		vRow, _ := ev.engine.LogicalToVisual(curOffset)
		newVRow := vRow + height
		ev.engine.GetLogLineAtVisualRow(newVRow)
		totalVRows, complete := ev.engine.KnownVisualRows()
		if complete && newVRow >= totalVRows {
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
		// Jump by word only if it's the real Left arrow + Ctrl
		wordJump := ctrl && !isAlias
		if ev.CursorVirtualSpaces > 0 {
			if !wordJump {
				ev.CursorVirtualSpaces--
				ev.updateDesiredVisualCol()
				ev.ensureCursorVisible()
				return true
			}
			// Past EOL a word jump behaves as if the cursor stood at the
			// real end of the line: the virtual spaces are dropped first
			// and the jump then starts from the last character.
			ev.CursorVirtualSpaces = 0
		}

		if wordJump {
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
							if stopBeforeRuneLeft(prev, curr, shift) {
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
							if stopBeforeRuneRight(prev, curr, shift) {
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
		// A vertical block is a selection too. It lives in rectSelActive
		// rather than selActive, and checking only the latter is what made
		// Del eat the character under the cursor while a block was up.
		if ev.selActive || ev.rectSelActive {
			ev.DeleteSelection()
		} else {
			offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			if offset > 0 {
				ev.noteBufferEdit()
				ev.saveUndo(opOther)
				ev.modified = true
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
		// Shift+Del is Cut, and Cut it stays even if the hotkey dispatcher
		// never resolved it: far2l copies the block to the clipboard and only
		// then deletes it, and does nothing at all when no block is marked
		// (editor.cpp, KEY_SHIFTDEL). Falling through to the plain delete below
		// destroys a selection with no clipboard copy — unrecoverable — so the
		// modified key is answered here rather than left to key naming, which
		// differs between input backends. Ctrl+Del is likewise handled instead
		// of eating the character under the cursor.
		if shift && !ctrl && !alt {
			if ev.selActive || ev.rectSelActive {
				ev.CopySelection()
				ev.DeleteSelection()
			}
			ev.updateDesiredVisualCol()
			ev.ensureCursorVisible()
			return true
		}
		if ctrl && !shift && !alt {
			ev.deleteSpacersForward()
			ev.updateDesiredVisualCol()
			ev.ensureCursorVisible()
			return true
		}
		if ev.selActive || ev.rectSelActive {
			ev.DeleteSelection()
		} else {
			offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
			if offset < ev.pt.Size() {
				ev.noteBufferEdit()
				ev.saveUndo(opOther)
				ev.modified = true
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
		ev.noteBufferEdit()
		ev.saveUndo(opOther)
		if ev.selActive || ev.rectSelActive {
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
			ev.noteBufferEdit()
			ev.saveUndo(opTyping)
			if ev.selActive || ev.rectSelActive {
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
		ev.noteBufferEdit()
		ev.saveUndo(opTyping)
		if ev.selActive || ev.rectSelActive {
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

	// Injected-event fallback: KeyBar mouse clicks reach ProcessKey via
	// InjectEvents, which skips FrameManager.EventFilter and therefore the
	// hotkey manager. Route them through the same lookup so clicking F2/F7/
	// F10/… on the bottom bar triggers the configured Editor action.
	if MacroMgr.LookupHotkey(e) {
		return true
	}

	return false
}

func (ev *EditorView) fillCells(target []vtui.CharInfo, data []byte, defaultAttr, selAttr uint64, offset int, selActive bool, selMin, selMax int, syntax []uint64, startVisualCol int, isCrossRow bool, crossVCol int, horzCrossAttr, vertCrossAttr uint64, visualRow int) []vtui.CharInfo {
	return ev.fillCellsSpan(target, data, defaultAttr, selAttr, offset, selActive, selMin, selMax, syntax, startVisualCol, isCrossRow, crossVCol, horzCrossAttr, vertCrossAttr, visualRow, 0, 0, int(^uint(0)>>1))
}

// fillCellsSpan is the shared console/native row projection. It scans source
// only up to the right edge and allocates cells only for the visible span.
// tabOrigin belongs to the logical line; crosshair/selection columns belong to
// the visual fragment. Keeping them distinct prevents wrapped tabs from moving.
func (ev *EditorView) fillCellsSpan(target []vtui.CharInfo, data []byte, defaultAttr, selAttr uint64, offset int, selActive bool, selMin, selMax int, syntax []uint64, startVisualCol int, isCrossRow bool, crossVCol int, horzCrossAttr, vertCrossAttr uint64, visualRow, tabOrigin, clipLeft, clipRight int) []vtui.CharInfo {
	target = target[:0]
	currByte := 0
	charIdx := 0
	visualCol := startVisualCol
	tabSize := ev.TabSize
	if tabSize <= 0 {
		tabSize = 8
	}

	for len(data) > 0 && visualCol < clipRight {
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		displayRune, w := vtui.SanitizeRune(r)
		if r == '\t' {
			w = tabSize - ((tabOrigin + visualCol) % tabSize)
			displayRune = ' '
			if ev.ShowWhitespaces {
				displayRune = '→'
			}
		} else if r == ' ' && ev.ShowWhitespaces {
			displayRune = '·'
		} else if r < 0x20 || r == 0x7F {
			w = 1
			if !ev.ShowWhitespaces {
				displayRune = ' '
			}
		}
		if w <= 0 {
			w = 1
		}
		if visualCol+w <= clipLeft {
			charIdx++
			currByte += size
			visualCol += w
			continue
		}

		attr := defaultAttr
		if charIdx < len(syntax) {
			attr = syntax[charIdx]
		}

		// Horizontal crosshair line applies to the entire character in the active row
		if isCrossRow && horzCrossAttr != 0 {
			if horzCrossAttr&vtui.IsBgRGB != 0 {
				attr = vtui.SetRGBBack(attr, vtui.GetRGBBack(horzCrossAttr))
			} else {
				attr = vtui.SetIndexBack(attr, vtui.GetIndexBack(horzCrossAttr))
			}
		}

		if ev.rectSelActive {
			minY, maxY := ev.rectSelStartLine, ev.CursorLine
			if minY > maxY {
				minY, maxY = maxY, minY
			}
			minX, maxX := ev.rectSelStartCol, ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
			if minX > maxX {
				minX, maxX = maxX, minX
			}
			if visualRow >= minY && visualRow <= maxY && visualCol >= minX && visualCol < maxX {
				attr = selAttr
			}
		} else if selActive {
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
				if !isCrossRow && (visualCol+j == crossVCol) && vertCrossAttr != 0 {
					if vertCrossAttr&vtui.IsBgRGB != 0 {
						cellAttr = vtui.SetRGBBack(cellAttr, vtui.GetRGBBack(vertCrossAttr))
					} else {
						cellAttr = vtui.SetIndexBack(cellAttr, vtui.GetIndexBack(vertCrossAttr))
					}
				}
				if visualCol+j >= clipLeft && visualCol+j < clipRight {
					target = append(target, vtui.CharInfo{Char: charVal, Attributes: cellAttr})
				}
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

// syncVirtualSpaces re-derives CursorVirtualSpaces after a vertical move:
// when the cursor lands on a shorter line and "cursor beyond EOL" is on, the
// desired visual column is kept alive by virtual spaces past the line end.
func (ev *EditorView) syncVirtualSpaces() {
	lineLen := ev.getLineLength(ev.CursorLine)
	if ev.CursorPos == lineLen && ev.CursorBeyondEOL {
		_, endVCol := ev.engine.LogicalToVisual(ev.li.GetLineOffset(ev.CursorLine) + lineLen)
		if ev.DesiredVisualCol > endVCol {
			ev.CursorVirtualSpaces = ev.DesiredVisualCol - endVCol
			return
		}
	}
	ev.CursorVirtualSpaces = 0
}

// scrollViewBy scrolls the text by delta visual rows under a cursor that
// keeps its row on the screen: far2l's Editor::ScrollUp/ScrollDown move
// TopScreen and CurLine together, preserving the cell column. Only when the
// view cannot scroll any further — the top of the file is already shown, or
// the last screenful is — do they fall back to a bare Up()/Down() and let the
// cursor walk on alone. Nothing happens once the cursor sits on the first or
// last row. The selection is left untouched: scrolling is not a navigation.
func (ev *EditorView) scrollViewBy(delta int) {
	height := ev.viewportHeight()
	if height <= 0 {
		return
	}
	curOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	vRow, _ := ev.engine.LogicalToVisual(curOffset)
	ev.engine.GetLogLineAtVisualRow(max(vRow+delta, ev.ScrollTopRow+delta+height-1))
	totalRows, complete := ev.engine.KnownVisualRows()

	targetRow := vRow + delta
	if targetRow < 0 || targetRow >= totalRows {
		return
	}

	ev.ScrollTopRow = max(ev.ScrollTopRow+delta, 0)
	if complete {
		ev.ScrollTopRow = min(ev.ScrollTopRow, max(totalRows-height, 0))
	}

	newOffset := ev.engine.VisualToLogical(targetRow, ev.DesiredVisualCol)
	ev.CursorLine = ev.li.GetLineAtOffset(newOffset)
	ev.CursorPos = newOffset - ev.li.GetLineOffset(ev.CursorLine)
	ev.syncVirtualSpaces()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
}

func (ev *EditorView) ensureCursorVisible() {
	if ev.targetLine != -1 {
		return // Skip clamping and scrolling while waiting for the target line to be indexed
	}
	// A wrap/tab/width action may call this before the next paint. Resolve the
	// caret in the new layout, rather than scrolling with the old mapping and
	// only reflowing it later in SemanticNode/DisplayObject.
	if !ev.ensureEngineWidth() {
		return
	}

	if ev.HexMode || ev.DecodeMode {
		height := ev.viewportHeight()
		if height <= 0 {
			return
		}
		absPos := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos

		if ev.HexMode {
			row := absPos / 16
			topRow := ev.HexTopOffset / 16

			if row < topRow {
				ev.HexTopOffset = row * 16
			} else if row >= topRow+height {
				ev.HexTopOffset = (row - height + 1) * 16
			}
		} else {
			curr := ev.HexTopOffset
			visible := false
			for y := 0; y < height; y++ {
				if curr >= ev.pt.Size() {
					break
				}
				data, _ := ev.pt.GetRange(curr, 15)
				instLen := 1
				if len(data) > 0 {
					if ev.DisasmMode == 0 {
						header, _ := ev.pt.GetRange(0, 1024)
						ev.DisasmMode = detectX86Mode(header)
					}
					inst, err := x86asm.Decode(data, ev.DisasmMode)
					if err == nil {
						instLen = inst.Len
					}
				}
				if absPos >= curr && absPos < curr+instLen {
					visible = true
					break
				}
				curr += instLen
			}
			if !visible {
				ev.HexTopOffset = absPos
			}
		}
		return
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

	width := ev.viewportWidth()
	height := ev.viewportHeight()
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
		speed := AppConfig.WheelEditorDown
		vk := uint16(vtinput.VK_DOWN)
		if e.WheelDirection > 0 {
			speed = AppConfig.WheelEditorUp
			vk = vtinput.VK_UP
		}
		for i := 0; i < wheelScrollLines(speed); i++ {
			ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed {
		mx, my := int(e.MouseX), int(e.MouseY)
		if mx >= ev.X1 && mx <= ev.X2 && my >= ev.Y1+1 && my <= ev.Y2 {
			visualCol := mx - ev.X1 + ev.ScrollLeft
			visualRow := my - (ev.Y1 + 1) + ev.ScrollTopRow
			offset := ev.engine.VisualToLogical(visualRow, visualCol)

			if e.MouseEventFlags&vtinput.DoubleClick != 0 {
				ev.CursorLine = ev.li.GetLineAtOffset(offset)
				ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
				ev.selectWordUnderCursor()
			} else {
				if !ev.selActive || e.MouseEventFlags&vtinput.MouseMoved == 0 {
					ev.selActive = false
					ev.rectSelActive = false
					ev.CursorLine = ev.li.GetLineAtOffset(offset)
					ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
					ev.updateDesiredVisualCol()

					if e.MouseEventFlags&vtinput.MouseMoved == 0 {
						ev.selActive = true
						ev.selAnchorOffset = offset
					}
				} else if ev.selActive && e.MouseEventFlags&vtinput.MouseMoved != 0 {
					ev.CursorLine = ev.li.GetLineAtOffset(offset)
					ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
					ev.updateDesiredVisualCol()
				}
			}
			// A hit on a shorter line must not reset a horizontally scrolled
			// viewport to that line's beginning while the pointer stays inside.
			left := ev.ScrollLeft
			ev.ensureCursorVisible()
			ev.ScrollLeft = left
			vtui.FrameManager.Redraw()
			return true
		}
	} else if e.ButtonState == vtinput.RightmostButtonPressed {
		mx, my := int(e.MouseX), int(e.MouseY)
		if mx >= ev.X1 && mx <= ev.X2 && my >= ev.Y1+1 && my <= ev.Y2 {
			visualCol := mx - ev.X1 + ev.ScrollLeft
			visualRow := my - (ev.Y1 + 1) + ev.ScrollTopRow
			offset := ev.engine.VisualToLogical(visualRow, visualCol)

			if !ev.rectSelActive || e.MouseEventFlags&vtinput.MouseMoved == 0 {
				ev.selActive = false
				ev.rectSelActive = false
				ev.CursorLine = ev.li.GetLineAtOffset(offset)
				ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
				ev.updateDesiredVisualCol()

				if e.MouseEventFlags&vtinput.MouseMoved == 0 {
					ev.rectSelActive = true
					ev.rectSelStartLine = ev.CursorLine
					ev.rectSelStartCol = visualCol
				}
			} else if ev.rectSelActive && e.MouseEventFlags&vtinput.MouseMoved != 0 {
				ev.CursorLine = ev.li.GetLineAtOffset(offset)
				ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
				ev.updateDesiredVisualCol()
			}
			left := ev.ScrollLeft
			ev.ensureCursorVisible()
			ev.ScrollLeft = left
			vtui.FrameManager.Redraw()
			return true
		}
	} else if e.ButtonState == 0 {
		if ev.selActive && ev.selAnchorOffset == ev.li.GetLineOffset(ev.CursorLine)+ev.CursorPos {
			ev.selActive = false
			vtui.FrameManager.Redraw()
		}
		if ev.rectSelActive && ev.rectSelStartLine == ev.CursorLine && ev.rectSelStartCol == ev.getVisualColOf(ev.CursorLine, ev.CursorPos) {
			ev.rectSelActive = false
			vtui.FrameManager.Redraw()
		}
	}

	return false
}

// processDocumentPointer consumes source-addressed coordinates from the native
// document surface. scrollLeft belongs to the displayed window, not to a newer
// server-side scroll request that may still be in flight. Its result describes
// presentation changes, not pointer capture: a stale release always clears
// capture but does not itself require another document projection.
func (ev *EditorView) processDocumentPointer(e *vtinput.InputEvent, fragmentOffset int64, column, scrollLeft int, layoutRevision uint64) (changed bool) {
	if e == nil || e.Type != vtinput.MouseEventType {
		return false
	}
	before := ev.documentPointerPresentation()
	defer func() {
		changed = before != ev.documentPointerPresentation()
		if changed {
			vtui.FrameManager.Redraw()
		}
	}()
	if e.ButtonState == 0 {
		// Always release capture, even if geometry changed during the drag.
		ev.semanticPointerActive = false
		if ev.selActive && ev.selAnchorOffset == ev.li.GetLineOffset(ev.CursorLine)+ev.CursorPos {
			ev.selActive = false
		}
		if ev.rectSelActive && ev.rectSelStartLine == ev.CursorLine && ev.rectSelStartCol == ev.getVisualColOf(ev.CursorLine, ev.CursorPos) {
			ev.rectSelActive = false
		}
		return false
	}
	ev.ensureEngineWidth()
	if layoutRevision == 0 || layoutRevision != ev.semanticLayoutRevision || fragmentOffset < 0 || fragmentOffset > int64(ev.pt.Size()) {
		ev.semanticPointerActive = false
		return false
	}
	moved := e.MouseEventFlags&vtinput.MouseMoved != 0
	if moved && (!ev.semanticPointerActive || ev.semanticPointerRevision != layoutRevision) {
		return false
	}
	line := ev.li.GetLineAtOffset(int(fragmentOffset))
	var fragment *textlayout.LineFragment
	fragments := ev.engine.GetFragmentsForOffset(line, int(fragmentOffset))
	if !ev.WordWrap {
		fragments = ev.engine.GetProjectionFragments(line, 1, max(1, scrollLeft+column+1))
	}
	for _, candidate := range fragments {
		if candidate.ByteOffsetStart == int(fragmentOffset) && !candidate.Loading {
			copy := candidate
			fragment = &copy
			break
		}
	}
	if fragment == nil {
		return false
	}
	// A source-addressed pointer belongs to the window actually displayed by
	// Qt and supersedes an earlier destination that has not been published.
	ev.semanticPendingScroll = false
	visualCol := max(0, scrollLeft+column)
	offset := ev.engine.FragmentColumnToLogical(*fragment, visualCol)
	ev.targetLine = -1
	ev.CursorLine = ev.li.GetLineAtOffset(offset)
	ev.CursorPos = offset - ev.li.GetLineOffset(ev.CursorLine)
	ev.CursorVirtualSpaces = 0
	ev.updateDesiredVisualCol()
	if !moved {
		ev.semanticPointerActive = true
		ev.semanticPointerRevision = layoutRevision
		ev.selActive, ev.rectSelActive = false, false
		if e.ButtonState == vtinput.FromLeft1stButtonPressed {
			if e.MouseEventFlags&vtinput.DoubleClick != 0 {
				ev.selectWordUnderCursor()
			} else {
				ev.selActive = true
				ev.selAnchorOffset = offset
			}
		} else if e.ButtonState == vtinput.RightmostButtonPressed {
			ev.rectSelActive = true
			ev.rectSelStartLine, ev.rectSelStartCol = ev.CursorLine, visualCol
		}
	}
	left := ev.ScrollLeft
	ev.ensureCursorVisible()
	if column >= 0 && column < ev.viewportWidth() {
		ev.ScrollLeft = left
	} else if !ev.WordWrap {
		// The pointer, not a short line's clamped cursor, drives edge scrolling.
		if column < 0 {
			ev.ScrollLeft = max(0, scrollLeft+column)
		} else {
			ev.ScrollLeft = max(0, scrollLeft+column-ev.viewportWidth()+1)
		}
	}
	return true
}

// This is a per-input value, not retained document state. Different pointer
// columns can resolve to the same source offset on short/empty lines or inside
// a tab/wide cell. Such input still updates capture/navigation bookkeeping but
// must not reconstruct and serialize the identical scene.
type editorPointerPresentation struct {
	line, pos, virtualSpaces, top, left, targetLine int
	selection, rectSelection                        bool
	anchor, rectLine, rectColumn                    int
	layoutRevision                                  uint64
}

func (ev *EditorView) documentPointerPresentation() editorPointerPresentation {
	return editorPointerPresentation{
		line: ev.CursorLine, pos: ev.CursorPos, virtualSpaces: ev.CursorVirtualSpaces,
		top: ev.ScrollTopRow, left: ev.ScrollLeft, targetLine: ev.targetLine,
		selection: ev.selActive, rectSelection: ev.rectSelActive,
		anchor: ev.selAnchorOffset, rectLine: ev.rectSelStartLine, rectColumn: ev.rectSelStartCol,
		layoutRevision: ev.semanticLayoutRevision,
	}
}

func (ev *EditorView) SetPosition(x1, y1, x2, y2 int) {
	ev.ScreenObject.SetPosition(x1, y1, x2, y2)
	if ev.topBar != nil {
		ev.topBar.SetPosition(x1, y1, x2, y1)
	}
	if ev.menuBar != nil {
		ev.menuBar.SetPosition(x1, y1, x2, y1)
	}
	if ev.scrollBar != nil {
		ev.scrollBar.SetPosition(x2, y1+1, x2, y2)
		ev.scrollBar.PgStep = ev.viewportHeight()
	}
	ev.ensureEngineWidth()
	ev.ensureCursorVisible()
}

// viewportHeight returns the row count used for cursor visibility and native
// semantic navigation. DisplayObject still renders from the terminal frame
// dimensions; bounded semantic styling temporarily changes those dimensions
// to render its overscan window.
func (ev *EditorView) viewportHeight() int {
	height := ev.Y2 - ev.Y1
	if ev.nativeViewportRows > 0 {
		return ev.nativeViewportRows
	}
	return height
}

func (ev *EditorView) viewportWidth() int {
	if ev.nativeViewportColumns > 0 {
		return ev.nativeViewportColumns
	}
	width := ev.X2 - ev.X1 + 1
	if ev.scrollBar != nil {
		width--
	}
	return width
}

// scheduleNativeVisualExtent finishes the document's wrap extent after the
// first viewport has been produced. Each task advances existing layout state
// only on the UI thread, yielding to priority native input between batches.
// There is no timer, speculative read-ahead, or independently cached layout.
func (ev *EditorView) scheduleNativeVisualExtent() {
	if !ev.WordWrap || ev.engine == nil || vtui.FrameManager == nil || ev.IsDone() {
		return
	}
	if _, complete := ev.engine.KnownVisualRows(); complete {
		return
	}
	revision, session := ev.semanticLayoutRevision, ev.editSession
	if ev.semanticExtentTaskPending && ev.semanticExtentTaskRevision == revision {
		return
	}
	ev.semanticExtentTaskPending = true
	ev.semanticExtentTaskRevision = revision
	vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
		if ev.semanticExtentTaskRevision != revision {
			return false
		}
		ev.semanticExtentTaskPending = false
		if ev.IsDone() || ev.editSession != session || ev.semanticLayoutRevision != revision {
			return false
		}
		progressed, complete := ev.engine.AdvanceVisualRows(64, 64*1024)
		if progressed && !complete {
			ev.scheduleNativeVisualExtent()
		}
		return complete && ev.semanticExtentKnown
	})
}

func (ev *EditorView) ResizeConsole(w, h int) {
	// Редактор в f4 занимает всё пространство до KeyBar (h-1)
	ev.SetPosition(0, vtui.FrameManager.WorkspaceTopInset(), w-1, h-2)
}

// GetMenuBar returns the editor's menu bar. Items are regenerated from
// the action registry on every call, so shortcuts and toggle states are
// always current.
func (ev *EditorView) GetMenuBar() *vtui.MenuBar {
	ev.menuBar.Items = BuildMenuBarItems("Editor")
	return ev.menuBar
}

// applyPendingTarget restores the position saved for this document once the
// line index contains the requested line. In-memory documents already have a
// complete index before StartIndexing is called, while streaming documents
// call this from the UI tasks that append index batches.
func (ev *EditorView) applyPendingTarget() bool {
	if ev.targetLine == -1 || ev.li == nil {
		return false
	}

	ev.CursorLine = ev.targetLine
	if ev.CursorLine >= ev.li.LineCount() {
		ev.CursorLine = ev.li.LineCount() - 1
	}
	if ev.CursorLine < 0 {
		ev.CursorLine = 0
	}

	targetOff := ev.li.GetLineOffset(ev.CursorLine) + ev.targetPos
	if targetOff >= 0 && ev.asyncBuf != nil {
		_, _ = ev.asyncBuf.Read(targetOff, 4096)
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
	return true
}

func (ev *EditorView) StartIndexing() {
	ev.semanticLoadError = ""
	if ev.asyncBuf == nil {
		ev.semanticExtentKnown = true
		// NewEditorView builds the complete line index synchronously for an
		// in-memory piece table. Resolve saved state here instead of leaving
		// targetLine active forever (which paints Loading and disables the
		// cursor-only semantic patch path).
		ev.applyPendingTarget()
		return
	}
	if ev.indexCancel != nil {
		ev.indexCancel()
	}

	ev.cancelHighlighting()
	ev.editSession++
	ev.semanticExtentKnown = false
	sessionID := ev.editSession
	// A saved position inside the ready prefix needs no background round trip.
	// The final indexed line may be incomplete, so leave that target pending.
	if ev.targetLine >= 0 && ev.targetLine < ev.li.LineCount()-1 {
		ev.applyPendingTarget()
	}

	ctx, cancel := context.WithCancel(context.Background())
	ev.indexCancel = cancel
	ev.indexing = true
	// Capture live UI state before starting the worker. Published batches are
	// immutable; the worker never consults mutable editor/line-layout state.
	buf, li := ev.asyncBuf, ev.li
	filesystem, path, codepage := ev.vfs, ev.filePath, ev.Codepage
	targetLine := ev.targetLine
	initialLineCount := li.LineCount()
	initialOffset := max(0, li.GetLineOffset(initialLineCount-1))

	go func() {
		startedAt := time.Now()
		vtui.DebugLog("EDITOR_INDEX: Start indexing with targetLine=%d", targetLine)
		indexed, batches := 0, 0
		var waited time.Duration

		// Runs on completion and on cancellation alike, so the flag the
		// highlight walker throttles on can never stay stuck at true.
		defer func() {
			vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
				if ev.editSession == sessionID {
					ev.indexing = false
				}
				vtui.DebugLog("EDITOR: Indexer stopped: %d lines in %v, %v of it waiting for data, %d UI batches",
					indexed, time.Since(startedAt), waited, batches)
				return false
			})
		}()

		maxSize := buf.Size()

		if indexer, ok := filesystem.(vfs.LineIndexer); ok && codepage == 65001 {
			vtui.DebugLog("EDITOR_INDEX: Using remote LineIndexer")
			var currentLine int64 = int64(initialLineCount + 1)
			const batchSize = 100000
			remoteSuccess := true
			for {
				if ctx.Err() != nil {
					return
				}
				res, err := indexer.LineIndex(ctx, path, currentLine, batchSize)
				if err != nil {
					vtui.DebugLog("EDITOR_INDEX: Remote LineIndex failed: %v, falling back to local", err)
					remoteSuccess = false
					break
				}

				if len(res.Offsets) > 0 {
					batchFinished := len(res.Offsets) < batchSize || (res.Total >= 0 && currentLine+int64(len(res.Offsets)) > res.Total)
					batchOffsets := make([]int, 0, len(res.Offsets))
					for _, off := range res.Offsets {
						batchOffsets = append(batchOffsets, int(off))
					}

					applied := make(chan struct{})
					vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
						defer close(applied)
						if ctx.Err() != nil || ev.edited || ev.editSession != sessionID {
							return false
						}
						lastLineBefore := li.LineCount() - 1
						li.AppendOffsets(batchOffsets, maxSize)
						ev.engine.InvalidateFrom(lastLineBefore)

						if ev.targetLine != -1 && (ev.targetLine < li.LineCount()-1 || batchFinished) {
							ev.applyPendingTarget()
						}

						if ev.highlighter != nil && !ev.highlighting && len(ev.lineStates) < li.LineCount() {
							ev.startHighlighting()
						}
						return true
					})
					select {
					case <-ctx.Done():
						return
					case <-applied:
					}

					indexed += len(batchOffsets)
					batches++
					currentLine += int64(len(res.Offsets))
				}

				if len(res.Offsets) < batchSize || (res.Total >= 0 && currentLine > res.Total) {
					break
				}
			}

			if remoteSuccess {
				vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
					if ctx.Err() == nil && !ev.edited && ev.editSession == sessionID {
						ev.semanticExtentKnown = true
						ev.applyPendingTarget()
						if ev.highlighter != nil && !ev.highlighting && len(ev.lineStates) < li.LineCount() {
							ev.startHighlighting()
						}
						return true
					}
					return false
				})
				return
			}
		}

		absPos := initialOffset
		chunkSize := buf.chunkSize

		pendingOffsets := make([]int, 0, 10000)
		var indexError error

		for absPos < maxSize {
			select {
			case <-ctx.Done():
				return
			default:
			}
			waitStart := time.Now()
			data, err := buf.ReadContext(ctx, absPos, chunkSize)
			waited += time.Since(waitStart)
			if len(data) == 0 {
				indexError = err
				if indexError == nil {
					indexError = io.ErrNoProgress
				}
				break
			}

			// Fast SIMD-accelerated newline scanning using bytes.IndexByte
			scanPos := 0
			for scanPos < len(data) {
				idx := bytes.IndexByte(data[scanPos:], '\n')
				if idx == -1 {
					break
				}
				pendingOffsets = append(pendingOffsets, absPos+scanPos+idx+1)
				scanPos += idx + 1
			}

			absPos += len(data)

			// Update UI in 5000-line batches, or immediately when targetLine is reached, to avoid UI thread congestion
			if len(pendingOffsets) >= 5000 || absPos >= maxSize || err != nil || (targetLine != -1 && initialLineCount+indexed+len(pendingOffsets) > targetLine) {
				currentBatch := pendingOffsets
				batchEnd := absPos
				indexed += len(currentBatch)
				batches++
				pendingOffsets = make([]int, 0, 10000)

				applied := make(chan struct{})
				vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
					defer close(applied)
					if ctx.Err() != nil || ev.edited || ev.editSession != sessionID {
						return false
					}
					// Incremental update: we only need to invalidate visual cache
					// from the line that was previously the "last" one.
					lastLineBefore := li.LineCount() - 1
					li.AppendOffsets(currentBatch, maxSize)
					// Restoration immediately asks the engine for source-to-row
					// coordinates. Drop the old unindexed-tail layout first.
					ev.engine.InvalidateFrom(lastLineBefore)

					if ev.targetLine != -1 && (ev.targetLine < li.LineCount()-1 || batchEnd >= maxSize) {
						ev.applyPendingTarget()
					}

					if ev.highlighter != nil && !ev.highlighting && len(ev.lineStates) < li.LineCount() {
						ev.startHighlighting()
					}
					return true
				})
				// One immutable batch may be pending. Do not fill the input queue
				// with obsolete index/redraw work while the user scrolls or closes.
				select {
				case <-ctx.Done():
					return
				case <-applied:
				}
			}
			if err != nil {
				indexError = err
				break
			}
		}

		vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
			if ctx.Err() == nil && !ev.edited && ev.editSession == sessionID {
				if len(pendingOffsets) > 0 {
					previousLast := li.LineCount() - 1
					li.AppendOffsets(pendingOffsets, maxSize)
					ev.engine.InvalidateFrom(previousLast)
				}
				ev.semanticExtentKnown = absPos >= maxSize
				if indexError != nil {
					ev.semanticLoadError = indexError.Error()
				}
				if ev.semanticExtentKnown || (ev.targetLine >= 0 && ev.targetLine < li.LineCount()-1) {
					ev.applyPendingTarget()
				}
				if ev.highlighter != nil && !ev.highlighting && len(ev.lineStates) < li.LineCount() {
					ev.startHighlighting()
				}
				return true
			}
			return false
		})
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
	if cmd == CmReplace {
		ev.showReplaceDialog()
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
	nextCp := vfs.GetNextFastSwitchCodepage(ev.Codepage)
	nextCpName := vfs.DisplayCodepageName(nextCp)

	fallbacks := &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			Msg("KeyBar.EditorF1"), Msg("KeyBar.EditorF2"), Msg("KeyBar.EditorF3"),
			"", Msg("KeyBar.EditorF5"), "", Msg("KeyBar.EditorF7"), nextCpName, "", Msg("KeyBar.EditorF10"),
		},
		NormalIcons: vtui.KeyBarIconNames{
			"circle-question-mark", "save", "text-wrap", "", "space", "", "search", "languages", "", "x", "", "",
		},
	}
	res := KeyBarLabelsForArea("Editor", fallbacks)
	if hm := GlobalHotkeysMgr; hm != nil {
		if hm.GetAction("Editor", "F8") == "Editor.CodepageNext" {
			res.Normal[7] = nextCpName
		}
	}
	return res
}
func (ev *EditorView) showSearchDialog() {
	dlgW, dlgH := 66, 15
	dlg := vtui.NewCenteredDialog(dlgW, dlgH, Msg("Viewer.SearchTitle"))
	dlg.ShowClose = true

	lblPrompt := vtui.NewLabel(0, 0, Msg("Search.Prompt"), nil)
	editPattern := vtui.NewEdit(0, 0, 40, LastEditorSearch)
	editPattern.SelectAll()
	lblPrompt.FocusLink = editPattern
	dlg.SetFocusedItem(editPattern)

	chkCase := vtui.NewCheckbox(0, 0, Msg("Search.CaseSensitive"), false)
	if LastEditorSearchCase {
		chkCase.State = 1
	}

	chkWholeWord := vtui.NewCheckbox(0, 0, Msg("Search.WholeWords"), false)
	if LastEditorSearchWholeWord {
		chkWholeWord.State = 1
	}

	chkReverse := vtui.NewCheckbox(0, 0, Msg("Search.Reverse"), false)
	if LastEditorSearchReverse {
		chkReverse.State = 1
	}

	chkRegexp := vtui.NewCheckbox(0, 0, Msg("Search.Regex"), false)
	if LastEditorSearchRegexp {
		chkRegexp.State = 1
	}

	chkHex := vtui.NewCheckbox(0, 0, Msg("Search.HexPattern"), false)
	if LastEditorSearchHex {
		chkHex.State = 1
	}

	btnFind := vtui.NewButton(0, 0, Msg("Search.BtnFind"))
	btnFind.IsDefault = true
	btnAll := vtui.NewButton(0, 0, Msg("Search.BtnAll"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(lblPrompt)
	dlg.AddItem(editPattern)
	dlg.AddItem(chkCase)
	dlg.AddItem(chkWholeWord)
	dlg.AddItem(chkReverse)
	dlg.AddItem(chkRegexp)
	dlg.AddItem(btnFind)
	dlg.AddItem(btnAll)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, dlgW-4, dlgH-4)
	vbox.Add(lblPrompt, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editPattern, vtui.Margins{Top: 1}, vtui.AlignFill)

	col1 := vtui.NewVBoxLayout(0, 0, (dlgW-4)/2, 5)
	col1.Add(chkCase, vtui.Margins{}, vtui.AlignLeft)
	col1.Add(chkWholeWord, vtui.Margins{Top: 1}, vtui.AlignLeft)
	col1.Add(chkReverse, vtui.Margins{Top: 1}, vtui.AlignLeft)

	col2 := vtui.NewVBoxLayout(0, 0, (dlgW-4)/2, 5)
	col2.Add(chkRegexp, vtui.Margins{}, vtui.AlignLeft)

	rowChecks := vtui.NewHBoxLayout(0, 0, dlgW-4, 5)
	rowChecks.Add(col1, vtui.Margins{}, vtui.AlignFill)
	rowChecks.Add(col2, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowChecks, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, dlgW-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnFind, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnAll, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	saveSearchParams := func() {
		LastEditorSearch = editPattern.GetText()
		LastEditorSearchCase = chkCase.State == 1
		LastEditorSearchReverse = chkReverse.State == 1
		LastEditorSearchRegexp = chkRegexp.State == 1
		LastEditorSearchWholeWord = chkWholeWord.State == 1
		SaveSession()
	}
	btnFind.OnClick = func() {
		LastEditorSearchHex = chkHex.State == 1
		saveSearchParams()
		dlg.Close()
		ev.Search(LastEditorSearch, LastEditorSearchCase, LastEditorSearchReverse, LastEditorSearchRegexp, LastEditorSearchWholeWord, false)
	}
	btnAll.OnClick = func() {
		LastEditorSearchHex = chkHex.State == 1
		saveSearchParams()
		dlg.Close()
		// FindAll doesn't support hex pattern search yet
		ev.FindAll(LastEditorSearch, LastEditorSearchCase, LastEditorSearchRegexp, LastEditorSearchWholeWord)
	}
	btnCancel.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}

// replaceAllFold is a case-insensitive ReplaceAll under Unicode simple
// case-folding (strings.EqualFold semantics). strcase locates matches in
// the original string — no lowered copy, so no offset drift — and a folded
// match can be a different byte length than old (K U+212A matches "k"),
// which is why the consumed length comes from CutPrefix per match.
func replaceAllFold(s, old, new string) string {
	if old == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))

	start := 0
	for {
		idx := strcase.Index(s[start:], old)
		if idx == -1 {
			b.WriteString(s[start:])
			break
		}
		pos := start + idx
		after, ok := strcase.CutPrefix(s[pos:], old)
		if !ok {
			// Cannot happen: Index just matched at pos. Bail rather than
			// loop forever if the two ever disagree.
			b.WriteString(s[start:])
			break
		}
		b.WriteString(s[start:pos])
		b.WriteString(new)
		start = len(s) - len(after)
	}
	return b.String()
}

func (ev *EditorView) Replace(pattern, replacement string, caseSensitive, reverse, regexp, wholeWord, all bool) {
	if pattern == "" {
		return
	}

	searchPattern := pattern
	if LastEditorSearchHex {
		var err error
		searchPattern, err = parseHexPatternToRegex(pattern)
		if err != nil {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid hex pattern:\n%v", err), []string{"&Ok"})
			return
		}
		regexp = true
		wholeWord = false
		caseSensitive = true

		repBytes, err := parseHexReplacement(replacement)
		if err != nil {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid hex replacement:\n%v", err), []string{"&Ok"})
			return
		}
		var escapedRepl strings.Builder
		for _, b := range repBytes {
			if b == '$' {
				escapedRepl.WriteString("$$")
			} else if b == '\\' {
				escapedRepl.WriteString("\\\\")
			} else {
				escapedRepl.WriteByte(b)
			}
		}
		replacement = escapedRepl.String()
	}

	// Only whole-word matching needs the regex engine (for the \b wrapping);
	// literal replace, case-sensitive or folded, is handled without it.
	var re *coregex.Regex
	if regexp || wholeWord {
		var err error
		re, err = buildSearchRegex(searchPattern, caseSensitive, regexp, wholeWord)
		if err != nil {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid regular expression:\n%v", err), []string{"&Ok"})
			return
		}
	}

	if all {
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			bytes, _ := ev.pt.Bytes()
			text := string(bytes)
			var newText string
			switch {
			case re != nil:
				newText = string(renderReplacement(re, regexp, bytes, []byte(replacement)))
			case caseSensitive:
				newText = strings.ReplaceAll(text, pattern, replacement)
			default:
				newText = replaceAllFold(text, pattern, replacement)
			}
			if newText != text {
				ctx.RunOnUI(func() {
					ev.saveUndo(opOther)
					ev.SetText(newText)
					ev.modified = true
					ev.ensureCursorVisible()
					vtui.FrameManager.Redraw()
				})
			} else {
				ctx.RunOnUI(func() {
					vtui.ShowMessage(Msg("Replace.ConfirmTitle"), Msg("Search.NotFound"), []string{Msg("vtui.Ok")})
				})
			}
		})
		return
	}

	// Interactive path, ported from Far Manager: every occurrence is
	// confirmed with a Replace / All / Skip / Cancel dialog; replacing
	// without confirmation is only reachable through [ Replace all ].
	st := &replaceLoop{
		ev:            ev,
		pattern:       pattern,
		replacement:   replacement,
		caseSensitive: caseSensitive,
		reverse:       reverse,
		regexp:        regexp,
		wholeWord:     wholeWord,
		re:            re,
		session:       ev.editSession,
	}
	// A selection that is exactly one occurrence (made deliberately by the
	// user, or left by a previous Cancel) is prompted as-is: re-searching
	// from its start could pick a wider match (regex "a+" around a
	// one-character selection) and silently replace more than was selected.
	if ev.selActive {
		selMin, selMax := ev.getSelectionRange()
		if data, err := ev.pt.Bytes(); err == nil && selMax > selMin &&
			selectionIsMatch(data[selMin:selMax], pattern, caseSensitive, re) {
			st.promptAt(data, selMin, selMax-selMin)
			return
		}
	}
	st.searchFrom = ev.searchSeedOffset(reverse, true)
	st.findNext()
}

// noteBufferEdit records that the buffer is about to change: the background
// line indexer is canceled and every editSession fence held by an in-flight
// task goes stale.
func (ev *EditorView) noteBufferEdit() {
	if !ev.edited {
		ev.edited = true
		if ev.indexCancel != nil {
			ev.indexCancel()
		}
	}
	ev.editSession++
	ev.invalidateDocumentMapping()
}

// Pending visual-row destinations and pointer capture belong to the mapping
// in which they were created, never to a later edit or layout epoch.
func (ev *EditorView) invalidateDocumentMapping() {
	ev.semanticLayoutRevision++
	ev.semanticPointerActive = false
	ev.semanticPendingScroll = false
	ev.semanticPendingGeneration = 0
}

// replaceRange replaces bytes [min, max) with repBytes as a single
// undoable edit and leaves the cursor at the end of the replacement.
func (ev *EditorView) replaceRange(min, max int, repBytes []byte) {
	ev.replaceSpans([]matchSpan{{min, max - min}}, [][]byte{repBytes})
}

// replaceSpans replaces each span (ascending, non-overlapping) with its
// rendered replacement as one undoable edit; the cursor lands at the end of
// the last replacement. Splicing per span keeps memory at the size of the
// replacements instead of the whole covered region.
func (ev *EditorView) replaceSpans(spans []matchSpan, renders [][]byte) {
	if len(spans) == 0 {
		return
	}
	ev.noteBufferEdit()
	ev.saveUndo(opOther)
	ev.modified = true

	// Splice bottom-up so the still-pending spans keep their offsets.
	for i := len(spans) - 1; i >= 0; i-- {
		s := spans[i]
		ev.pt.Delete(s.Off, s.Len)
		ev.li.UpdateAfterDelete(s.Off, s.Len)
		ev.pt.Insert(s.Off, renders[i])
		ev.li.UpdateAfterInsert(s.Off, renders[i])
	}

	// Invalidate from the edit, not from the cursor: a reverse replace
	// edits text above the current line.
	startLine := ev.li.GetLineAtOffset(spans[0].Off)
	ev.invalidateStates(startLine)
	ev.engine.InvalidateFrom(startLine)

	last := len(spans) - 1
	newCursorOff := spans[last].Off + len(renders[last])
	ev.CursorLine = ev.li.GetLineAtOffset(newCursorOff)
	ev.CursorPos = newCursorOff - ev.li.GetLineOffset(ev.CursorLine)

	ev.selActive = false
	ev.updateDesiredVisualCol()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
}

func (ev *EditorView) showReplaceDialog() {
	dlgW, dlgH := 66, 19
	dlg := vtui.NewCenteredDialog(dlgW, dlgH, Msg("Replace.Title"))
	dlg.ShowClose = true

	lblPrompt := vtui.NewLabel(0, 0, Msg("Search.Prompt"), nil)
	editPattern := vtui.NewEdit(0, 0, 40, LastEditorSearch)
	editPattern.SelectAll()
	lblPrompt.FocusLink = editPattern
	dlg.SetFocusedItem(editPattern)

	lblReplace := vtui.NewLabel(0, 0, Msg("Replace.Prompt"), nil)
	editReplace := vtui.NewEdit(0, 0, 40, LastEditorReplace)
	editReplace.SelectAll()

	chkCase := vtui.NewCheckbox(0, 0, Msg("Search.CaseSensitive"), false)
	if LastEditorSearchCase {
		chkCase.State = 1
	}

	chkWholeWord := vtui.NewCheckbox(0, 0, Msg("Search.WholeWords"), false)
	if LastEditorSearchWholeWord {
		chkWholeWord.State = 1
	}

	chkReverse := vtui.NewCheckbox(0, 0, Msg("Search.Reverse"), false)
	if LastEditorSearchReverse {
		chkReverse.State = 1
	}

	chkRegexp := vtui.NewCheckbox(0, 0, Msg("Search.Regex"), false)
	if LastEditorSearchRegexp {
		chkRegexp.State = 1
	}

	chkHex := vtui.NewCheckbox(0, 0, Msg("Search.HexPattern"), false)
	if LastEditorSearchHex {
		chkHex.State = 1
	}

	btnReplace := vtui.NewButton(0, 0, Msg("Replace.BtnReplace"))
	btnReplaceAll := vtui.NewButton(0, 0, Msg("Replace.BtnReplaceAll"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	btnReplace.IsDefault = true

	dlg.AddItem(lblPrompt)
	dlg.AddItem(editPattern)
	dlg.AddItem(lblReplace)
	dlg.AddItem(editReplace)
	dlg.AddItem(chkCase)
	dlg.AddItem(chkWholeWord)
	dlg.AddItem(chkReverse)
	dlg.AddItem(chkRegexp)
	dlg.AddItem(btnReplace)
	dlg.AddItem(btnReplaceAll)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, dlgW-4, dlgH-4)
	vbox.Add(lblPrompt, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editPattern, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(lblReplace, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editReplace, vtui.Margins{Top: 1}, vtui.AlignFill)

	col1 := vtui.NewVBoxLayout(0, 0, (dlgW-4)/2, 5)
	col1.Add(chkCase, vtui.Margins{}, vtui.AlignLeft)
	col1.Add(chkWholeWord, vtui.Margins{Top: 1}, vtui.AlignLeft)
	col1.Add(chkReverse, vtui.Margins{Top: 1}, vtui.AlignLeft)

	col2 := vtui.NewVBoxLayout(0, 0, (dlgW-4)/2, 5)
	col2.Add(chkRegexp, vtui.Margins{}, vtui.AlignLeft)

	col2.Add(chkHex, vtui.Margins{Top: 1}, vtui.AlignLeft)

	col2.Add(chkHex, vtui.Margins{Top: 1}, vtui.AlignLeft)

	rowChecks := vtui.NewHBoxLayout(0, 0, dlgW-4, 5)
	rowChecks.Add(col1, vtui.Margins{}, vtui.AlignFill)
	rowChecks.Add(col2, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowChecks, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, dlgW-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnReplace, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnReplaceAll, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	doReplace := func(all bool) {
		LastEditorSearch = editPattern.GetText()
		LastEditorReplace = editReplace.GetText()
		LastEditorSearchCase = chkCase.State == 1
		LastEditorSearchReverse = chkReverse.State == 1
		LastEditorSearchRegexp = chkRegexp.State == 1
		LastEditorSearchWholeWord = chkWholeWord.State == 1
		SaveSession()
		dlg.Close()
		LastEditorSearchHex = chkHex.State == 1
		ev.Replace(LastEditorSearch, LastEditorReplace, LastEditorSearchCase, LastEditorSearchReverse, LastEditorSearchRegexp, LastEditorSearchWholeWord, all)
	}
	btnReplace.OnClick = func() { doReplace(false) }
	btnReplaceAll.OnClick = func() { doReplace(true) }
	btnCancel.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}

func (ev *EditorView) ReloadWithCodepage(cpID int) {
	if ev.Codepage == cpID {
		return
	}

	bytes, err := ev.pt.Bytes()
	if err != nil {
		return
	}

	rawData, err := vfs.EncodeBytes(bytes, ev.Codepage)
	if err != nil {
		rawData = bytes // Fallback
	}

	decoded, err := vfs.DecodeBytes(rawData, cpID)
	if err != nil {
		return
	}

	wasModified := ev.modified
	ev.saveUndo(opOther)

	oldLine := ev.CursorLine
	oldPos := ev.CursorPos

	ev.SetText(string(decoded))

	ev.CursorLine = oldLine
	if ev.CursorLine >= ev.li.LineCount() {
		ev.CursorLine = ev.li.LineCount() - 1
	}
	if ev.CursorLine < 0 {
		ev.CursorLine = 0
	}
	ev.CursorPos = oldPos
	if ev.CursorPos > ev.getLineLength(ev.CursorLine) {
		ev.CursorPos = ev.getLineLength(ev.CursorLine)
	}

	ev.Codepage = cpID
	ev.modified = wasModified

	ev.updateDesiredVisualCol()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
}

func (ev *EditorView) ReloadWithAutoDetect() {
	if ev.file == nil {
		return
	}
	size := ev.file.Size()
	detectLen := 16 * 1024
	if int64(detectLen) > size {
		detectLen = int(size)
	}
	header := make([]byte, detectLen)
	_, _ = ev.file.ReadAt(context.Background(), header, 0)

	cpID := vfs.DetectEncoding(header, AppConfig.EditorAutodetectCodePage, AppConfig.EditorDefaultCodePage)
	ev.ReloadWithCodepage(cpID)
}

func (ev *EditorView) showCodepageDialog() {
	items, currIdx := vfs.BuildCodepageMenuItems(ev.Codepage, AppConfig.EditorAutodetectCodePage)
	menu := vtui.NewVMenu(Msg("Codepage.Title"))
	for _, item := range items {
		menu.AddItem(item)
	}

	w, h := 45, len(items)+2
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	maxH := scrH - 2
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}
	x := (scrW - w) / 2
	y := (scrH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)

	menu.OnAction = func(idx int) {
		menu.Close()
		if idx >= 0 && idx < len(menu.Items) {
			if cpID, ok := menu.Items[idx].UserData.(int); ok {
				if cpID == -1 {
					AppConfig.EditorAutodetectCodePage = !AppConfig.EditorAutodetectCodePage
					SaveConfig()
					ev.ReloadWithAutoDetect()
				} else {
					AppConfig.EditorAutodetectCodePage = false
					AppConfig.EditorDefaultCodePage = cpID
					SaveConfig()
					ev.ReloadWithCodepage(cpID)
				}
			}
		}
	}
	menu.SetSelectPos(currIdx)
	vtui.FrameManager.PushMenu(menu)
}
func (ev *EditorView) showConvertCodepageDialog() {
	items, _ := vfs.BuildCodepageMenuItems(ev.Codepage, false)
	menu := vtui.NewVMenu(Msg("Codepage.ConvertTitle"))
	realItems := 0
	for _, item := range items {
		if item.UserData == -1 {
			continue // Skip auto-detect
		}
		menu.AddItem(item)
		realItems++
	}

	w, h := 45, realItems+2
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	maxH := scrH - 2
	if maxH < 5 {
		maxH = 5
	}
	if h > maxH {
		h = maxH
	}

	x := (scrW - w) / 2
	y := (scrH - h) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	menu.SetPosition(x, y, x+w-1, y+h-1)

	menu.OnAction = func(idx int) {
		menu.Close()
		if idx >= 0 && idx < len(menu.Items) {
			if cpID, ok := menu.Items[idx].UserData.(int); ok {
				ev.Codepage = cpID
				ev.modified = true
				vtui.ShowToast(fmt.Sprintf("Will be saved as: %s", vfs.DisplayCodepageName(cpID)), 2*time.Second)
				ev.updateDesiredVisualCol()
				ev.ensureCursorVisible()
				vtui.FrameManager.Redraw()
			}
		}
	}

	selIdx := 0
	for i, item := range menu.Items {
		if item.UserData == ev.Codepage {
			selIdx = i
			break
		}
	}
	menu.SetSelectPos(selIdx)
	vtui.FrameManager.PushMenu(menu)
}

func (ev *EditorView) selectWordUnderCursor() {
	lineStart := ev.li.GetLineOffset(ev.CursorLine)
	lineRunes := ev.getLogicalLineRunes(ev.CursorLine)
	byteAcc := 0
	runeIdx := -1
	for i, r := range lineRunes {
		if byteAcc > ev.CursorPos {
			break
		}
		runeIdx = i
		byteAcc += utf8.RuneLen(r)
	}
	if runeIdx == -1 {
		runeIdx = len(lineRunes) - 1
	}

	if runeIdx >= 0 && runeIdx < len(lineRunes) {
		startIdx := runeIdx
		for startIdx > 0 && getCharCategory(lineRunes[startIdx-1]) == catWord {
			startIdx--
		}
		endIdx := runeIdx
		for endIdx < len(lineRunes) && getCharCategory(lineRunes[endIdx]) == catWord {
			endIdx++
		}

		startOff := 0
		for i := 0; i < startIdx; i++ {
			startOff += utf8.RuneLen(lineRunes[i])
		}
		endOff := startOff
		for i := startIdx; i < endIdx; i++ {
			endOff += utf8.RuneLen(lineRunes[i])
		}

		ev.selActive = true
		ev.selAnchorOffset = lineStart + startOff
		ev.CursorPos = endOff
		ev.updateDesiredVisualCol()
		ev.ensureCursorVisible()
	}
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

// nopWriteCloser stands in for the destination handle when the file has
// already been assembled by the file system itself.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

// patchPiecesFromTable describes the edited text as pieces of the file on
// disk plus the bytes that are new, which is exactly what a piece table is:
// a piece pointing at the original buffer is a range of the file, and one
// pointing at the add buffer is text the user typed.
//
// It is only meaningful while the original buffer still is the file. That
// holds for a raw UTF-8 load; with any other codepage the buffer holds
// decoded text whose offsets have nothing to do with the bytes on disk.
func patchPiecesFromTable(pt *piecetable.PieceTable) ([]vfs.PatchPiece, bool) {
	state := pt.GetState()
	pieces := make([]vfs.PatchPiece, 0, len(state.Pieces))
	logical := 0
	for _, p := range state.Pieces {
		if p.Buf == piecetable.Original {
			pieces = append(pieces, vfs.PatchPiece{Offset: int64(p.Start), Length: int64(p.Length)})
		} else {
			data, err := pt.GetRange(logical, p.Length)
			if err != nil {
				return nil, false
			}
			pieces = append(pieces, vfs.PatchPiece{
				Length: int64(len(data)),
				Data:   append([]byte(nil), data...),
			})
		}
		logical += p.Length
	}
	return pieces, true
}

// editorTempSibling returns a temporary name through the VFS path algebra
// instead of appending bytes to the serialized path. Cloud files use opaque
// canonical URIs (for example g:item:<id>), where `path + ".f4tmp"` corrupts
// the identifier rather than naming a sibling object.
func editorTempSiblingWithToken(filesystem vfs.VFS, filePath, token string) (string, error) {
	if filesystem == nil || filePath == "" {
		return "", os.ErrInvalid
	}
	if token == "" || strings.ContainsAny(token, `/\\:`) {
		return "", errors.New("invalid editor temporary-file token")
	}
	name := filesystem.Base(filePath)
	parent := filesystem.Dir(filePath)
	if name == "" || parent == "" {
		return "", errors.New("cannot resolve a sibling temporary path")
	}
	// Keep the component independent of the original basename. Appending a
	// token to a valid 240-byte filename can exceed the common 255-byte limit.
	// A unique hidden sibling plus explicit no-overwrite creation also prevents
	// concurrent editors (or a user file) from clobbering staged data.
	tempPath := filesystem.Join(parent, ".f4tmp-"+token)
	if tempPath == "" || tempPath == filePath {
		return "", errors.New("cannot create a distinct sibling temporary path")
	}
	return tempPath, nil
}

func cleanupEditorStage(filesystem vfs.VFS, tempPath string) {
	if filesystem == nil || tempPath == "" {
		return
	}
	// Cleanup is a single best-effort mutation detached from the task Cancel.
	// The VFS session lifetime still gates CloudFox operations internally.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = filesystem.Remove(cleanupCtx, tempPath)
}

func editorTempSibling(filesystem vfs.VFS, filePath string) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate editor temporary-file name: %w", err)
	}
	return editorTempSiblingWithToken(filesystem, filePath, hex.EncodeToString(random[:]))
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
	createNewTarget := ev.createNewTarget

	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		// To preserve original file ownership, permissions and xattrs (crucial for root-owned files),
		// we write directly to the original file instead of using atomic rename via temp file.
		oldAsync := ev.asyncBuf
		oldFile := ev.file
		//needsBufferRecovery := oldAsync != nil && ev.pt.GetOriginalBuffer() == oldAsync

		capabilities := ev.vfs.GetCapabilities()
		// Capture original metadata to restore it after atomic rename
		originalStat, statErr := ev.vfs.Stat(ctx.Context, ev.filePath)
		if createNewTarget {
			var destinationErr error
			switch {
			case statErr == nil:
				destinationErr = vfs.ErrDestinationExists
			case !errors.Is(statErr, os.ErrNotExist):
				destinationErr = statErr
			}
			if destinationErr != nil {
				ctx.RunOnUI(func() {
					ev.saving = false
					vtui.ShowMessage(" Error ", fmt.Sprintf("Cannot create the new file without replacing an existing target:\n%v", destinationErr), []string{"&Ok"})
				})
				return
			}
			if !capabilities.HasAtomicNoReplaceRename {
				ctx.RunOnUI(func() {
					ev.saving = false
					vtui.ShowMessage(" Error ", fmt.Sprintf("This file system cannot safely create the new file without replacing a concurrent destination:\n%v", vfs.ErrNoReplaceUnsupported), []string{"&Ok"})
				})
				return
			}
		}

		// Google Drive can update media on the existing object ID. Prefer that
		// explicit capability over generic temp+Rename: replacing the Drive item
		// would sever permissions, share links, shortcuts, comments and history.
		identityPreservingWrite := capabilities.HasIdentityPreservingWrite && !createNewTarget
		// A colon denotes an NTFS alternate stream only for a local OS VFS.
		// Treating cloud:// as an ADS used to bypass staging and overwrite remote
		// objects directly on Windows.
		useTemp := !identityPreservingWrite && (!isLocalOSVFS(ev.vfs) || !isAlternateDataStream(ev.filePath))
		tempPath := ""
		var f io.WriteCloser
		var err error
		if useTemp {
			tempPath, err = editorTempSibling(ev.vfs, ev.filePath)
		}

		// A file system that can assemble a file out of pieces of another
		// one saves the unchanged parts from ever crossing the network. It
		// needs a second path to build into, which the temp file already
		// provides, and a buffer whose offsets still describe the file on
		// disk, which is only true for a raw UTF-8 load.
		saved := false
		if err == nil {
			if patcher, ok := ev.vfs.(vfs.InPlacePatcher); ok && ev.Codepage == 65001 && !createNewTarget {
				if pieces, ok := patchPiecesFromTable(ev.pt); ok {
					perr := patcher.PatchInPlace(ctx.Context, ev.filePath, pieces)
					if perr == nil {
						saved = true
						// The original was committed directly; there is no staged
						// sibling to rename during finalization.
						useTemp = false
						tempPath = ""
					} else {
						vtui.DebugLog("EDITOR: in-place patch unavailable: %v", perr)
					}
				}
			}
		}

		if !saved && err == nil {
			if delta, isDelta := ev.vfs.(vfs.DeltaWriter); isDelta && useTemp && ev.Codepage == 65001 {
				if pieces, ok := patchPiecesFromTable(ev.pt); ok {
					perr := delta.PatchFile(vfs.WithDestinationOverwrite(ctx.Context, false), ev.filePath, tempPath, pieces)
					if perr == nil {
						saved = true
					} else {
						vtui.DebugLog("EDITOR: delta save unavailable, writing in full: %v", perr)
					}
				}
			}
		}

		if saved {
			f = nopWriteCloser{}
		} else if useTemp && err == nil {
			f, err = ev.vfs.Create(vfs.WithDestinationOverwrite(ctx.Context, false), tempPath)
		} else if err == nil {
			f, err = ev.vfs.Create(vfs.WithDestinationOverwrite(ctx.Context, !createNewTarget), ev.filePath)
		}
		if err == nil && useTemp && !saved {
			// os.Create-style APIs commonly start at 0666/umask (often 0644).
			// Tighten the stage before the first byte of potentially sensitive
			// content is written. Providers without Unix modes safely ignore this.
			stageAttrErr := ev.vfs.SetAttributes(ctx.Context, tempPath, vfs.VFSItem{UnixMode: 0o600, Uid: -1, Gid: -1})
			if stageAttrErr != nil && isLocalOSVFS(ev.vfs) {
				_ = f.Close()
				cleanupEditorStage(ev.vfs, tempPath)
				f = nil
				err = fmt.Errorf("secure editor temporary file: %w", stageAttrErr)
			}
		}

		if err != nil {
			ctx.RunOnUI(func() {
				ev.saving = false
				if useTemp {
					vtui.DebugLog("EDITOR: Failed to create temp file for saving: %v", err)
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to create temporary file:\n%v", err), []string{"&Ok"})
				} else {
					vtui.DebugLog("EDITOR: Failed to open file for direct saving: %v", err)
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file for writing:\n%v", err), []string{"&Ok"})
				}
			})
			return
		}

		var saveErr error
		if saved {
			// The file system already wrote it.
		} else if ev.Codepage == 65001 {
			curr := 0
			total := ev.pt.Size()
			for curr < total {
				if ctx.Err() != nil {
					saveErr = ctx.Err()
					break
				}
				take := 256 * 1024
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
		} else {
			bytes, errBytes := ev.pt.Bytes()
			if errBytes != nil {
				saveErr = errBytes
			} else {
				encoded, errEnc := vfs.EncodeBytes(bytes, ev.Codepage)
				if errEnc != nil {
					saveErr = errEnc
				} else {
					_, saveErr = f.Write(encoded)
				}
			}
		}
		// The last chunk of a buffering writer is only sent by Close, so a
		// save is not finished until Close has succeeded. Over a network file
		// system, dropping this error means reporting a save that lost its
		// tail.
		if cerr := f.Close(); cerr != nil && saveErr == nil {
			saveErr = cerr
		}

		if saveErr != nil {
			if useTemp && tempPath != "" {
				cleanupEditorStage(ev.vfs, tempPath)
			}
			ctx.RunOnUI(func() {
				ev.saving = false
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to save data:\n%v", saveErr), []string{"&Ok"})
			})
			return
		}

		// Success: finalize the save atomically. Keep the old read handles alive
		// through Rename and the subsequent reopen. Remote mutations do not need
		// them closed, and on an error the current piece table may still depend on
		// that lazy AsyncBuffer. Closing it early made the editor unreadable and
		// unretryable after a failed finalization.
		oldBackingClosed := false
		if useTemp {
			// Windows does not allow replacing a local file while our reader handle
			// is open. A full local save has already read every piece still referenced
			// by the piece table, so its AsyncBuffer remains a usable recovery snapshot
			// after cancellation. Remote VFS handles stay open: their Rename can have
			// an unknown/partial outcome and closing a lazy source there would strand
			// the editor precisely when recovery matters most.
			if isLocalOSVFS(ev.vfs) {
				if oldAsync != nil {
					oldAsync.Close()
				}
				if oldFile != nil {
					_ = oldFile.Close()
				}
				oldBackingClosed = true
			}
			renameCtx := vfs.WithDestinationOverwrite(ctx.Context, !createNewTarget)
			if err := ev.vfs.Rename(renameCtx, tempPath, ev.filePath); err != nil {
				// Do not remove the staged path after an uncertain/partial rename.
				// A remote provider may have committed the move and merely lost the
				// response (or failed while removing its backup). In that state the
				// temp alias can already identify the newly saved authoritative file;
				// deleting it here would turn a save error into data loss. The random
				// sibling name also makes leaving an unconfirmed stage non-destructive.
				// Definitive pre-commit errors are safe to clean once; uncertain,
				// partial and canceled operations must remain untouched.
				if !operationMustNotRetry(err) {
					cleanupEditorStage(ev.vfs, tempPath)
				}
				ctx.RunOnUI(func() {
					ev.saving = false
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to finalize save (rename failed):\n%v", err), []string{"&Ok"})
				})
				return
			}
			// Replacement can change a remote object's immutable identity (Google
			// Drive moves the new temp object into place and deletes the old ID).
			// Persist the VFS-remapped canonical path so reopen/history survives a
			// later session instead of retaining the deleted object URI.
			if canonical, absErr := ev.vfs.Abs(ev.filePath); absErr == nil && canonical != "" {
				ev.filePath = canonical
			}
		}

		// Restore original metadata (owner, group, perms, times). Remote VFSes may
		// explicitly not support attributes; local failures are a committed but
		// user-visible partial save and must not be silently ignored.
		var metadataErr error
		if statErr == nil {
			if attrErr := ev.vfs.SetAttributes(ctx.Context, ev.filePath, originalStat); attrErr != nil && isLocalOSVFS(ev.vfs) {
				metadataErr = attrErr
			}
		}

		newFile, err := ev.vfs.Open(ctx.Context, ev.filePath)
		var newPt *piecetable.PieceTable
		var newEngine *textlayout.WrapEngine
		var newBuf *AsyncBuffer

		if err == nil {
			if ev.Codepage == 65001 {
				newBuf = NewAsyncBuffer(ctx.Context, newFile)
				newPt = piecetable.NewWithBuffer(newBuf)
			} else {
				size := newFile.Size()
				fullData := make([]byte, size)
				_, _ = newFile.ReadAt(ctx.Context, fullData, 0)
				decoded, errDec := vfs.DecodeBytes(fullData, ev.Codepage)
				if errDec != nil {
					decoded = fullData
				}
				newPt = piecetable.New(decoded)
			}
			// Reuse the existing LineIndex since the logical content is identical
			newEngine = textlayout.NewWrapEngine(newPt, ev.li)
			// A confirmed replacement and a confirmed new backing make it safe to
			// release the old lazy reader. If reopen failed, retaining it is what
			// keeps the in-memory edit session usable.
			if oldAsync != nil && !oldBackingClosed {
				oldAsync.Close()
			}
			if oldFile != nil && !oldBackingClosed {
				_ = oldFile.Close()
			}
		}

		// PRELOAD CACHE TO PREVENT SCREEN FLICKER
		// This MUST be outside RunOnUI to prevent blocking the main thread for 500ms.
		if newBuf != nil {
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
		}

		ctx.RunOnUI(func() {
			ev.saving = false
			if err == nil {
				vtui.DebugLog("EDITOR: Successfully saved %s (%d bytes)", ev.filePath, ev.pt.Size())
			}
			vtui.FrameManager.Broadcast(CmFileChanged, nil)

			if err == nil {
				ev.modified = false
				ev.unsavedBaseline = false
				ev.createNewTarget = false
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
				if metadataErr != nil {
					vtui.ShowMessage(" Warning ", fmt.Sprintf("File content was saved, but original metadata could not be restored:\n%v", metadataErr), []string{"&Ok"})
				}
			} else {
				// The content mutation already committed. Keep the old backing alive,
				// mark the current logical state clean, and report only the failed
				// reopen; retrying the save mutation would be unsafe and unnecessary.
				ev.modified = false
				ev.unsavedBaseline = false
				ev.createNewTarget = false
				ev.cleanState = ev.pt.GetState()
				ev.edited = false
				vtui.ShowMessage(" Warning ", fmt.Sprintf("File content was saved, but the file could not be reopened:\n%v", err), []string{"&Ok"})
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

// findPanelsFrameAnyScreen locates the PanelsFrame the user is
// currently working in. The active screen is consulted first: with
// several workspaces open (Ctrl+N) every screen has a PanelsFrame of
// its own, and answering with whichever one happens to sit earliest
// in the slice is how hotkeys ended up hiding panels in one workspace
// while the user was looking at another (issue #424).
//
// When the active screen has none — a full-screen editor or viewer is
// added via AddScreen, so it becomes the active screen while the
// panels stay on the one before — the search walks outwards in
// most-recently-used order. SwitchScreen moves the screen it switches
// to to the end of the slice, so walking down from ActiveIdx visits
// the workspaces in the order the user last used them, and the editor
// finds the panels it was opened from rather than the oldest ones.
func findPanelsFrameAnyScreen() *PanelsFrame {
	if vtui.FrameManager == nil {
		return nil
	}
	screens := vtui.FrameManager.Screens
	if len(screens) == 0 {
		return nil
	}

	pick := func(idx int) *PanelsFrame {
		if idx < 0 || idx >= len(screens) || screens[idx] == nil {
			return nil
		}
		frames := screens[idx].Frames
		for i := len(frames) - 1; i >= 0; i-- {
			if pf, ok := frames[i].(*PanelsFrame); ok && !pf.closed {
				return pf
			}
		}
		return nil
	}

	active := vtui.FrameManager.ActiveIdx
	if pf := pick(active); pf != nil {
		return pf
	}
	for i := active - 1; i >= 0; i-- {
		if pf := pick(i); pf != nil {
			return pf
		}
	}
	for i := active + 1; i < len(screens); i++ {
		if pf := pick(i); pf != nil {
			return pf
		}
	}
	return nil
}

// leftPanelPathForEditor / rightPanelPathForEditor return the
// on-screen visually-left / visually-right file panel's path, or
// "" if the panels frame isn't available. Deliberately uses the
// same visualLeftFSP / visualRightFSP resolvers PanelsFrame uses
// for its own Ctrl+[/Ctrl+] command-line binds, so after Ctrl+U
// the editor stays in lock-step with the panel behaviour instead
// of routing by stale slot index.
func leftPanelPathForEditor() string {
	pf := findPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	if fsp := pf.visualLeftFSP(); fsp != nil {
		return fsp.vfs.GetPath()
	}
	return ""
}

func rightPanelPathForEditor() string {
	pf := findPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	if fsp := pf.visualRightFSP(); fsp != nil {
		return fsp.vfs.GetPath()
	}
	return ""
}

// activePanelNameForEditor returns the currently-selected file name
// on the active file panel, or "" if there isn't one. Not
// distinguishing "no selection" from "no panel" — both yield ""
// which the caller treats as a no-op.
func activePanelNameForEditor() string {
	pf := findPanelsFrameAnyScreen()
	if pf == nil {
		return ""
	}
	fsp := pf.getActivePanel()
	if fsp == nil {
		return ""
	}
	return fsp.GetSelectedName()
}

// insertTextAtCursor writes bytes at the cursor position, taking
// care of an active selection (deleted first), any accumulated
// virtual-space column past EOL (materialised as real spaces before
// the insert), and the bookkeeping the rest of the editor expects
// after an edit — undo checkpoint, line-index update, cache
// invalidation, cursor advance, autocomplete refresh.
func (ev *EditorView) insertTextAtCursor(data []byte) {
	if len(data) == 0 {
		return
	}
	ev.noteBufferEdit()
	ev.saveUndo(opOther)
	if ev.selActive || ev.rectSelActive {
		ev.inGroup = true
		ev.DeleteSelection()
		ev.inGroup = false
	}
	ev.modified = true
	offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
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
}

// deleteSpacersForward removes every run of spaces and tabs starting
// at the cursor, stopping at the first non-spacer byte (or EOF). No-
// op when the cursor is already on a non-spacer. Matches FAR's
// Ctrl+Del behaviour word-for-word — "spacer" is the same tokeniser
// term the issue uses.
func (ev *EditorView) deleteSpacersForward() {
	offset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	total := ev.pt.Size()
	if offset >= total {
		return
	}
	count := 0
	for offset+count < total {
		b, _ := ev.pt.GetRange(offset+count, 1)
		if len(b) == 0 || (b[0] != ' ' && b[0] != '\t') {
			break
		}
		count++
	}
	if count == 0 {
		return
	}
	ev.noteBufferEdit()
	ev.saveUndo(opOther)
	ev.modified = true
	ev.pt.Delete(offset, count)
	ev.li.UpdateAfterDelete(offset, count)
	ev.invalidateStates(ev.CursorLine)
	ev.engine.InvalidateFrom(ev.CursorLine)
	ev.ensureCursorVisible()
}

func (ev *EditorView) CopySelection() {
	if ev.rectSelActive {
		GlobalLastClipboardWasRectangular = true
		minY, maxY := ev.rectSelStartLine, ev.CursorLine
		if minY > maxY {
			minY, maxY = maxY, minY
		}
		minX, maxX := ev.rectSelStartCol, ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
		if minX > maxX {
			minX, maxX = maxX, minX
		}

		var lines []string
		for y := minY; y <= maxY; y++ {
			lineStart := ev.li.GetLineOffset(y)
			lineLen := ev.getLineLength(y)
			lineData, _ := ev.pt.GetRange(lineStart, lineLen)

			var piece []rune
			visualCol := 0
			runes := []rune(string(lineData))
			tabSize := ev.TabSize
			if tabSize <= 0 {
				tabSize = 8
			}

			for _, r := range runes {
				rw := 1
				if r == '\t' {
					rw = tabSize - (visualCol % tabSize)
				} else {
					rw = runewidth.RuneWidth(r)
					if rw <= 0 {
						rw = 1
					}
				}

				if visualCol >= minX && visualCol < maxX {
					piece = append(piece, r)
				} else if visualCol < minX && visualCol+rw > minX {
					piece = append(piece, ' ')
				}

				visualCol += rw
				if visualCol >= maxX {
					break
				}
			}

			if visualCol < maxX {
				needed := maxX - visualCol
				if visualCol < minX {
					needed = maxX - minX
				}
				piece = append(piece, []rune(strings.Repeat(" ", needed))...)
			}

			lines = append(lines, string(piece))
		}

		vtui.SetClipboard(strings.Join(lines, "\n"))
		return
	}

	min, max := ev.getSelectionRange()
	if max > min {
		GlobalLastClipboardWasRectangular = false
		data, _ := ev.pt.GetRange(min, max-min)
		if data != nil {
			vtui.SetClipboard(string(data))
			vtui.DebugLog("EDITOR: Copied %d bytes to clipboard", max-min)
		}
	}
}

func (ev *EditorView) PasteRectangular(text string, targetCol int) {
	if text == "" {
		return
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return
	}
	neededLines := ev.CursorLine + len(lines)
	mutates := neededLines > ev.li.LineCount()
	if !mutates {
		for _, line := range lines {
			if line != "" {
				mutates = true
				break
			}
		}
	}
	if !mutates {
		return
	}

	ev.noteBufferEdit()
	ev.saveUndo(opOther)
	ev.modified = true

	for ev.li.LineCount() < neededLines {
		offset := ev.pt.Size()
		ev.pt.Insert(offset, []byte("\n"))
		ev.li.UpdateAfterInsert(offset, []byte("\n"))
	}
	ev.engine.InvalidateFrom(ev.CursorLine)

	for i := len(lines) - 1; i >= 0; i-- {
		y := ev.CursorLine + i
		lineStart := ev.li.GetLineOffset(y)
		lineLen := ev.getLineLength(y)
		lineData, _ := ev.pt.GetRange(lineStart, lineLen)

		visualCol := 0
		runes := []rune(string(lineData))
		tabSize := ev.TabSize
		if tabSize <= 0 {
			tabSize = 8
		}

		insertByteOff := -1
		byteAcc := 0

		for _, r := range runes {
			rw := 1
			if r == '\t' {
				rw = tabSize - (visualCol % tabSize)
			} else {
				rw = runewidth.RuneWidth(r)
				if rw <= 0 {
					rw = 1
				}
			}

			if visualCol >= targetCol && insertByteOff == -1 {
				insertByteOff = byteAcc
				break
			}

			visualCol += rw
			byteAcc += utf8.RuneLen(r)
		}

		if insertByteOff == -1 {
			padding := strings.Repeat(" ", targetCol-visualCol)
			insertOff := lineStart + byteAcc
			ev.pt.Insert(insertOff, []byte(padding+lines[i]))
			ev.li.UpdateAfterInsert(insertOff, []byte(padding+lines[i]))
		} else {
			insertOff := lineStart + insertByteOff
			ev.pt.Insert(insertOff, []byte(lines[i]))
			ev.li.UpdateAfterInsert(insertOff, []byte(lines[i]))
		}
	}

	ev.invalidateStates(ev.CursorLine)
	ev.engine.InvalidateFrom(ev.CursorLine)
	ev.ensureCursorVisible()
}

func (ev *EditorView) PasteText(text string) {
	if GlobalLastClipboardWasRectangular {
		targetCol := ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
		ev.PasteRectangular(text, targetCol)
		return
	}
	if text == "" && !ev.selActive {
		return
	}

	ev.noteBufferEdit()
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
	if ev.rectSelActive {
		minY, maxY := ev.rectSelStartLine, ev.CursorLine
		if minY > maxY {
			minY, maxY = maxY, minY
		}
		minX, maxX := ev.rectSelStartCol, ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
		if minX > maxX {
			minX, maxX = maxX, minX
		}

		mutated := false

		for y := maxY; y >= minY; y-- {
			lineStart := ev.li.GetLineOffset(y)
			lineLen := ev.getLineLength(y)
			lineData, _ := ev.pt.GetRange(lineStart, lineLen)

			visualCol := 0
			runes := []rune(string(lineData))
			tabSize := ev.TabSize
			if tabSize <= 0 {
				tabSize = 8
			}

			startByte := -1
			endByte := -1
			byteAcc := 0

			for _, r := range runes {
				rw := 1
				if r == '\t' {
					rw = tabSize - (visualCol % tabSize)
				} else {
					rw = runewidth.RuneWidth(r)
					if rw <= 0 {
						rw = 1
					}
				}

				if visualCol >= minX && startByte == -1 {
					startByte = byteAcc
				}
				if visualCol >= maxX && endByte == -1 {
					endByte = byteAcc
				}

				visualCol += rw
				byteAcc += utf8.RuneLen(r)
			}

			if startByte != -1 {
				if endByte == -1 {
					endByte = byteAcc
				}
				if endByte > startByte {
					if !mutated {
						if !ev.inGroup {
							ev.noteBufferEdit()
						}
						ev.saveUndo(opOther)
						ev.modified = true
						mutated = true
					}
					delOff := lineStart + startByte
					delLen := endByte - startByte
					ev.pt.Delete(delOff, delLen)
					ev.li.UpdateAfterDelete(delOff, delLen)
				}
			}
		}

		if mutated {
			ev.invalidateStates(minY)
			ev.engine.InvalidateFrom(minY)
		}
		ev.rectSelActive = false
		ev.ensureCursorVisible()
		return
	}

	min, max := ev.getSelectionRange()
	if min < 0 {
		min = 0
	}
	if max > ev.pt.Size() {
		max = ev.pt.Size()
	}
	if max > min {
		if !ev.inGroup {
			ev.noteBufferEdit()
		}

		ev.saveUndo(opOther)

		ev.modified = true
		ev.pt.Delete(min, max-min)
		ev.li.UpdateAfterDelete(min, max-min)
		ev.clearCaches()
		ev.selActive = false
		ev.CursorLine = ev.li.GetLineAtOffset(min)
		ev.CursorPos = min - ev.li.GetLineOffset(ev.CursorLine)
	}
}

func (ev *EditorView) DeleteCurrentLine() {
	if ev.pt.Size() == 0 {
		return
	}
	ev.noteBufferEdit()
	ev.saveUndo(opOther)
	ev.modified = true

	lineStart := ev.li.GetLineOffset(ev.CursorLine)
	lineEnd := ev.pt.Size()

	lastLineDeleted := false

	if ev.CursorLine+1 < ev.li.LineCount() {
		lineEnd = ev.li.GetLineOffset(ev.CursorLine + 1)
		ev.pt.Delete(lineStart, lineEnd-lineStart)
		ev.li.UpdateAfterDelete(lineStart, lineEnd-lineStart)
	} else {
		lastLineDeleted = true
		if ev.CursorLine > 0 {
			prevLineLen := ev.getLineLength(ev.CursorLine - 1)
			prevLineStart := ev.li.GetLineOffset(ev.CursorLine - 1)
			deleteStart := prevLineStart + prevLineLen

			ev.pt.Delete(deleteStart, lineEnd-deleteStart)
			ev.li.UpdateAfterDelete(deleteStart, lineEnd-deleteStart)

			ev.CursorLine--
			ev.CursorPos = prevLineLen
		} else {
			ev.pt.Delete(0, lineEnd)
			ev.li.UpdateAfterDelete(0, lineEnd)
			ev.CursorPos = 0
		}
	}

	ev.invalidateStates(ev.CursorLine)
	ev.engine.InvalidateFrom(ev.CursorLine)

	if !lastLineDeleted {
		vRow := ev.engine.GetRowOffset(ev.CursorLine)
		newOffset := ev.engine.VisualToLogical(vRow, ev.DesiredVisualCol)
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
	} else {
		lineLen := ev.getLineLength(ev.CursorLine)
		if ev.CursorPos > lineLen {
			ev.CursorPos = lineLen
		}
		ev.CursorVirtualSpaces = 0
		ev.updateDesiredVisualCol()
	}

	ev.ensureCursorVisible()
}

func (ev *EditorView) GetType() vtui.FrameType { return vtui.TypeUser + 2 }
func (ev *EditorView) IsBusy() bool            { return ev.pasting || ev.saving }
func (ev *EditorView) GetTitle() string {
	if ev.DisplayTitle != "" {
		return ev.DisplayTitle
	}
	if ev.filePath != "" {
		return "Edit: " + filepath.Base(ev.filePath)
	}
	return "Editor"
}

// GetWorkspaceTabTitle provides a compact title for the workspace
// tab bar while leaving GetTitle available for contexts that need the fuller
// textual description.
func (ev *EditorView) GetWorkspaceTabTitle() string {
	if ev.DisplayTitle != "" {
		return ev.DisplayTitle
	}
	if ev.filePath != "" {
		return filepath.Base(ev.filePath)
	}
	return "Editor"
}

func (ev *EditorView) GetWorkspaceTabMarker() string { return "E" }

func (ev *EditorView) GetWorkspaceTabSurfaceKind() string { return "editor" }

// buildSearchRegex compiles an editor search pattern with the rules shared
// by Find, Find All and Replace: non-regex input is quoted literally, case
// insensitivity becomes an (?i) prefix and whole-word wraps the pattern in
// word boundaries.
func buildSearchRegex(pattern string, caseSensitive, useRegex, wholeWord bool) (*coregex.Regex, error) {
	finalPattern := pattern
	if !useRegex {
		finalPattern = coregex.QuoteMeta(pattern)
	}
	if !caseSensitive {
		finalPattern = "(?i)" + finalPattern
	}
	if wholeWord {
		finalPattern = `\b(?:` + finalPattern + `)\b`
	}
	return coregex.Compile(finalPattern)
}

// showSearchProgressDialog shows the cancelable " Searching... " popup used
// by Find and Find All while the buffer scan runs in the background.
func showSearchProgressDialog(pattern string) (dlg *vtui.Window, btnCancel *vtui.Button) {
	dlg = vtui.NewCenteredDialog(50, 8, Msg("Search.Searching"))
	lbl := vtui.NewLabel(0, 0, fmt.Sprintf("Looking for: %s", pattern), nil)
	dlg.AddItem(lbl)
	btnCancel = vtui.NewButton(0, 0, Msg("vtui.Cancel"))
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 8-4)
	vbox.Add(lbl, vtui.Margins{}, vtui.AlignCenter)
	vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	vtui.FrameManager.AddScreenHeadless(dlg)
	return dlg, btnCancel
}

// runSearchWithProgress shows the progress popup and runs worker in the
// background. The cancel wiring lives here, on the UI thread: the Cancel
// button and any other close of the dialog (Esc, F10, a border click) all
// cancel the task, so a dismissed search can never resurface its result.
// Wiring after RunAsync is race-free because queued UI tasks only run once
// the current one returns. Must be called on the UI thread; workers close
// dlg from their RunOnUI callback and must check ctx.Err() before acting.
func runSearchWithProgress(pattern string, worker func(ctx *vtui.TaskContext, dlg *vtui.Window)) (*vtui.Window, *vtui.TaskContext) {
	dlg, btnCancel := showSearchProgressDialog(pattern)
	ctx := vtui.RunAsync(func(c *vtui.TaskContext) { worker(c, dlg) })
	btnCancel.OnClick = func() { ctx.Cancel(); dlg.Close() }
	dlg.OnResult = func(int) { ctx.Cancel() }
	return dlg, ctx
}

// findMatch locates one occurrence of pattern in data: forward from
// startOff, or backward from just before it when reverse is set; next
// additionally skips a match starting exactly at startOff. The offset is
// -1 when nothing matches. The returned length is the matched byte length,
// which for regex or case-folded matches can differ from len(pattern).
func findMatch(data []byte, pattern string, caseSensitive, reverse, regexp, wholeWord, next bool, startOff int) (int, int, error) {
	foundOffset := -1
	matchLen := len(pattern)
	totalSize := len(data)

	// Only whole-word matching needs the regex engine (for the \b
	// wrapping); literal search, case-sensitive or folded, is handled
	// below without it.
	if regexp || wholeWord {
		re, err := buildSearchRegex(pattern, caseSensitive, regexp, wholeWord)
		if err != nil {
			return -1, 0, err
		}

		if !reverse {
			currOff := startOff
			if next {
				currOff++
			}
			if currOff < totalSize {
				loc := re.FindIndex(data[currOff:])
				if loc != nil {
					foundOffset = currOff + loc[0]
					matchLen = loc[1] - loc[0]
				}
			}
		} else {
			currOff := startOff
			if next {
				currOff--
			}
			if currOff > totalSize {
				currOff = totalSize
			}
			if currOff > 0 {
				// FindAllIndex scans left-to-right without overlap, so for
				// self-overlapping patterns (e.g. "яя" in "яяя") this can
				// land left of the true rightmost hit. Accepted: unlike the
				// old ToLower+LastIndex path it cannot corrupt byte offsets.
				locs := re.FindAllIndex(data[:currOff], -1)
				if len(locs) > 0 {
					last := locs[len(locs)-1]
					foundOffset = last[0]
					matchLen = last[1] - last[0]
				}
			}
		}
		return foundOffset, matchLen, nil
	}

	text := string(data)
	index, lastIndex := strings.Index, strings.LastIndex
	if !caseSensitive {
		// strcase folds while scanning the original text, so the offsets
		// it returns need no translation.
		index, lastIndex = strcase.Index, strcase.LastIndex
	}
	if !reverse {
		currOff := startOff
		if next {
			currOff++
		}
		if currOff < len(text) {
			idx := index(text[currOff:], pattern)
			if idx != -1 {
				foundOffset = currOff + idx
			}
		}
	} else {
		currOff := startOff
		if next {
			currOff--
		}
		if currOff > len(text) {
			currOff = len(text)
		}
		if currOff > 0 {
			idx := lastIndex(text[:currOff], pattern)
			if idx != -1 {
				foundOffset = idx
			}
		}
	}
	// A folded match can differ in byte length from the pattern
	// (K U+212A matches "k"), so measure what it consumed.
	if foundOffset != -1 && !caseSensitive {
		if after, ok := strcase.CutPrefix(text[foundOffset:], pattern); ok {
			matchLen = len(text) - foundOffset - len(after)
		}
	}
	return foundOffset, matchLen, nil
}

// searchSeedOffset returns the offset a buffer scan starts from. An active
// selection — typically the previously found or confirmed match — takes
// precedence over the raw cursor: a replace scan (includeSelection) starts
// at the selection's near edge so the highlighted occurrence is prompted
// again, while a plain search starts at its end and continues past it,
// exactly as if the cursor sat there.
func (ev *EditorView) searchSeedOffset(reverse, includeSelection bool) int {
	if ev.selActive {
		selMin, selMax := ev.getSelectionRange()
		if includeSelection && !reverse {
			return selMin
		}
		return selMax
	}
	return ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
}

func parseHexPatternToRegex(pattern string) (string, error) {
	var re strings.Builder
	re.WriteString("(?s)") // dot matches newline
	tokens := strings.Fields(pattern)
	for _, t := range tokens {
		if t == "?" || t == "??" {
			re.WriteString(".")
		} else {
			if len(t) == 1 {
				t = "0" + t
			}
			b, err := hex.DecodeString(t)
			if err != nil {
				return "", fmt.Errorf("invalid hex token: %s", t)
			}
			re.WriteString(fmt.Sprintf("\\x%02x", b[0]))
		}
	}
	return re.String(), nil
}

func parseHexReplacement(repl string) ([]byte, error) {
	var res []byte
	tokens := strings.Fields(repl)
	for _, t := range tokens {
		if len(t) == 1 {
			t = "0" + t
		}
		b, err := hex.DecodeString(t)
		if err != nil {
			return nil, fmt.Errorf("invalid hex token: %s", t)
		}
		res = append(res, b[0])
	}
	return res, nil
}
func (ev *EditorView) Search(pattern string, caseSensitive, reverse, regexp, wholeWord, next bool) {
	if pattern == "" {
		return
	}
	if LastEditorSearch != pattern || LastEditorSearchCase != caseSensitive || LastEditorSearchReverse != reverse || LastEditorSearchRegexp != regexp || LastEditorSearchWholeWord != wholeWord {
		LastEditorSearch = pattern
		LastEditorSearchCase = caseSensitive
		LastEditorSearchReverse = reverse
		LastEditorSearchRegexp = regexp
		LastEditorSearchWholeWord = wholeWord
		SaveSession()
	}

	searchPattern := pattern
	if LastEditorSearchHex {
		var err error
		searchPattern, err = parseHexPatternToRegex(pattern)
		if err != nil {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid hex pattern:\n%v", err), []string{"&Ok"})
			return
		}
		regexp = true
		wholeWord = false
		caseSensitive = true
	}

	vtui.DebugLog("EDITOR_SEARCH: Starting for %q (sensitive=%v, reverse=%v, regexp=%v, wholeWord=%v, next=%v)",
		pattern, caseSensitive, reverse, regexp, wholeWord, next)

	vtui.FrameManager.PostTask(func() {
		// Read on the UI thread, before the background scan starts.
		startOff := ev.searchSeedOffset(reverse, false)

		runSearchWithProgress(searchPattern, func(ctx *vtui.TaskContext, dlg *vtui.Window) {
			bytes, errBytes := ev.pt.Bytes()
			if errBytes != nil {
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", "Failed to read file buffer.", []string{"&Ok"})
				})
				return
			}

			foundOffset, matchLen, err := findMatch(bytes, pattern, caseSensitive, reverse, regexp, wholeWord, next, startOff)
			if err != nil {
				ctx.RunOnUI(func() {
					dlg.Close()
					vtui.ShowMessage(" Error ", fmt.Sprintf("Invalid regular expression:\n%v", err), []string{"&Ok"})
				})
				return
			}

			ctx.RunOnUI(func() {
				// Closing the dialog cancels the task via OnResult, so the
				// cancellation state must be read first: a search the user
				// dismissed neither jumps nor complains.
				canceled := ctx.Err() != nil
				dlg.Close()
				if canceled {
					return
				}
				if foundOffset != -1 {
					ev.selectFoundPattern(foundOffset, matchLen)
				} else {
					vtui.ShowMessage(Msg("Search.Title"), Msg("Search.NotFound"), []string{Msg("vtui.Ok")})
				}
			})
		})
	})
}

// selectFoundPattern selects a match found by Search or FindAll: the match
// becomes the active selection and the cursor lands at its end.
func (ev *EditorView) selectFoundPattern(off, length int) {
	ev.selActive = true
	ev.selAnchorOffset = off

	end := off + length
	ev.CursorLine = ev.li.GetLineAtOffset(end)
	ev.CursorPos = end - ev.li.GetLineOffset(ev.CursorLine)

	ev.updateDesiredVisualCol()
	ev.ensureCursorVisible()
	vtui.FrameManager.Redraw()
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

func isAlternateDataStream(path string) bool {
	if runtime.GOOS != "windows" {
		return false
	}
	// URI schemes contain a colon but can never denote an NTFS stream.
	if strings.Contains(path, "://") {
		return false
	}
	vol := filepath.VolumeName(path)
	rest := path
	if vol != "" {
		rest = strings.TrimPrefix(path, vol)
	}
	return strings.Contains(rest, ":")
}

func (ev *EditorView) getVisualColOf(line, pos int) int {
	offset := ev.li.GetLineOffset(line) + pos
	_, vCol := ev.engine.LogicalToVisual(offset)
	return vCol
}
