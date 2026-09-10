package editor

import (
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/textlayout"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type semanticEditorCursorState struct {
	secondaryCarets       []extui.CaretModel
	line                  int
	pos                   int
	visualRow             int
	visualColumn          int
	visible               bool
	shape                 string
	absoluteRow           int64
	absoluteColumn        int
	selection             bool
	selectionAnchorRow    int64
	selectionAnchorColumn int
	selectionForeground   string
	selectionBackground   string
	selectionBold         bool
	selectionUnderline    bool
	selectionStrikeout    bool
}

func (state semanticEditorCursorState) ToMap() map[string]any {
	return map[string]any{
		"cursorLine":            state.line,
		"cursorPos":             state.pos,
		"cursorVisualRow":       state.visualRow,
		"cursorVisualColumn":    state.visualColumn,
		"cursorVisible":         state.visible,
		"cursorShape":           state.shape,
		"cursorAbsoluteRow":     state.absoluteRow,
		"cursorAbsoluteColumn":  state.absoluteColumn,
		"selection":             state.selection,
		"selectionAnchorRow":    state.selectionAnchorRow,
		"selectionAnchorColumn": state.selectionAnchorColumn,
		"selectionForeground":   state.selectionForeground,
		"selectionBackground":   state.selectionBackground,
		"selectionBold":         state.selectionBold,
		"selectionUnderline":    state.selectionUnderline,
		"selectionStrikeout":    state.selectionStrikeout,
		"secondaryCarets":       extui.CaretsToMaps(state.secondaryCarets),
	}
}

func (ev *EditorView) semanticSurfaceWidth() int {
	return ev.viewportWidth()
}

func (ev *EditorView) semanticCursorState(width int) semanticEditorCursorState {
	if ev.reflow != nil {
		return semanticEditorCursorState{line: ev.CursorLine, pos: ev.CursorPos}
	}
	cursorOffset := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos
	cursorAbsoluteRow, cursorAbsoluteColumn := ev.Engine.LogicalToVisual(cursorOffset)
	cursorVisualRow := cursorAbsoluteRow - ev.ScrollTopRow
	cursorVisualColumn := cursorAbsoluteColumn + ev.CursorVirtualSpaces - ev.ScrollLeft
	cursorVisible := ev.IsVisible() && !ev.pasting && !ev.Saving &&
		cursorVisualColumn >= 0 && cursorVisualColumn < width
	cursorShape := "underline"
	if ev.Overtype {
		cursorShape = "block"
	}
	// DisasmMode is the remembered CPU bitness, also detected for plain text
	// files. Only DecodeMode/HexMode indicate a non-text presentation.
	selected := semantic.RunModel("", vtui.Palette[vtui.ColDialogEditSelected])
	state := semanticEditorCursorState{
		line:           ev.CursorLine,
		pos:            ev.CursorPos,
		visualRow:      cursorVisualRow,
		visualColumn:   cursorVisualColumn,
		visible:        cursorVisible,
		shape:          cursorShape,
		absoluteRow:    int64(cursorAbsoluteRow),
		absoluteColumn: cursorAbsoluteColumn + ev.CursorVirtualSpaces,
		selection: ev.SelActive && !ev.RectSelActive && !ev.HexMode &&
			!ev.DecodeMode && !ev.Saving &&
			!ev.pasting && ev.TargetLine == -1,
		selectionForeground: selected.Foreground,
		selectionBackground: selected.Background,
		selectionBold:       selected.Bold,
		selectionUnderline:  selected.Underline,
		selectionStrikeout:  selected.Strikeout,
	}
	if !ev.HexMode && !ev.DecodeMode && !ev.Saving && !ev.pasting {
		for _, caret := range ev.extraCursors {
			row, column := ev.Engine.LogicalToVisual(caret.off)
			anchorRow, anchorColumn := ev.Engine.LogicalToVisual(caret.anchor)
			state.secondaryCarets = append(state.secondaryCarets, extui.CaretModel{
				CursorAbsoluteRow: int64(row), CursorAbsoluteColumn: column,
				Selection: caret.hasSel, SelectionAnchorRow: int64(anchorRow), SelectionAnchorColumn: anchorColumn,
			})
		}
	}
	if state.selection {
		anchorRow, anchorColumn := ev.Engine.LogicalToVisual(ev.SelAnchorOffset)
		state.selectionAnchorRow = int64(anchorRow)
		state.selectionAnchorColumn = anchorColumn
	}
	return state
}

func (ev *EditorView) queueSemanticCursorState() bool {
	if ev == nil || ev.Li == nil || ev.Engine == nil || vtui.FrameManager == nil {
		return false
	}
	screen := vtui.FrameManager.Screen()
	if screen == nil || screen.Renderer == nil {
		return false
	}
	renderer, ok := screen.Renderer.(interface {
		QueueSurfaceState(string, map[string]any) bool
	})
	if !ok {
		return false
	}
	state := ev.semanticCursorState(ev.semanticSurfaceWidth())
	stateMap := state.ToMap()
	stateMap["layoutRevision"] = ev.semanticLayoutRevision
	stateMap["windowGeneration"] = ev.semanticWindowGeneration
	stateMap["documentKey"] = vtui.SemanticID(ev)
	_, stateMap["topBarRight"] = viewer.SemanticTopBarStrings(ev.topBar)
	return renderer.QueueSurfaceState(vtui.SemanticID(ev), stateMap)
}

func (ev *EditorView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	layoutReady := ev.EnsureEngineWidth()
	window := semantic.SemanticSurfaceWindow{ViewportRows: ev.ViewportHeight()}
	previousTop := ev.ScrollTopRow
	if layoutReady && !semantic.NativeDocumentLayoutPending(ev.NativeViewportRevision) && ev.TargetLine == -1 {
		if ev.semanticPendingScroll {
			ev.ScrollTopRow = ev.semanticPendingTop
		}
		window = ev.semanticWindowMetadata()
	}
	width := ev.semanticSurfaceWidth()
	if window.LoadError == "" {
		window.LoadError = ev.semanticLoadError
	}
	if window.Ready {
		window.Rows = semanticStyledEditorWindowRows(ev, window, width)
		window.Ready = len(window.Rows) == int(window.End-window.Start)
		if !window.Ready && window.LoadError == "" {
			window.LoadError = ev.semanticLoadError
		}
		ev.scheduleNativeVisualExtent()
	}
	if ev.semanticPendingScroll {
		if window.Ready {
			ev.semanticWindowGeneration = ev.semanticPendingGeneration
			ev.semanticPendingScroll = false
		} else {
			ev.ScrollTopRow = previousTop
		}
	}
	totalRows, extentReady := ev.Engine.KnownVisualRows()
	visibleEnd := min(window.ViewportRow+window.ViewportRows, len(window.Rows))
	visibleRows := window.Rows
	if window.ViewportRow >= 0 && window.ViewportRow <= visibleEnd {
		visibleRows = window.Rows[window.ViewportRow:visibleEnd]
	}
	cursor := ev.semanticCursorState(width)
	topBarLeft, topBarRight := viewer.SemanticTopBarStrings(ev.topBar)

	surface := extui.SurfaceModel{
		ID:                      vtui.SemanticID(ev),
		Kind:                    "editor",
		DefaultBackground:       semantic.SemanticAttrColor(ColorerEditorBaseAttr(vtui.Palette[theme.ColEditorText]), false),
		Title:                   ev.GetTitle(),
		Path:                    ev.FilePath,
		LocalPath:               semantic.SemanticLocalPath(ev.Vfs, ev.FilePath),
		BaseName:                semantic.SemanticBaseName(ev.Vfs, ev.FilePath),
		TopBarLeft:              topBarLeft,
		TopBarRight:             topBarRight,
		IconColor:               theme.SemanticFileIconColor(semantic.SemanticBaseName(ev.Vfs, ev.FilePath)),
		Busy:                    ev.IsBusy(),
		Dirty:                   ev.Modified,
		Saving:                  ev.Saving,
		WordWrap:                ev.WordWrap,
		Overtype:                ev.Overtype,
		CursorLine:              cursor.line,
		CursorPos:               cursor.pos,
		CursorVisualRow:         cursor.visualRow,
		CursorVisualColumn:      cursor.visualColumn,
		CursorVisible:           cursor.visible,
		CursorShape:             cursor.shape,
		CursorAbsoluteColumn:    cursor.absoluteColumn,
		ScrollTop:               ev.ScrollTopRow,
		ScrollLeft:              ev.ScrollLeft,
		DocumentKey:             vtui.SemanticID(ev),
		ScrollAction:            "editor.scroll",
		ScrollUnit:              "rows",
		WindowStart:             window.Start,
		WindowEnd:               window.End,
		ViewportStart:           int64(ev.ScrollTopRow),
		ViewportSpan:            window.ViewportSpan,
		ContentExtent:           int64(totalRows),
		ContentExtentKnown:      ev.semanticExtentKnown && extentReady,
		ViewportRow:             window.ViewportRow,
		ViewportRows:            window.ViewportRows,
		CursorAbsoluteRow:       cursor.absoluteRow,
		WindowGeneration:        ev.semanticWindowGeneration,
		WindowRequestGeneration: ev.semanticWindowRequestGeneration,
		ViewportColumns:         width,
		GeometryRevision:        ev.NativeViewportRevision,
		LayoutRevision:          ev.semanticLayoutRevision,
		LayoutPending:           !window.Ready,
		LoadError:               window.LoadError,
		Selection:               cursor.selection,
		SelectionAnchorRow:      cursor.selectionAnchorRow,
		SelectionAnchorColumn:   cursor.selectionAnchorColumn,
		SelectionForeground:     cursor.selectionForeground,
		SelectionBackground:     cursor.selectionBackground,
		SelectionBold:           cursor.selectionBold,
		SelectionUnderline:      cursor.selectionUnderline,
		SelectionStrikeout:      cursor.selectionStrikeout,
		SecondaryCarets:         cursor.secondaryCarets,
		Rows:                    visibleRows,
		WindowRows:              window.Rows,
		Autocomplete:            ev.semanticAutocomplete(),
	}
	navtrace.TraceDocumentWindow(&surface)
	ev.tracePointerWindow(&surface)
	return surface.ToMap()
}

func (ev *EditorView) GetText() string {
	if ev.Pt == nil {
		return ""
	}
	return ev.Pt.String()
}

func (ev *EditorView) HandleSemanticAction(action map[string]any) bool {
	target := semantic.String(action["target"])
	if vtui.SemanticID(ev) != target {
		return false
	}

	switch semantic.String(action["action"]) {
	case "document.viewport":
		semantic.ApplyNativeDocumentViewport(ev, semantic.TargetedDocumentGeometry(action, ev.NativeViewportColumns))
		return true
	case "editor.setText":
		text := semantic.String(action["text"])
		ev.SetText(text)
		return true
	case "editor.insertText":
		text := semantic.String(action["text"])
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
		pattern := semantic.String(action["pattern"])
		caseSensitive := semantic.Bool(action["case"])
		reverse := semantic.Bool(action["reverse"])
		next := semantic.Bool(action["next"])
		ev.Search(pattern, caseSensitive, reverse, false, false, next)
		return true
	case "editor.mouse":
		pointerDisposition := "legacy"
		if navtrace.NavigationBenchmarkIsEnabled() {
			ev.tracePointerAction(action, "received", "received")
			defer func() { ev.tracePointerAction(action, "applied", pointerDisposition) }()
		}
		if !semantic.SemanticDocumentLayoutMatches(action, vtui.SemanticID(ev), ev.semanticLayoutRevision) &&
			semantic.String(action["phase"]) != "release" && semantic.String(action["phase"]) != "cancel" {
			pointerDisposition = "stale-layout"
			vtui.FrameManager.DeclareCurrentInputUnchanged()
			return true
		}
		buttonState := uint32(0)
		switch semantic.String(action["button"]) {
		case "left":
			buttonState = vtinput.FromLeft1stButtonPressed
		case "right":
			buttonState = vtinput.RightmostButtonPressed
		case "middle":
			buttonState = vtinput.FromLeft2ndButtonPressed
		}
		if phase := semantic.String(action["phase"]); phase == "release" || phase == "cancel" {
			buttonState = 0
		}
		flags := uint32(0)
		if semantic.Bool(action["moved"]) {
			flags |= vtinput.MouseMoved
		}
		if semantic.Bool(action["doubleClick"]) {
			flags |= vtinput.DoubleClick
		}
		controlState := vtinput.ControlKeyState(0)
		if semantic.Bool(action["shift"]) {
			controlState |= vtinput.ShiftPressed
		}
		if semantic.Bool(action["ctrl"]) {
			controlState |= vtinput.LeftCtrlPressed
		}
		if semantic.Bool(action["alt"]) {
			controlState |= vtinput.LeftAltPressed
		}
		column := semantic.Int(action["column"])
		if _, sourceAddressed := action["rowOffset"]; !sourceAddressed {
			column = max(0, column)
		}
		row := max(0, semantic.Int(action["row"]))
		event := &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(min(ev.X2, ev.X1+column)),
			MouseY:          int16(min(ev.Y2, ev.Y1+1+row)),
			ButtonState:     buttonState,
			MouseEventFlags: flags,
			WheelDirection:  semantic.Int(action["wheelDirection"]),
			KeyDown:         semantic.String(action["phase"]) != "release" && semantic.String(action["phase"]) != "cancel",
			ControlKeyState: controlState,
			InputSource:     "semantic",
		}
		if offset, exists := action["rowOffset"]; exists {
			guard := ev.editorCursorStateGuard()
			changed := ev.processDocumentPointer(event, semantic.Int64(offset), column,
				max(0, semantic.Int(action["scrollLeft"])),
				uint64(max(int64(0), semantic.Int64(action["layoutRevision"]))))
			if !changed {
				pointerDisposition = "unchanged"
				vtui.FrameManager.DeclareCurrentInputUnchanged()
			} else {
				pointerDisposition = "row-update"
				navtrace.NavigationBenchmarkPublishScene(navtrace.NavigationBenchmarkCurrentUI(), "editor.pointer")
				if guard.canPublish(ev, true) && ev.queueSemanticCursorState() {
					pointerDisposition = "cursor-state"
				}
			}
			return true
		}
		ev.ProcessMouse(event)
		return true
	case "editor.scroll":
		if !semantic.SemanticDocumentLayoutMatches(action, vtui.SemanticID(ev), ev.semanticLayoutRevision) {
			return true
		}
		generation, accepted := semantic.SemanticAcceptWindowGeneration(action,
			ev.semanticWindowGeneration, &ev.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		ev.EnsureEngineWidth()
		top := semantic.Int(action["visualRow"])
		height := max(1, ev.ViewportHeight())
		knownRows, extentReady := ev.Engine.KnownVisualRows()
		maxTop := max(0, knownRows-height)
		if top < 0 {
			top = 0
		}
		if extentReady && ev.semanticExtentKnown && top > maxTop {
			top = maxTop
		}
		ev.semanticPendingScroll = true
		ev.semanticPendingTop = top
		ev.semanticPendingGeneration = generation
		return true
	case "control.focus":
		ev.SetFocus(true)
		return true
	}
	return false
}

func (ev *EditorView) tracePointerAction(action map[string]any, phase, disposition string) {
	if !navtrace.NavigationBenchmarkIsEnabled() {
		return
	}
	_, generationKnown := action["windowGeneration"]
	_, sourceAddressed := action["rowOffset"]
	navtrace.NavigationBenchmarkUIEvent("editor.pointer."+phase,
		"documentKey", vtui.SemanticID(ev), "layoutRevision", ev.semanticLayoutRevision,
		"windowGeneration", ev.semanticWindowGeneration, "requestGeneration", ev.semanticWindowRequestGeneration,
		"inputDocumentKey", semantic.String(action["documentKey"]),
		"inputLayoutRevision", semantic.Int64(action["layoutRevision"]),
		"inputWindowGenerationKnown", generationKnown, "inputWindowGeneration", semantic.Int64(action["windowGeneration"]),
		"sourceAddressed", sourceAddressed, "fragmentOffset", semantic.Int64(action["rowOffset"]),
		"column", semantic.Int(action["column"]), "inputScrollLeft", semantic.Int(action["scrollLeft"]),
		"pointerPhase", semantic.String(action["phase"]), "button", semantic.String(action["button"]),
		"moved", semantic.Bool(action["moved"]), "disposition", disposition,
		"cursorLine", ev.CursorLine, "cursorPos", ev.CursorPos,
		"cursorOffset", ev.Li.GetLineOffset(ev.CursorLine)+ev.CursorPos,
		"selectionActive", ev.SelActive, "selectionAnchor", ev.SelAnchorOffset,
		"rectSelectionActive", ev.RectSelActive, "rectAnchorLine", ev.rectSelStartLine, "rectAnchorColumn", ev.rectSelStartCol,
		"pointerCaptured", ev.semanticPointerActive, "viewportStart", ev.ScrollTopRow, "scrollLeft", ev.ScrollLeft)
}

func (ev *EditorView) tracePointerWindow(surface *extui.SurfaceModel) {
	if !navtrace.NavigationBenchmarkIsEnabled() {
		return
	}
	traceID := ""
	if trace := navtrace.NavigationBenchmarkCurrentOrPublishedTrace(); trace != nil {
		traceID = trace.Id
	}
	windowContentKey := surface.WindowContentKey
	if windowContentKey == "" {
		windowContentKey = extui.WindowRowsContentKey(surface.WindowRows)
	}
	navtrace.NavigationBenchmarkEmit(traceID, "editor.pointer.window", "go.ui",
		"documentKey", surface.DocumentKey, "layoutRevision", surface.LayoutRevision,
		"windowContentKey", windowContentKey,
		"windowGeneration", surface.WindowGeneration, "requestGeneration", surface.WindowRequestGeneration,
		"ready", !surface.LayoutPending, "windowStart", surface.WindowStart, "windowEnd", surface.WindowEnd,
		"rows", len(surface.WindowRows), "viewportStart", surface.ViewportStart, "scrollLeft", ev.ScrollLeft,
		"cursorLine", surface.CursorLine, "cursorPos", surface.CursorPos,
		"cursorOffset", ev.Li.GetLineOffset(ev.CursorLine)+ev.CursorPos,
		"cursorAbsoluteRow", surface.CursorAbsoluteRow, "cursorVisualColumn", surface.CursorVisualColumn,
		"selectionActive", ev.SelActive, "selectionAnchor", ev.SelAnchorOffset,
		"rectSelectionActive", ev.RectSelActive, "rectAnchorLine", ev.rectSelStartLine, "rectAnchorColumn", ev.rectSelStartCol)
}

func (ev *EditorView) semanticWindow() semantic.SemanticSurfaceWindow {
	return ev.semanticWindowRows(false)
}

func (ev *EditorView) semanticWindowMetadata() semantic.SemanticSurfaceWindow {
	return ev.semanticWindowRows(true)
}

func (ev *EditorView) semanticWindowRows(metadataOnly bool) semantic.SemanticSurfaceWindow {
	window := semantic.SemanticSurfaceWindow{}
	if ev.Pt == nil || ev.Li == nil || ev.Engine == nil {
		return window
	}
	if !ev.EnsureEngineWidth() {
		window.LoadError = ev.semanticLoadError
		return window
	}
	height := ev.ViewportHeight()
	if height <= 0 {
		return window
	}
	window.ViewportRows = height
	buffer := semantic.SemanticWindowBufferRows(height)
	start := max(0, ev.ScrollTopRow-buffer)
	end := ev.ScrollTopRow + height + buffer
	mapping := ev.Engine.AdvanceToVisualRow(max(0, end-1), editorMappingWorkBytes)
	if !mapping.Ready {
		ev.continueEditorMapping(mapping.Progress, mapping.Err)
		window.LoadError = ev.semanticLoadError
		return window
	}
	total, complete := ev.Engine.KnownVisualRows()
	if complete && ev.semanticExtentKnown && ev.semanticPendingScroll {
		ev.ScrollTopRow = min(ev.ScrollTopRow, max(0, total-height))
		start = max(0, ev.ScrollTopRow-buffer)
		end = ev.ScrollTopRow + height + buffer
	}
	if err := ev.Engine.LastReadError(); err != nil && err != piecetable.ErrLoading {
		window.LoadError = err.Error()
	}
	end = min(end, total)
	window.Start, window.End = int64(start), int64(end)
	window.ViewportRow = ev.ScrollTopRow - start
	window.ViewportSpan = int64(min(height, max(0, total-ev.ScrollTopRow)))
	line, fragment := ev.Engine.GetLogLineAtVisualRow(start)
	for logIdx := line; logIdx < ev.Li.LineCount() && len(window.Rows) < end-start; logIdx++ {
		base := ev.Engine.GetRowOffset(logIdx)
		var fragments []textlayout.LineFragment
		if metadataOnly {
			fragments = ev.Engine.GetProjectionFragments(logIdx, end-base, ev.ScrollLeft+ev.viewportWidth())
		} else {
			fragments = ev.Engine.GetFragmentsThrough(logIdx, end-base)
		}
		for index, frag := range fragments {
			if logIdx == line && index < fragment {
				continue
			}
			if frag.Loading {
				if err := ev.Engine.LastReadError(); err != nil {
					window.LoadError = err.Error()
				}
				return window
			}
			visual := base + index
			if visual >= end {
				break
			}
			text := ""
			if !metadataOnly {
				data, err := ev.Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
				if err != nil {
					if err != piecetable.ErrLoading {
						window.LoadError = err.Error()
					}
					return window
				}
				text = string(data)
			}
			window.Rows = append(window.Rows, extui.TextRowModel{
				Index: len(window.Rows), VisualRow: visual, LogicalLine: logIdx,
				Offset: int64(frag.ByteOffsetStart), EndOffset: int64(frag.ByteOffsetEnd),
				VisualWidth: frag.VisualWidth, HasVisualWidth: true, Text: text,
			})
		}
	}
	visible := len(window.Rows) - window.ViewportRow
	window.Ready = visible >= height || (complete && ev.semanticExtentKnown && int(window.End) >= total)
	return window
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
