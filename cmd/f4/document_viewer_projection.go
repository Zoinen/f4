package main

import (
	"bytes"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
	"golang.org/x/arch/x86/x86asm"
)

// A row is a projection of one ready source span, not a retained rendering.
// Both native export and the console painter consume this same operation.
type viewerRowProjection struct {
	end        int64
	text       string
	cells      []vtui.CharInfo
	nextColumn int
}

// These are unfinished operations, not retained document windows. A completed
// window transfers ownership to its caller and releases the builder. In
// particular, an expensive source may replace its small read buffer while a
// long row is being scanned without making the next UI turn restart that row.
type viewerRowConstruction struct {
	row         viewerRowProjection
	initialized bool
}

type viewerWindowConstructionKey struct {
	backend                   *ViewerBackend
	top, size                 int64
	width, height, overscan   int
	tabSize                   int
	layout, geometry, request uint64
	hex, decode, wrap         bool
	disasmMode                int
	textAttr, arrowAttr       uint64
}

type viewerConstructedRow struct {
	start      int64
	projection viewerRowProjection
}

type viewerWindowConstruction struct {
	key                                     viewerWindowConstructionKey
	start, current                          int64
	column, preceding, viewportRow          int
	originReady, initialized, foundViewport bool
	rows                                    []viewerConstructedRow
	row                                     *viewerRowConstruction
}

func (vv *ViewerView) projectRow(offset int64, width int) (viewerRowProjection, error) {
	column, err := vv.viewerRowOrigin(offset, width)
	if err != nil {
		return viewerRowProjection{end: offset}, err
	}
	return vv.projectRowAt(offset, width, column)
}

func (vv *ViewerView) projectRowAt(offset int64, width, column int) (viewerRowProjection, error) {
	var pending *viewerRowConstruction
	return vv.projectRowProgress(offset, width, column, &pending)
}

func (vv *ViewerView) projectRowProgress(offset int64, width, column int, pending **viewerRowConstruction) (viewerRowProjection, error) {
	row := viewerRowProjection{end: offset}
	if width <= 0 || offset >= vv.backend.Size() {
		return row, io.EOF
	}
	attr := vtui.Palette[ColViewerText]
	if vv.HexMode {
		data, err := vv.backend.ReadAt(offset, 16)
		if err != nil && (err != io.EOF || len(data) == 0) {
			return row, err
		}
		if len(data) < int(min(int64(16), vv.backend.Size()-offset)) {
			return row, io.ErrUnexpectedEOF
		}
		row.end = offset + int64(len(data))
		row.text = semanticHexLine(offset, data)
		cells := make([]vtui.CharInfo, width)
		for i := range cells {
			cells[i] = vtui.CharInfo{Char: ' ', Attributes: attr}
		}
		put := func(at int, s string, color uint64) {
			for _, r := range s {
				if at >= width {
					break
				}
				cells[at] = vtui.CharInfo{Char: uint64(r), Attributes: color}
				at++
			}
		}
		put(0, fmt.Sprintf("%010X: ", offset), vtui.Palette[ColViewerArrows])
		const hexDigits = "0123456789ABCDEF"
		for i := 0; i < 16; i++ {
			if i < len(data) {
				at := 12 + i*3 + i/8
				if at < width {
					cells[at].Char = uint64(hexDigits[data[i]>>4])
				}
				if at+1 < width {
					cells[at+1].Char = uint64(hexDigits[data[i]&15])
				}
			}
		}
		put(62, "│ ", attr)
		for i, value := range data {
			if value < 32 || value > 126 {
				value = '.'
			}
			if 64+i < width {
				cells[64+i].Char = uint64(value)
			}
		}
		row.cells = cells
		return row, nil
	}
	if vv.DecodeMode {
		maxLen := int(min(int64(15), vv.backend.Size()-offset))
		if maxLen <= 0 {
			return row, io.EOF
		}
		data, err := vv.backend.ReadAt(offset, maxLen)
		if err != nil && (err != io.EOF || len(data) == 0) {
			return row, err
		}
		if len(data) == 0 {
			return row, io.ErrUnexpectedEOF
		}
		mode := vv.effectiveDisasmMode()
		instLen := 1
		asmStr := fmt.Sprintf("db 0x%02X", data[0])
		inst, decErr := x86asm.Decode(data, mode)
		if decErr == nil {
			instLen = inst.Len
			asmStr = x86asm.IntelSyntax(inst, uint64(offset), nil)
		}
		row.end = offset + int64(instLen)
		hexStr := ""
		for i := 0; i < instLen; i++ {
			hexStr += fmt.Sprintf("%02X ", data[i])
		}
		row.text = fmt.Sprintf("%010X: %-24s%s", offset, hexStr, asmStr)
		cells := make([]vtui.CharInfo, width)
		for i := range cells {
			cells[i] = vtui.CharInfo{Char: ' ', Attributes: attr}
		}
		put := func(at int, s string, color uint64) {
			for _, r := range s {
				if at >= width {
					break
				}
				cells[at] = vtui.CharInfo{Char: uint64(r), Attributes: color}
				at++
			}
		}
		put(0, fmt.Sprintf("%010X: ", offset), vtui.Palette[ColViewerArrows])
		put(12, fmt.Sprintf("%-24s", hexStr), attr)
		put(38, asmStr, attr)
		row.cells = cells
		return row, nil
	}
	if *pending == nil {
		*pending = &viewerRowConstruction{}
	}
	progress := *pending
	if !progress.initialized {
		data, err := vv.backend.ReadAt(offset, max(4, width*4))
		if err != nil && (err != io.EOF || len(data) == 0) {
			return row, err
		}
		if len(data) == 0 {
			return row, io.ErrUnexpectedEOF
		}
		scan := scanViewerText(data, width, vv.WrapMode, column, attr, true)
		row.end = offset + int64(scan.lineLen)
		row.text = string(data[:scan.textLen])
		row.cells = scan.cells
		row.nextColumn = scan.nextColumn
		progress.row, progress.initialized = row, true
		if vv.WrapMode || scan.newline {
			for len(row.cells) < width {
				row.cells = append(row.cells, vtui.CharInfo{Char: ' ', Attributes: attr})
			}
			*pending = nil
			return row, nil
		}
	} else {
		row = progress.row
	}
	if !vv.WrapMode {
		started, processed, iterations := time.Now(), 0, 0
		for row.end < vv.backend.Size() {
			chunk, readErr := vv.backend.ReadAt(row.end, 1024)
			if readErr != nil && (readErr != io.EOF || len(chunk) == 0) {
				progress.row = row
				return row, readErr
			}
			if len(chunk) == 0 {
				return row, io.ErrUnexpectedEOF
			}
			if end := bytes.IndexByte(chunk, '\n'); end >= 0 {
				row.end += int64(end + 1)
				row.nextColumn = 0
				break
			}
			row.end += int64(len(chunk))
			processed += len(chunk)
			iterations++
			if row.end < vv.backend.Size() && (processed >= viewerProjectionWorkBytes ||
				(iterations%32 == 0 && time.Since(started) >= 2*time.Millisecond)) {
				progress.row = row
				vv.scheduleProjectionContinuation()
				return row, piecetable.ErrLoading
			}
		}
	}
	for len(row.cells) < width {
		row.cells = append(row.cells, vtui.CharInfo{Char: ' ', Attributes: attr})
	}
	*pending = nil
	return row, nil
}

func (vv *ViewerView) constructionKey(top int64, width, height, overscan int) viewerWindowConstructionKey {
	return viewerWindowConstructionKey{backend: vv.backend, top: top,
		size: vv.backend.Size(), width: width, height: height, overscan: overscan,
		tabSize: effectiveViewerTabSize(),
		layout:  vv.semanticLayoutRevision, geometry: vv.nativeViewportRevision,
		request: vv.semanticWindowRequestGeneration,
		hex:     vv.HexMode, decode: vv.DecodeMode, wrap: vv.WrapMode,
		disasmMode: vv.effectiveDisasmMode(),
		textAttr:   vtui.Palette[ColViewerText], arrowAttr: vtui.Palette[ColViewerArrows]}
}

func effectiveViewerTabSize() int {
	if AppConfig.EditorTabSize <= 0 {
		return 8
	}
	return AppConfig.EditorTabSize
}

// Settings can change while an expensive read is pending. Tab stops belong
// to the source-to-cell mapping just as much as width and wrap mode do.
func (vv *ViewerView) ensureTextLayoutSettings() {
	tabSize := effectiveViewerTabSize()
	if vv.semanticLayoutRevision == 0 {
		vv.semanticLayoutRevision = 1
	}
	if vv.layoutTabSize == tabSize {
		return
	}
	previous := vv.layoutTabSize
	vv.layoutTabSize = tabSize
	if previous == 0 {
		return
	}
	vv.semanticLayoutRevision++
	vv.semanticNeedsReflow = true
	vv.semanticWrapSeek = semanticWrapSeekState{}
	vv.semanticPendingScroll, vv.semanticPendingGeneration = false, 0
	vv.semanticProjection, vv.consoleProjection = nil, nil
	vv.projectionContinuationPending = false
	vv.lineOffsets = nil
}

const viewerProjectionWorkBytes = 64 * 1024

func (vv *ViewerView) currentProjectionIntentKey() viewerWindowConstructionKey {
	top := vv.TopOffset
	if vv.semanticPendingScroll {
		top = vv.semanticPendingOffset
	}
	height := vv.viewportHeight()
	return vv.constructionKey(top, vv.semanticContentWidth(), height, semanticWindowBufferRows(height))
}

// A work-bound yield is continued through the UI queue, with no timer or
// delay. A close, reflow or newer destination makes its redraw obsolete.
func (vv *ViewerView) scheduleProjectionContinuation() {
	if vtui.FrameManager == nil || vv.IsDone() {
		return
	}
	key := vv.currentProjectionIntentKey()
	if vv.projectionContinuationPending && vv.projectionContinuationKey == key {
		return
	}
	vv.projectionContinuationPending, vv.projectionContinuationKey = true, key
	vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
		if !vv.projectionContinuationPending || vv.projectionContinuationKey != key {
			return false
		}
		vv.projectionContinuationPending = false
		return !vv.IsDone() && vv.currentProjectionIntentKey() == key
	})
}

func (vv *ViewerView) pendingConstructionOffset() (int64, bool) {
	build := vv.semanticProjection
	if build == nil || vv.semanticPendingGeneration != build.key.request {
		return 0, false
	}
	height := vv.viewportHeight()
	key := vv.constructionKey(build.key.top, vv.semanticContentWidth(), height, semanticWindowBufferRows(height))
	return build.key.top, build.key == key
}

func (vv *ViewerView) constructWindow(width, height, overscan int, pending **viewerWindowConstruction) (*viewerWindowConstruction, bool, error) {
	vv.ensureTextLayoutSettings()
	key := vv.constructionKey(vv.TopOffset, width, height, overscan)
	if *pending == nil || (*pending).key != key {
		*pending = &viewerWindowConstruction{key: key, start: key.top}
	}
	build := *pending
	if !build.originReady {
		if key.hex {
			build.start = max(int64(0), key.top-int64(overscan*16)) &^ int64(15)
		} else if key.decode {
			build.start = key.top
			var precedingOffsets []int64
			curr := key.top
			mode := vv.effectiveDisasmMode()
			for len(precedingOffsets) < overscan && curr > 0 {
				prev := vv.findPrecedingInstructionOffset(curr)
				if prev >= curr {
					break
				}
				data, err := vv.backend.ReadAt(prev, 15)
				if err != nil && len(data) == 0 {
					break
				}
				inst, decErr := x86asm.Decode(data, mode)
				instLen := 1
				if decErr == nil {
					instLen = inst.Len
				}
				if prev+int64(instLen) != curr {
					break
				}
				precedingOffsets = append(precedingOffsets, prev)
				curr = prev
			}
			if len(precedingOffsets) > 0 {
				build.start = precedingOffsets[len(precedingOffsets)-1]
				build.preceding = len(precedingOffsets)
			}
		} else {
			column, err := vv.viewerRowOrigin(build.start, width)
			if err != nil {
				return build, false, err
			}
			build.column = column
		}
		build.originReady = true
	}
	if !build.initialized {
		for !key.hex && !key.decode && build.preceding < overscan && build.start > 0 {
			previous, ready := vv.semanticPreviousTextRowPosition(build.start, width)
			if !ready {
				if vv.semanticLoadError != "" {
					return build, false, fmt.Errorf("%s", vv.semanticLoadError)
				}
				return build, false, piecetable.ErrLoading
			}
			if previous.offset >= build.start {
				break
			}
			build.start, build.column = previous.offset, previous.column
			build.preceding++
		}
		build.current, build.initialized = build.start, true
	}
	for len(build.rows) < height+2*overscan && build.current < key.size {
		row, err := vv.projectRowProgress(build.current, width, build.column, &build.row)
		if err != nil {
			if build.foundViewport && len(build.rows)-build.viewportRow >= height {
				vv.projectionContinuationPending = false
				vv.backend.suppressOptionalReadNotification()
				*pending = nil
				return build, true, nil
			}
			return build, false, err
		}
		if row.end <= build.current {
			return build, false, io.ErrNoProgress
		}
		if !build.foundViewport && (build.current == key.top || (key.decode && build.current >= key.top)) {
			build.viewportRow, build.foundViewport = len(build.rows), true
		}
		build.rows = append(build.rows, viewerConstructedRow{start: build.current, projection: row})
		build.current, build.column = row.end, row.nextColumn
	}
	ready := key.size == 0 || (build.foundViewport &&
		(len(build.rows)-build.viewportRow >= height || build.current >= key.size))
	if ready {
		*pending = nil
	}
	return build, ready, nil
}

type viewerTextScan struct {
	lineLen, textLen, nextColumn int
	newline                      bool
	cells                        []vtui.CharInfo
}

// scanViewerText is the single source scan for byte boundaries, tab stops,
// navigation and painted cells. origin is carried across soft wraps and reset
// only by a real newline; emit=false resolves navigation without allocating.
func scanViewerText(data []byte, width int, wrap bool, origin int, attr uint64, emit bool) viewerTextScan {
	result := viewerTextScan{nextColumn: origin}
	if width <= 0 {
		return result
	}
	if emit {
		result.cells = make([]vtui.CharInfo, 0, width)
	}
	tab := effectiveViewerTabSize()
	visual := 0
	for result.lineLen < len(data) {
		r, size := utf8.DecodeRune(data[result.lineLen:])
		if r == '\n' {
			result.lineLen += size
			result.newline, result.nextColumn = true, 0
			return result
		}
		if r == '\r' {
			result.lineLen += size
			continue
		}
		display, count := vtui.SanitizeRune(r)
		if r == '\t' {
			count, display = tab-(origin+visual)%tab, ' '
		} else if r < 32 || r == 127 {
			display = ' '
		}
		count = max(1, count)
		if wrap && visual > 0 && visual+count > width {
			break
		}
		if emit {
			character := uint64(display)
			for i := 0; i < count && len(result.cells) < width; i++ {
				result.cells = append(result.cells, vtui.CharInfo{Char: character, Attributes: attr})
				if r != '\t' {
					character = vtui.WideCharFiller
				}
			}
		}
		visual += count
		result.lineLen += size
		if wrap || visual <= width {
			result.textLen = result.lineLen
		}
		result.nextColumn = origin + visual
	}
	return result
}

// viewerRowOrigin resolves the first displayed fragment once. Subsequent rows
// carry nextColumn directly, so their tab origin never requires prefix rereads.
func (vv *ViewerView) viewerRowOrigin(offset int64, width int) (int, error) {
	vv.ensureTextLayoutSettings()
	if !vv.WrapMode || vv.HexMode || vv.DecodeMode || offset <= 0 {
		return 0, nil
	}
	if column, ok := vv.semanticWrapSeek.historyColumn(offset, width); ok {
		return column, nil
	}
	resolved, ready := vv.semanticWrappedRowStart(offset, width)
	if !ready {
		if vv.semanticLoadError != "" {
			return 0, fmt.Errorf("%s", vv.semanticLoadError)
		}
		return 0, piecetable.ErrLoading
	}
	if resolved == offset {
		return vv.semanticWrapSeek.resolvedColumn, nil
	}
	// A legacy console cursor can point inside a fragment. Count only that
	// bounded prefix; never reinterpret it as a fresh logical line.
	data, err := vv.backend.ReadAt(resolved, int(offset-resolved))
	if err != nil {
		return 0, err
	}
	scan := scanViewerText(data, max(1, len(data)*8), false, vv.semanticWrapSeek.resolvedColumn, 0, false)
	return scan.nextColumn, nil
}

func viewerProjectionLoading(err error) bool { return err == piecetable.ErrLoading }
