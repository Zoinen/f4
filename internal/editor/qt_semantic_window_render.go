package editor

import (
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"reflect"
	"time"
)

type semanticEditorStyledRowsContext struct {
	occurrenceNeedle  string
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

func semanticStyledEditorWindowRows(ev *EditorView, window semantic.SemanticSurfaceWindow, width int) []extui.TextRowModel {
	if ev == nil || width <= 0 || len(window.Rows) == 0 {
		return window.Rows
	}
	if !semanticEditorStyledRowsCacheEligible(ev) {
		ev.semanticStyledRows = nil
		ev.semanticStyledRowsContextValid = false
		ev.semanticStyledRowsRendered += uint64(len(window.Rows))
		return semanticRenderStyledEditorWindowRows(ev, window, width)
	}

	context := semanticEditorStyledRowsContextFor(ev, width)
	if !ev.semanticStyledRowsContextValid || ev.semanticStyledRowsContext != context {
		ev.semanticStyledRows = nil
		ev.semanticStyledRowsContext = context
		ev.semanticStyledRowsContextValid = true
	}

	selection := semanticEditorSelectionStateFor(ev)
	result := append([]extui.TextRowModel(nil), window.Rows...)
	keys := make([]semanticEditorStyledRowKey, len(window.Rows))
	missing := make([]bool, len(window.Rows))
	for index, row := range window.Rows {
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
		rangeWindow := semantic.SemanticSurfaceWindow{
			Rows:  window.Rows[first:last],
			Start: int64(window.Rows[first].VisualRow),
			End:   int64(window.Rows[last-1].VisualRow + 1),
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

func semanticRenderStyledEditorWindowRows(ev *EditorView, window semantic.SemanticSurfaceWindow, width int) []extui.TextRowModel {
	if ev == nil || width <= 0 || len(window.Rows) == 0 {
		return window.Rows
	}
	if !ev.HexMode && !ev.DecodeMode && !ev.Saving && !ev.pasting && ev.TargetLine == -1 {
		result := append([]extui.TextRowModel(nil), window.Rows...)
		projected, failed := 0, false
		ev.projectTextRows(int(window.Start), width, len(result), ev.textProjectionStyle(), func(row editorProjectedTextRow) {
			index := row.visualRow - int(window.Start)
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
			result[index].Runs = semantic.RunsFromCells(row.cells)
			projected++
		})
		if failed || projected != len(result) {
			return nil
		}
		ev.semanticLoadError = ""
		return semantic.SemanticRowsWithContentKeys(result)
	}

	scrollTopRow := ev.ScrollTopRow
	x2, y2 := ev.X2, ev.Y2
	visible := ev.IsVisible()
	scrollBar := semantic.SemanticCaptureScrollBar(ev.ScrollBar)
	defer func() {
		ev.ScrollTopRow = scrollTopRow
		ev.X2 = x2
		ev.Y2 = y2
		ev.SetVisible(visible)
		semantic.SemanticRestoreScrollBar(ev.ScrollBar, scrollBar)
	}()

	ev.ScrollTopRow = int(window.Start)
	ev.X2 = ev.X1 + width - 1
	if ev.ScrollBar != nil {
		ev.X2++
	}
	ev.Y2 = ev.Y1 + len(window.Rows)
	// SemanticNode is normally requested only for visible frames, but making
	// the off-screen render independent of that flag keeps the helper total
	// and does not leak visibility back into the live frame.
	ev.SetVisible(true)
	rendered := semantic.SemanticRenderSurface(ev.X1, ev.Y1+1,
		ev.X1+width-1, ev.Y1+len(window.Rows), ev.DisplayObject)
	return semantic.SemanticRowsWithContentKeys(
		semantic.SemanticRowsWithRenderedRunsAt(window.Rows, rendered.Rows, 0))
}

func semanticEditorStyledRowsCacheEligible(ev *EditorView) bool {
	if ev == nil || ev.Pt == nil || ev.Li == nil || ev.Engine == nil ||
		ev.pasting || ev.Saving || ev.TargetLine != -1 || ev.HexMode ||
		ev.DecodeMode || ev.highlighting ||
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
	if ev.Highlighter != nil {
		highlighterType = reflect.TypeOf(ev.Highlighter)
		value := reflect.ValueOf(ev.Highlighter)
		switch value.Kind() {
		case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer,
			reflect.Slice, reflect.UnsafePointer:
			highlighterID = value.Pointer()
		}
	}
	return semanticEditorStyledRowsContext{
		occurrenceNeedle:  string(ev.occurrenceNeedle()),
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
	mix(ColorerEditorBaseAttr(vtui.Palette[theme.ColEditorText]))
	mix(vtui.Palette[vtui.ColDialogEditSelected])
	mix(vtui.Palette[theme.ColCommandLineUserScreen])
	for _, color := range vtui.ThemePalette {
		mix(uint64(color))
	}
	return hash
}

func semanticEditorSelectionStateFor(ev *EditorView) semanticEditorSelectionState {
	if ev.RectSelActive {
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
