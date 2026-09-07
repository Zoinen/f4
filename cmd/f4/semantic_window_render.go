package main

import (
	"reflect"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

type semanticEditorStyledRowsContext struct {
	editSession       int
	width             int
	scrollLeft        int
	tabSize           int
	wordWrap          bool
	showWhitespaces   bool
	paletteHash       uint64
	colorerGeneration uint64
	highlighterType   reflect.Type
	highlighterID     uintptr
}

type semanticEditorRowSelectionKey struct {
	kind        uint8
	left, right int
}

type semanticEditorStyledRowKey struct {
	visualRow, logicalLine int
	offset, endOffset      int64
	text                   string
	selection              semanticEditorRowSelectionKey
}

type semanticEditorStyledRowCacheEntry struct {
	key semanticEditorStyledRowKey
	row extui.TextRowModel
}

type semanticEditorSelectionState struct {
	kind        uint8
	top, bottom int
	left, right int
}

// semanticStyledViewerWindowRows renders the complete bounded semantic
// window, rather than only the terminal-sized viewport in its middle.  The
// viewer renderers are kept as the single source of truth for tabs, wide
// characters and the differently coloured offset/hex/ascii regions.
//
// renderText and renderHex normally update navigation state as a side effect.
// A semantic snapshot must not do that: mouse momentum may ask for a new
// window while terminal input is using lineOffsets/eofVisible.  Save and
// restore those fields around the bounded off-screen render.
func semanticStyledViewerWindowRows(vv *ViewerView, window semanticSurfaceWindow, width int) []extui.TextRowModel {
	if vv == nil || width <= 0 || len(window.rows) == 0 {
		return window.rows
	}

	topOffset := vv.TopOffset
	lineOffsets := cloneInt64Slice(vv.lineOffsets)
	eofVisible := vv.eofVisible
	lastKnownSize := vv.lastKnownSize
	defer func() {
		vv.TopOffset = topOffset
		vv.lineOffsets = lineOffsets
		vv.eofVisible = eofVisible
		vv.lastKnownSize = lastKnownSize
	}()

	vv.TopOffset = window.start
	rowCount := len(window.rows)
	rendered := semanticRenderSurface(vv.X1, vv.Y1+1,
		vv.X1+width-1, vv.Y1+rowCount, func(scr *vtui.ScreenBuf) {
			background := vtui.Palette[ColViewerText]
			scr.FillRect(vv.X1, vv.Y1+1, vv.X1+width-1,
				vv.Y1+rowCount, ' ', background)
			if vv.Busy {
				scr.Write(vv.X1, vv.Y1+1,
					vtui.StringToCharInfo(" [ Loading... ] ", background))
				return
			}
			if vv.HexMode {
				vv.renderHex(scr, width, rowCount)
				return
			}
			vv.renderTextRows(scr, width, rowCount, false)
		})

	return semanticRowsWithContentKeys(
		semanticRowsWithRenderedRunsAt(window.rows, rendered.Rows, 0))
}

// semanticStyledEditorWindowRows consumes the shared editor row projection
// directly for text. Syntax, rectangular selection, crosshair, whitespace,
// horizontal scrolling and autocomplete therefore have one styling
// implementation for console and native output without a temporary screen or
// viewport mutation. Regular stream selection is a separate native overlay so
// moving its endpoint never mutates these base rows. Non-text modes retain
// their existing specialized renderer.
func semanticStyledEditorWindowRows(ev *EditorView, window semanticSurfaceWindow, width int) []extui.TextRowModel {
	if ev == nil || width <= 0 || len(window.rows) == 0 {
		return window.rows
	}
	if !semanticEditorStyledRowsCacheEligible(ev) {
		ev.semanticStyledRows = nil
		ev.semanticStyledRowsContextValid = false
		ev.semanticStyledRowsRendered += uint64(len(window.rows))
		return semanticRenderStyledEditorWindowRows(ev, window, width)
	}

	context := semanticEditorStyledRowsContextFor(ev, width)
	if !ev.semanticStyledRowsContextValid || ev.semanticStyledRowsContext != context {
		ev.semanticStyledRows = nil
		ev.semanticStyledRowsContext = context
		ev.semanticStyledRowsContextValid = true
	}

	selection := semanticEditorSelectionStateFor(ev)
	result := append([]extui.TextRowModel(nil), window.rows...)
	keys := make([]semanticEditorStyledRowKey, len(window.rows))
	missing := make([]bool, len(window.rows))
	for index, row := range window.rows {
		key := semanticEditorStyledRowKeyFor(row, selection)
		keys[index] = key
		entry, present := ev.semanticStyledRows[row.VisualRow]
		if !present || entry.key != key {
			missing[index] = true
			continue
		}
		result[index].Text = ""
		result[index].Runs = entry.row.Runs
		result[index].ContentKey = entry.row.ContentKey
	}

	for first := 0; first < len(missing); {
		for first < len(missing) && !missing[first] {
			first++
		}
		if first >= len(missing) {
			break
		}
		last := first + 1
		for last < len(missing) && missing[last] {
			last++
		}
		rangeWindow := semanticSurfaceWindow{
			rows:  window.rows[first:last],
			start: int64(window.rows[first].VisualRow),
			end:   int64(window.rows[last-1].VisualRow + 1),
		}
		ev.semanticStyledRowsRendered += uint64(last - first)
		styled := semanticRenderStyledEditorWindowRows(ev, rangeWindow, width)
		if len(styled) != last-first {
			ev.semanticStyledRows = nil
			ev.semanticStyledRowsContextValid = false
			return nil
		}
		copy(result[first:last], styled)
		first = last
	}

	// Rendering may have started the optional syntax fade. Do not retain an
	// intermediate colour blend: the fade heartbeat must continue to repaint
	// every row until the final colours are stable.
	if !semanticEditorStyledRowsCacheEligible(ev) {
		ev.semanticStyledRows = nil
		ev.semanticStyledRowsContextValid = false
		return result
	}

	nextCache := make(map[int]semanticEditorStyledRowCacheEntry, len(result))
	for index, row := range result {
		nextCache[row.VisualRow] = semanticEditorStyledRowCacheEntry{
			key: keys[index],
			row: row,
		}
	}
	ev.semanticStyledRows = nextCache
	return result
}

func semanticRenderStyledEditorWindowRows(ev *EditorView, window semanticSurfaceWindow, width int) []extui.TextRowModel {
	if ev == nil || width <= 0 || len(window.rows) == 0 {
		return window.rows
	}
	if !ev.HexMode && !ev.DecodeMode && !ev.saving && !ev.pasting && ev.targetLine == -1 {
		result := append([]extui.TextRowModel(nil), window.rows...)
		projected, failed := 0, false
		ev.projectTextRows(int(window.start), width, len(result), ev.textProjectionStyle(), func(row editorProjectedTextRow) {
			index := row.visualRow - int(window.start)
			if row.err != nil {
				failed = true
				if row.err != piecetable.ErrLoading {
					ev.semanticLoadError = row.err.Error()
				}
				return
			}
			if index < 0 || index >= len(result) ||
				result[index].Offset != int64(row.fragment.ByteOffsetStart) ||
				result[index].EndOffset != int64(row.fragment.ByteOffsetEnd) {
				failed = true
				return
			}
			result[index].Text = ""
			result[index].Runs = semanticRunsFromCells(row.cells)
			projected++
		})
		if failed || projected != len(result) {
			return nil
		}
		ev.semanticLoadError = ""
		return semanticRowsWithContentKeys(result)
	}

	scrollTopRow := ev.ScrollTopRow
	x2, y2 := ev.X2, ev.Y2
	visible := ev.IsVisible()
	scrollBar := semanticCaptureScrollBar(ev.scrollBar)
	defer func() {
		ev.ScrollTopRow = scrollTopRow
		ev.X2 = x2
		ev.Y2 = y2
		ev.SetVisible(visible)
		semanticRestoreScrollBar(ev.scrollBar, scrollBar)
	}()

	ev.ScrollTopRow = int(window.start)
	ev.X2 = ev.X1 + width - 1
	if ev.scrollBar != nil {
		ev.X2++
	}
	ev.Y2 = ev.Y1 + len(window.rows)
	// SemanticNode is normally requested only for visible frames, but making
	// the off-screen render independent of that flag keeps the helper total
	// and does not leak visibility back into the live frame.
	ev.SetVisible(true)
	rendered := semanticRenderSurface(ev.X1, ev.Y1+1,
		ev.X1+width-1, ev.Y1+len(window.rows), ev.DisplayObject)
	return semanticRowsWithContentKeys(
		semanticRowsWithRenderedRunsAt(window.rows, rendered.Rows, 0))
}

func semanticRowsWithContentKeys(rows []extui.TextRowModel) []extui.TextRowModel {
	for index := range rows {
		rows[index].ContentKey = extui.TextRowContentKey(rows[index])
	}
	return rows
}

func semanticEditorStyledRowsCacheEligible(ev *EditorView) bool {
	if ev == nil || ev.pt == nil || ev.li == nil || ev.engine == nil ||
		ev.pasting || ev.saving || ev.targetLine != -1 || ev.HexMode ||
		ev.DecodeMode || ev.DisasmMode != 0 || ev.highlighting ||
		(ev.acEnabled && len(ev.acMatches) > 0) {
		return false
	}
	showHorz, showVert, _, _ := EditorCrossAttrs()
	if showHorz || showVert {
		return false
	}
	return ev.syntaxFadeStart.IsZero() ||
		time.Since(ev.syntaxFadeStart) >= syntaxFadeDuration
}

func semanticEditorStyledRowsContextFor(ev *EditorView, width int) semanticEditorStyledRowsContext {
	var highlighterType reflect.Type
	var highlighterID uintptr
	if ev.highlighter != nil {
		highlighterType = reflect.TypeOf(ev.highlighter)
		value := reflect.ValueOf(ev.highlighter)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer,
			reflect.Slice, reflect.UnsafePointer:
			highlighterID = value.Pointer()
		}
	}
	return semanticEditorStyledRowsContext{
		editSession:       ev.editSession,
		width:             width,
		scrollLeft:        ev.ScrollLeft,
		tabSize:           ev.TabSize,
		wordWrap:          ev.WordWrap,
		showWhitespaces:   ev.ShowWhitespaces,
		paletteHash:       semanticEditorPaletteHash(),
		colorerGeneration: ColorerSchemeGeneration(),
		highlighterType:   highlighterType,
		highlighterID:     highlighterID,
	}
}

func semanticEditorPaletteHash() uint64 {
	const offset64 = uint64(14695981039346656037)
	const prime64 = uint64(1099511628211)
	hash := offset64
	mix := func(value uint64) {
		for byteIndex := 0; byteIndex < 8; byteIndex++ {
			hash ^= value & 0xff
			hash *= prime64
			value >>= 8
		}
	}
	mix(ColorerEditorBaseAttr(vtui.Palette[ColEditorText]))
	mix(vtui.Palette[vtui.ColDialogEditSelected])
	mix(vtui.Palette[ColCommandLineUserScreen])
	for _, color := range vtui.ThemePalette {
		mix(uint64(color))
	}
	return hash
}

func semanticEditorSelectionStateFor(ev *EditorView) semanticEditorSelectionState {
	if ev.rectSelActive {
		top, bottom := ev.rectSelStartLine, ev.CursorLine
		if top > bottom {
			top, bottom = bottom, top
		}
		left, right := ev.rectSelStartCol,
			ev.getVisualColOf(ev.CursorLine, ev.CursorPos)
		if left > right {
			left, right = right, left
		}
		return semanticEditorSelectionState{
			kind: 2, top: top, bottom: bottom, left: left, right: right,
		}
	}
	return semanticEditorSelectionState{}
}

func semanticEditorStyledRowKeyFor(row extui.TextRowModel,
	selection semanticEditorSelectionState,
) semanticEditorStyledRowKey {
	selectionKey := semanticEditorRowSelectionKey{}
	switch selection.kind {
	case 2:
		if row.VisualRow >= selection.top && row.VisualRow <= selection.bottom {
			selectionKey = semanticEditorRowSelectionKey{
				kind: 2, left: selection.left, right: selection.right,
			}
		}
	}
	return semanticEditorStyledRowKey{
		visualRow:   row.VisualRow,
		logicalLine: row.LogicalLine,
		offset:      row.Offset,
		endOffset:   row.EndOffset,
		text:        row.Text,
		selection:   selectionKey,
	}
}

func cloneInt64Slice(values []int64) []int64 {
	if values == nil {
		return nil
	}
	return append([]int64(nil), values...)
}

type semanticScrollBarSnapshot struct {
	present bool
	visible bool
	value   int
	min     int
	max     int
	pgStep  int
}

func semanticCaptureScrollBar(scrollBar *vtui.ScrollBar) semanticScrollBarSnapshot {
	if scrollBar == nil {
		return semanticScrollBarSnapshot{}
	}
	return semanticScrollBarSnapshot{
		present: true,
		visible: scrollBar.IsVisible(),
		value:   scrollBar.Value,
		min:     scrollBar.Min,
		max:     scrollBar.Max,
		pgStep:  scrollBar.PgStep,
	}
}

func semanticRestoreScrollBar(scrollBar *vtui.ScrollBar, state semanticScrollBarSnapshot) {
	if scrollBar == nil || !state.present {
		return
	}
	scrollBar.Value = state.value
	scrollBar.Min = state.min
	scrollBar.Max = state.max
	scrollBar.PgStep = state.pgStep
	scrollBar.SetVisible(state.visible)
}
