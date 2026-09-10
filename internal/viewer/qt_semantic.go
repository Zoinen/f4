package viewer

import (
	"fmt"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"strings"
	"time"
)

func (vv *ViewerView) HandleSemanticAction(action map[string]any) bool {
	target := semantic.String(action["target"])
	if vtui.SemanticID(vv) != target {
		return false
	}
	vv.ensureTextLayoutSettings()

	switch semantic.String(action["action"]) {
	case "document.viewport":
		semantic.ApplyNativeDocumentViewport(vv, semantic.TargetedDocumentGeometry(action, vv.NativeViewportColumns))
		return true
	case "viewer.scroll":
		if !semantic.SemanticDocumentLayoutMatches(action, vtui.SemanticID(vv), vv.SemanticLayoutRevision) {
			return true
		}
		generation, accepted := semantic.SemanticAcceptWindowGeneration(action,
			vv.semanticWindowGeneration, &vv.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		offset := semantic.Int64(action["offset"])
		if offset < 0 {
			offset = 0
		}
		if offset > vv.Backend.Size() {
			offset = vv.Backend.Size()
		}
		if vv.HexMode {
			offset &= ^int64(0xF)
		} else if vv.DecodeMode {
			// In DecodeMode, offset is a raw byte offset
		} else {
			offset = vv.clampTextScrollOffset(offset)
			offset = vv.Backend.FindLineStart(offset)
		}
		vv.TopOffset = offset
		vv.EofVisible = false
		vv.SemanticPendingScroll = false
		vv.SemanticPendingGeneration = 0
		vv.semanticWindowGeneration = generation
		return true
	case "viewer.scrollWindow":
		if !semantic.SemanticDocumentLayoutMatches(action, vtui.SemanticID(vv), vv.SemanticLayoutRevision) {
			return true
		}
		generation, accepted := semantic.SemanticAcceptWindowGeneration(action, vv.semanticWindowGeneration, &vv.semanticWindowRequestGeneration)
		if !accepted {
			return true
		}
		offset := max(int64(0), min(semantic.Int64(action["offset"]), vv.Backend.Size()))
		if vv.HexMode {
			offset &= ^int64(15)
		} else if vv.DecodeMode {
			// In DecodeMode, offset is a raw byte offset
		} else {
			offset = vv.clampTextScrollOffset(offset)
		}
		vv.SemanticPendingScroll = true
		vv.semanticPendingOffset = offset
		vv.SemanticPendingGeneration = generation
		return true
	case "control.focus":
		vv.SetFocus(true)
		return true
	}
	return false
}

func (vv *ViewerView) SemanticNode(ctx *vtui.SemanticContext) map[string]any {
	vv.ensureTextLayoutSettings()
	if vv.SemanticLayoutRevision == 0 {
		vv.SemanticLayoutRevision = 1
	}
	window := semantic.SemanticSurfaceWindow{ViewportRows: vv.viewportHeight()}
	layoutReady := !semantic.NativeDocumentLayoutPending(vv.NativeViewportRevision)
	if layoutReady && vv.SemanticNeedsReflow {
		resolved, ready := vv.TopOffset&^int64(15), true
		if vv.DecodeMode {
			resolved, ready = vv.TopOffset, true
		} else if !vv.HexMode {
			resolved, ready = vv.semanticResolveTextWindowOffset(vv.TopOffset)
		}
		if ready {
			vv.TopOffset, vv.SemanticNeedsReflow = resolved, false
		} else {
			layoutReady = false
		}
	}
	if layoutReady {
		if vv.SemanticPendingScroll {
			generation := vv.SemanticPendingGeneration
			if generation <= vv.semanticWindowGeneration || generation != vv.semanticWindowRequestGeneration {
				vv.SemanticPendingScroll = false
			} else {
				resolved, ready := vv.semanticPendingOffset&^int64(15), true
				if pendingOffset, resumable := vv.pendingConstructionOffset(); resumable {
					resolved = pendingOffset
				} else if vv.DecodeMode {
					resolved, ready = vv.semanticPendingOffset, true
				} else if !vv.HexMode {
					resolved, ready = vv.semanticResolveTextWindowOffset(vv.semanticPendingOffset)
				}
				if ready {
					previous := vv.TopOffset
					vv.TopOffset = resolved
					window = vv.semanticWindow()
					if window.Ready {
						vv.semanticWindowGeneration = generation
						vv.SemanticPendingScroll = false
						vv.SemanticPendingGeneration = 0
					} else {
						vv.TopOffset = previous
					}
				}
			}
		} else {
			window = vv.semanticWindow()
		}
	}
	if window.Ready && window.LoadError == "" {
		vv.semanticLoadError = ""
	}
	if window.LoadError == "" {
		window.LoadError = vv.semanticLoadError
	}
	if window.Ready {
		vv.LineOffsets = vv.LineOffsets[:0]
		for i := window.ViewportRow; i < min(len(window.Rows), window.ViewportRow+window.ViewportRows); i++ {
			vv.LineOffsets = append(vv.LineOffsets, window.Rows[i].Offset)
		}
		vv.EofVisible = vv.TopOffset+window.ViewportSpan >= vv.Backend.Size()
		vv.lastKnownSize = vv.Backend.Size()
	}
	width := vv.semanticContentWidth()
	for i := range window.Rows {
		window.Rows[i].Text = ""
	}
	visibleEnd := min(window.ViewportRow+window.ViewportRows, len(window.Rows))
	visibleRows := window.Rows
	if window.ViewportRow >= 0 && window.ViewportRow <= visibleEnd {
		visibleRows = window.Rows[window.ViewportRow:visibleEnd]
	}
	mode := "text"
	if vv.DecodeMode {
		mode = "decode"
	} else if vv.HexMode {
		mode = "hex"
	}
	topBarLeft, topBarRight := SemanticTopBarStrings(vv.TopBar)

	surface := extui.SurfaceModel{
		ID:                      vtui.SemanticID(vv),
		Kind:                    "viewer",
		DefaultBackground:       semantic.SemanticAttrColor(vtui.Palette[theme.ColViewerText], false),
		Title:                   vv.GetTitle(),
		Path:                    vv.Path,
		LocalPath:               semantic.SemanticLocalPath(vv.VFS, vv.Path),
		BaseName:                semantic.SemanticBaseName(vv.VFS, vv.Path),
		Mode:                    mode,
		TopBarLeft:              topBarLeft,
		TopBarRight:             topBarRight,
		IconColor:               theme.SemanticFileIconColor(semantic.SemanticBaseName(vv.VFS, vv.Path)),
		HexMode:                 vv.HexMode,
		DecodeMode:              vv.DecodeMode,
		WrapMode:                vv.WrapMode,
		Busy:                    vv.Busy,
		TopOffset:               vv.TopOffset,
		Size:                    vv.Backend.Size(),
		DocumentKey:             vtui.SemanticID(vv),
		ScrollAction:            "viewer.scrollWindow",
		ScrollUnit:              "bytes",
		WindowStart:             window.Start,
		WindowEnd:               window.End,
		ViewportStart:           vv.TopOffset,
		ViewportSpan:            window.ViewportSpan,
		ContentExtent:           vv.Backend.Size(),
		ContentExtentKnown:      true,
		ViewportRow:             window.ViewportRow,
		ViewportRows:            window.ViewportRows,
		WindowGeneration:        vv.semanticWindowGeneration,
		WindowRequestGeneration: vv.semanticWindowRequestGeneration,
		ViewportColumns:         width,
		GeometryRevision:        vv.NativeViewportRevision,
		LayoutRevision:          vv.SemanticLayoutRevision,
		LayoutPending:           !window.Ready,
		LoadError:               window.LoadError,
		Rows:                    visibleRows,
		WindowRows:              window.Rows,
	}
	navtrace.TraceDocumentWindow(&surface)
	return surface.ToMap()
}

func (vv *ViewerView) semanticContentWidth() int {
	return vv.viewportWidth()
}

func (vv *ViewerView) semanticResolveTextWindowOffset(offset int64) (int64, bool) {
	offset = vv.clampTextScrollOffset(offset)
	if vv.Backend == nil || offset <= 0 {
		return 0, true
	}
	if !vv.WrapMode {
		resolved, ready, yielded := vv.Backend.tryFindLineStartBounded(offset, viewerProjectionWorkBytes)
		if yielded {
			vv.scheduleProjectionContinuation()
		}
		if !ready {
			if err := vv.Backend.LastReadError(); err != nil {
				vv.semanticLoadError = err.Error()
			}
		} else {
			vv.semanticLoadError = ""
		}
		return resolved, ready
	}
	width := vv.semanticContentWidth()
	if width <= 0 {
		return vv.Backend.TryFindLineStart(offset)
	}
	return vv.semanticWrappedRowStart(offset, width)
}

func (vv *ViewerView) clampTextScrollOffset(offset int64) int64 {
	if vv.Backend == nil || offset <= 0 {
		return 0
	}
	size := vv.Backend.Size()
	if size <= 0 {
		return 0
	}
	if offset >= size {
		return size - 1
	}
	return offset
}

func (vv *ViewerView) semanticWrappedRowStart(offset int64, width int) (int64, bool) {
	vv.ensureTextLayoutSettings()
	seek := &vv.SemanticWrapSeek
	historyLimit := semantic.SemanticWindowBufferRows(max(1, vv.viewportHeight())) + 1
	if seek.width != width || len(seek.history) != historyLimit ||
		(!seek.active && !seek.ready) ||
		(seek.target != offset && (!seek.ready || offset < seek.resolved)) {
		seek.reset(offset, width, historyLimit)
	} else if seek.ready && seek.target == offset {
		return seek.resolved, true
	} else if seek.ready && offset == seek.resolved {
		seek.target = offset
		return seek.resolved, true
	} else if seek.ready && offset > seek.resolved {
		// A native scroll normally advances within the same large logical line.
		// Continue at the previously resolved fragment rather than searching and
		// scanning forward from that line's first byte again.
		seek.target = offset
		seek.active = true
		seek.ready = false
		seek.curr = seek.resolved
		seek.currColumn = seek.resolvedColumn
		seek.lineStartReady = true
	}

	if !seek.lineStartReady {
		lineStart, ready, yielded := vv.Backend.tryFindLineStartBounded(offset, viewerProjectionWorkBytes)
		if !ready {
			if yielded {
				vv.scheduleProjectionContinuation()
			}
			if err := vv.Backend.LastReadError(); err != nil {
				vv.semanticLoadError = err.Error()
			}
			return offset, false
		}
		seek.curr = lineStart
		seek.currColumn = 0
		seek.lineStartReady = true
		seek.appendHistory(lineStart, 0)
		if lineStart >= offset {
			seek.finish(lineStart)
			return lineStart, true
		}
	}
	if seek.curr >= offset {
		seek.finish(seek.curr)
		return seek.curr, true
	}

	started, processed, iterations := time.Now(), 0, 0
	for seek.curr < offset && seek.curr < vv.Backend.Size() {
		data, err := vv.Backend.ReadAt(seek.curr, max(4, width*4))
		if err == piecetable.ErrLoading {
			return offset, false
		}
		if err != nil || len(data) == 0 {
			vv.semanticLoadError = "unexpected end of source while resolving wrapped row"
			if err != nil {
				vv.semanticLoadError = err.Error()
			}
			return offset, false
		}
		vv.semanticLoadError = ""
		scan := scanViewerText(data, width, true, seek.currColumn, 0, false)
		if scan.lineLen <= 0 {
			vv.semanticLoadError = "wrapped row resolver made no source progress"
			return offset, false
		}
		nextOffset := min(seek.curr+int64(scan.lineLen), vv.Backend.Size())
		if offset < nextOffset {
			seek.finish(seek.curr)
			return seek.curr, true
		}
		if offset == nextOffset {
			seek.currColumn = scan.nextColumn
			seek.appendHistory(nextOffset, scan.nextColumn)
			seek.finish(nextOffset)
			return nextOffset, true
		}
		if nextOffset <= seek.curr {
			vv.semanticLoadError = "wrapped row resolver made no source progress"
			return offset, false
		}
		seek.curr = nextOffset
		seek.currColumn = scan.nextColumn
		seek.appendHistory(nextOffset, scan.nextColumn)
		processed += scan.lineLen
		iterations++
		if processed >= viewerProjectionWorkBytes || (iterations%32 == 0 && time.Since(started) >= 2*time.Millisecond) {
			vv.scheduleProjectionContinuation()
			return offset, false
		}
	}
	seek.finish(seek.curr)
	return seek.curr, true
}

func (seek *SemanticWrapSeekState) reset(target int64, width, historyLimit int) {
	history := seek.history
	if len(history) != historyLimit {
		history = make([]viewerRowPosition, historyLimit)
	}
	*seek = SemanticWrapSeekState{
		active:  true,
		target:  target,
		width:   width,
		history: history,
	}
}

func (seek *SemanticWrapSeekState) appendHistory(offset int64, column int) {
	limit := len(seek.history)
	if limit == 0 {
		return
	}
	if seek.historyCount > 0 {
		last := (seek.historyHead + seek.historyCount - 1) % limit
		if seek.history[last].offset == offset {
			seek.history[last].column = column
			return
		}
	}
	if seek.historyCount < limit {
		index := (seek.historyHead + seek.historyCount) % limit
		seek.history[index] = viewerRowPosition{offset: offset, column: column}
		seek.historyCount++
		return
	}
	seek.history[seek.historyHead] = viewerRowPosition{offset: offset, column: column}
	seek.historyHead = (seek.historyHead + 1) % limit
}

func (seek *SemanticWrapSeekState) finish(offset int64) {
	seek.curr = offset
	seek.resolved = offset
	seek.resolvedColumn = seek.currColumn
	seek.active = false
	seek.ready = true
	seek.appendHistory(offset, seek.currColumn)
}

func (seek *SemanticWrapSeekState) previousHistoryOffset(offset int64, width int) (int64, bool) {
	if seek.width != width || seek.historyCount < 2 || len(seek.history) == 0 {
		return 0, false
	}
	for logicalIndex := seek.historyCount - 1; logicalIndex > 0; logicalIndex-- {
		index := (seek.historyHead + logicalIndex) % len(seek.history)
		if seek.history[index].offset != offset {
			continue
		}
		previous := (seek.historyHead + logicalIndex - 1) % len(seek.history)
		return seek.history[previous].offset, true
	}
	return 0, false
}

func (seek *SemanticWrapSeekState) historyColumn(offset int64, width int) (int, bool) {
	if seek.width != width {
		return 0, false
	}
	if seek.ready && seek.resolved == offset {
		return seek.resolvedColumn, true
	}
	for i := seek.historyCount - 1; i >= 0; i-- {
		position := seek.history[(seek.historyHead+i)%len(seek.history)]
		if position.offset == offset {
			return position.column, true
		}
	}
	return 0, false
}

func (vv *ViewerView) semanticPreviousTextRowStart(offset int64, width int) (int64, bool) {
	position, ready := vv.semanticPreviousTextRowPosition(offset, width)
	return position.offset, ready
}

func (vv *ViewerView) semanticPreviousTextRowPosition(offset int64, width int) (viewerRowPosition, bool) {
	if offset <= 0 {
		return viewerRowPosition{}, true
	}
	if !vv.WrapMode || width <= 0 {
		start, ready, yielded := vv.Backend.tryFindLineStartBounded(offset-1, viewerProjectionWorkBytes)
		if yielded {
			vv.scheduleProjectionContinuation()
		}
		if !ready {
			if err := vv.Backend.LastReadError(); err != nil {
				vv.semanticLoadError = err.Error()
			}
		}
		return viewerRowPosition{offset: start}, ready
	}
	if previous, ok := vv.SemanticWrapSeek.previousHistoryOffset(offset, width); ok {
		column, _ := vv.SemanticWrapSeek.historyColumn(previous, width)
		return viewerRowPosition{offset: previous, column: column}, true
	}
	// The byte before a fragment start belongs to its preceding fragment.
	// Reuse the resumable resolver: a physical line can span many source read
	// windows, so a fresh forward scan on every callback would oscillate.
	previous, ready := vv.semanticWrappedRowStart(offset-1, width)
	return viewerRowPosition{offset: previous, column: vv.SemanticWrapSeek.resolvedColumn}, ready
}

func (vv *ViewerView) semanticWindow() semantic.SemanticSurfaceWindow {
	window := semantic.SemanticSurfaceWindow{Ready: true}
	if vv.Backend == nil {
		return window
	}
	width, height := vv.semanticContentWidth(), vv.viewportHeight()
	window.ViewportRows = height
	if width <= 0 || height <= 0 || vv.Busy {
		window.Ready = false
		return window
	}
	build, ready, err := vv.constructWindow(width, height, semantic.SemanticWindowBufferRows(height), &vv.SemanticProjection)
	window.Start, window.End, window.ViewportRow, window.Ready = build.start, build.current, build.viewportRow, ready
	if err != nil && !viewerProjectionLoading(err) {
		window.LoadError = err.Error()
	}
	// Unfinished source spans are never exported, and successful publication
	// owns this array independently from the discarded construction state.
	for _, constructed := range build.rows {
		projection := constructed.projection
		row := extui.TextRowModel{Index: len(window.Rows), Offset: constructed.start, EndOffset: projection.end,
			Text: projection.text, Runs: semantic.RunsFromCells(projection.cells)}
		row.ContentKey = extui.TextRowContentKey(row)
		window.Rows = append(window.Rows, row)
	}
	end := min(window.ViewportRow+height, len(window.Rows))
	if build.foundViewport && end > window.ViewportRow {
		window.ViewportSpan = window.Rows[end-1].EndOffset - vv.TopOffset
	}
	return window
}

func semanticHexLine(offset int64, data []byte) string {
	const digits = "0123456789ABCDEF"
	var line strings.Builder
	line.Grow(80)
	fmt.Fprintf(&line, "%010X: ", offset)
	for i := 0; i < 16; i++ {
		if i < len(data) {
			line.WriteByte(digits[data[i]>>4])
			line.WriteByte(digits[data[i]&15])
			line.WriteByte(' ')
		} else {
			line.WriteString("   ")
		}
		if i == 7 {
			line.WriteByte(' ')
		}
	}
	line.WriteString(" | ")
	for _, value := range data[:min(16, len(data))] {
		if value < 32 || value > 126 {
			value = '.'
		}
		line.WriteByte(value)
	}
	return line.String()
}

func semanticViewerLineLen(data []byte, width int, wrap bool) (lineLen int, textLen int, foundNewline bool) {
	scan := scanViewerText(data, width, wrap, 0, 0, false)
	return scan.lineLen, scan.textLen, scan.newline
}

func SemanticTopBarStrings(topBar *TopBar) (left, right string) {
	if topBar == nil {
		return "", ""
	}
	if topBar.GetLeft != nil {
		left = topBar.GetLeft()
	}
	if topBar.GetRight != nil {
		right = topBar.GetRight()
	}
	return left, right
}
