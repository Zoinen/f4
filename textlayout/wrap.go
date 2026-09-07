package textlayout

import (
	"github.com/mattn/go-runewidth"
	"io"
	"sort"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// LineFragment описывает один визуальный кусок логической строки после свертки.
type LineFragment struct {
	LogicalLineIdx  int // Номер оригинальной строки (до \n)
	ByteOffsetStart int // Смещение начала фрагмента (от начала всего файла/буфера)
	ByteOffsetEnd   int // Смещение конца фрагмента
	VisualWidth     int // Ширина фрагмента в колонках терминала (учитывая CJK)
	// VisualColumnStart is the logical-line column of the first cell. Tabs
	// continue their logical tab stops across a soft wrap.
	VisualColumnStart int
	RuneStart         int // Logical-line rune index, for syntax attributes.
	Loading           bool
}

// A line's existing layout entry also owns its unfinished scan. No source
// content is retained: the continuation is just the coordinates of the one
// unfinished row. Completed lines need only their fragments.
type lineLayout struct {
	fragments []LineFragment
	pending   *lineScan
}

type lineScan struct {
	offset, start, column, runeStart int
	width, runes                     int
	spaceEnd, spaceWidth, spaceRunes int
	stopped                          bool // A newline was reached before the index.
}

func (l *lineLayout) complete() bool { return l.fragments != nil && l.pending == nil }

const layoutReadBytes = 4096

// WrapEngine отвечает за вычисление визуальной разметки текста.
type WrapEngine struct {
	pt            *piecetable.PieceTable
	li            *piecetable.LineIndex
	wrapWidth     int
	wordWrap      bool
	fragmentCache []lineLayout
	tabSize       int

	// rowOffsets[i] хранит общее количество визуальных строк во всех
	// логических строках ПЕРЕД строкой i.
	rowOffsets []int
	totalRows  int
	validUntil int // Index of the last logical line with a valid calculated row offset

	tmpBuf        []byte // Reusable buffer for avoiding allocations
	lastReadError error
}

// growingCacheCapacity keeps the two line-indexed caches amortized O(1) when
// a live terminal appends lines. Exact-size reallocations here used to copy an
// array proportional to the complete scrollback on every semantic frame.
func growingCacheCapacity(current, required int) int {
	if current >= required {
		return current
	}
	next := current
	if next < 64 {
		next = 64
	}
	for next < required {
		next += max(64, next/2)
	}
	return next
}

func (we *WrapEngine) resizeFragmentCache(lineCount int) {
	oldLength := len(we.fragmentCache)
	if oldLength == lineCount {
		return
	}
	if lineCount <= cap(we.fragmentCache) {
		if lineCount < oldLength {
			clear(we.fragmentCache[lineCount:])
		}
		we.fragmentCache = we.fragmentCache[:lineCount]
		if lineCount > oldLength {
			clear(we.fragmentCache[oldLength:])
		}
		return
	}
	grown := make([]lineLayout, lineCount,
		growingCacheCapacity(cap(we.fragmentCache), lineCount))
	copy(grown, we.fragmentCache)
	we.fragmentCache = grown
}

func (we *WrapEngine) resizeRowOffsets(lineCount int) {
	oldLength := len(we.rowOffsets)
	if oldLength == lineCount {
		return
	}
	if lineCount <= cap(we.rowOffsets) {
		we.rowOffsets = we.rowOffsets[:lineCount]
		if lineCount > oldLength {
			clear(we.rowOffsets[oldLength:])
		}
		return
	}
	grown := make([]int, lineCount,
		growingCacheCapacity(cap(we.rowOffsets), lineCount))
	copy(grown, we.rowOffsets)
	we.rowOffsets = grown
}

func NewWrapEngine(pt *piecetable.PieceTable, li *piecetable.LineIndex) *WrapEngine {
	return &WrapEngine{
		pt:            pt,
		li:            li,
		wrapWidth:     80,
		wordWrap:      true,
		fragmentCache: nil,
		tabSize:       8,
		validUntil:    -1,
	}
}

type visualCluster struct {
	text       string
	width      int
	byteStart  int
	byteEnd    int
	logicalPos int
	logicalEnd int
}

// noWrapLayout caches what a cursor-column lookup needs for one unwrapped
// logical line, which otherwise rebuilds the complete long line on every key
// press. Only the cluster ends and their prefix widths are kept: retaining the
// line text and the clusters themselves cost roughly seventy times the size of
// the text, so scrolling through a file was enough to exhaust memory.
type noWrapLayout struct {
	fragments    []LineFragment
	clusterEnds  []int
	prefixWidths []int
	hasRTL       bool
}

// noWrapCacheBudget caps the cluster entries kept across all cached lines. At
// eight bytes per entry this holds the cache to about 8 MB; going over drops it
// wholesale, which costs one rescan of the lines still on screen.
const noWrapCacheBudget = 1 << 20

// logicalTextClusters keeps zoin-bot's grapheme boundaries in document order.
func logicalTextClusters(text string) []visualCluster {
	base := VisualClusters(text)
	logical := make([]visualCluster, 0, len(base))
	for _, cluster := range base {
		logical = append(logical, visualCluster{
			text:       cluster.Text,
			width:      cluster.Width,
			byteStart:  cluster.Start,
			byteEnd:    cluster.End,
			logicalPos: cluster.RuneStart,
			logicalEnd: cluster.RuneEnd,
		})
	}
	return logical
}

// layoutLine runs the bidi algorithm (UAX #9) over the editor's own cluster
// boundaries: the virama joined clusters of VisualClusters, not the UAX #29
// ones vtui's string helpers would segment on their own, so that wrapping,
// painting, hit testing and caret movement all agree on one set of units.
func layoutLine(text string, logical []visualCluster) vtui.BidiLayout {
	spans := make([]vtui.ClusterSpan, len(logical))
	for i, cluster := range logical {
		spans[i] = vtui.ClusterSpan{Start: cluster.byteStart, End: cluster.byteEnd}
	}
	return vtui.LayoutBidi(text, spans, vtui.DefaultBidiParagraph)
}

// visualClusters returns the clusters of text in the order they are drawn,
// with mirrored glyphs substituted where a cluster reads right to left.
func visualClusters(text string) []visualCluster {
	logical := logicalTextClusters(text)
	if vtui.DefaultBidiMode != vtui.BidiFull || !vtui.HasRTL(text) {
		return logical
	}
	layout := layoutLine(text, logical)
	visual := make([]visualCluster, 0, len(logical))
	for _, index := range layout.VisualToLogical {
		cluster := logical[index]
		cluster.text = layout.Text(index, cluster.text)
		visual = append(visual, cluster)
	}
	return visual
}

// VisualClustersInVisualOrder returns grapheme clusters in terminal order.
// zoin-bot uses this shared mapping for editor painting and hit testing.
func VisualClustersInVisualOrder(text string) []VisualCluster {
	clusters := visualClusters(text)
	result := make([]VisualCluster, 0, len(clusters))
	for _, cluster := range clusters {
		result = append(result, VisualCluster{
			Text:      cluster.text,
			Width:     cluster.width,
			Start:     cluster.byteStart,
			End:       cluster.byteEnd,
			RuneStart: cluster.logicalPos,
			RuneEnd:   cluster.logicalEnd,
		})
	}
	return result
}

// visualCaretMap is the caret equivalent of visualClusters, over the same
// cluster boundaries: LogicalToVisual[b] is the visual boundary the caret is
// drawn at when it stands at logical boundary b (between clusters b-1 and
// b), VisualToLogical[v] the logical boundary a click at visual boundary v
// selects. The placement rule is vtui.BidiLayout.CaretVisual: the caret
// stands at the trailing edge of the cluster it follows, so inside a right to
// left word it walks leftwards while the logical position advances, as in
// Notepad and the Windows edit controls.
type visualCaretMap struct {
	VisualToLogical []int
	LogicalToVisual []int
}

func buildVisualCaretMap(text string) visualCaretMap {
	logical := logicalTextClusters(text)
	n := len(logical)
	visualToLogical := make([]int, n+1)
	logicalToVisual := make([]int, n+1)
	if vtui.DefaultBidiMode != vtui.BidiFull || !vtui.HasRTL(text) || n == 0 {
		for i := 0; i <= n; i++ {
			visualToLogical[i] = i
			logicalToVisual[i] = i
		}
		return visualCaretMap{VisualToLogical: visualToLogical, LogicalToVisual: logicalToVisual}
	}
	layout := layoutLine(text, logical)
	for i := 0; i <= n; i++ {
		logicalToVisual[i] = layout.CaretVisual(i)
		visualToLogical[i] = layout.CaretLogical(i)
	}
	return visualCaretMap{VisualToLogical: visualToLogical, LogicalToVisual: logicalToVisual}
}

func logicalClusterIndexAtByte(clusters []visualCluster, byteOffset int) int {
	for index, cluster := range clusters {
		if byteOffset <= cluster.byteStart || byteOffset < cluster.byteEnd {
			return index
		}
	}
	return len(clusters)
}

func visualClusterWidths(clusters []visualCluster, tabSize int, origins ...int) []int {
	if tabSize <= 0 {
		tabSize = 8
	}
	widths := make([]int, len(clusters))
	column := 0
	if len(origins) > 0 {
		column = origins[0]
	}
	for i, cluster := range clusters {
		width := cluster.width
		if cluster.text == "\t" {
			width = tabSize - (column % tabSize)
		}
		if width <= 0 {
			width = 1
		}
		widths[i] = width
		column += width
	}
	return widths
}

func fragmentLogicalToVisual(text string, byteOffset, tabSize int, origins ...int) int {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(text) {
		byteOffset = len(text)
	}
	clusters := visualClusters(text)
	visualPos := 0
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		logicalIndex := logicalClusterIndexAtByte(logicalTextClusters(text), byteOffset)
		caret := buildVisualCaretMap(text)
		if logicalIndex < len(caret.LogicalToVisual) {
			visualPos = caret.LogicalToVisual[logicalIndex]
		}
	} else {
		for _, cluster := range clusters {
			if byteOffset < cluster.byteEnd {
				break
			}
			visualPos++
		}
	}
	if visualPos > len(clusters) {
		visualPos = len(clusters)
	}
	widths := visualClusterWidths(clusters, tabSize, origins...)
	width := 0
	for _, clusterWidth := range widths[:visualPos] {
		width += clusterWidth
	}
	return width
}

func fragmentVisualToLogical(text string, visualCol, tabSize int, origins ...int) int {
	clusters := visualClusters(text)
	widths := visualClusterWidths(clusters, tabSize, origins...)
	visualPos := 0
	if visualCol > 0 {
		width := 0
		for visualPos < len(clusters) && width+widths[visualPos] <= visualCol {
			width += widths[visualPos]
			visualPos++
		}
	}
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		logical := logicalTextClusters(text)
		caret := buildVisualCaretMap(text)
		logicalIndex := len(logical)
		if visualPos < len(caret.VisualToLogical) {
			logicalIndex = caret.VisualToLogical[visualPos]
		}
		if logicalIndex < len(logical) {
			return logical[logicalIndex].byteStart
		}
		return len(text)
	} else if visualPos < len(clusters) {
		return clusters[visualPos].byteStart
	} else {
		return len(text)
	}
}

func fragmentVisualMove(text string, byteOffset, direction int) (int, bool) {
	clusters := visualClusters(text)
	if len(clusters) == 0 {
		return byteOffset, false
	}
	visualPos := 0
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		logicalIndex := logicalClusterIndexAtByte(logicalTextClusters(text), byteOffset)
		caret := buildVisualCaretMap(text)
		if logicalIndex < len(caret.LogicalToVisual) {
			visualPos = caret.LogicalToVisual[logicalIndex]
		}
	} else {
		for _, cluster := range clusters {
			if byteOffset < cluster.byteEnd {
				break
			}
			visualPos++
		}
	}
	target := visualPos + direction
	if target < 0 || target > len(clusters) {
		return byteOffset, false
	}
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		logical := logicalTextClusters(text)
		caret := buildVisualCaretMap(text)
		logicalIndex := len(logical)
		if target < len(caret.VisualToLogical) {
			logicalIndex = caret.VisualToLogical[target]
		}
		if logicalIndex < len(logical) {
			return logical[logicalIndex].byteStart, true
		}
		return len(text), true
	} else if target < len(clusters) {
		return clusters[target].byteStart, true
	} else {
		return len(text), true
	}
}

// MoveVisual moves one grapheme cluster in the direction shown on screen.
// It also crosses wrapped rows, which lets the editor use one navigation rule
// for LTR, RTL, combining, and wide text.
func (we *WrapEngine) MoveVisual(byteOffset, direction int) int {
	if direction == 0 {
		return byteOffset
	}
	visualRow, _ := we.LogicalToVisual(byteOffset)
	logLineIdx, fragIdx := we.GetLogLineAtVisualRow(visualRow)
	fragments := we.GetFragments(logLineIdx)
	if fragIdx < 0 || fragIdx >= len(fragments) {
		return byteOffset
	}
	frag := fragments[fragIdx]
	we.tmpBuf = we.tmpBuf[:0]
	we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
	rel := byteOffset - frag.ByteOffsetStart
	if rel < 0 {
		rel = 0
	}
	if rel > len(we.tmpBuf) {
		rel = len(we.tmpBuf)
	}
	if moved, ok := fragmentVisualMove(string(we.tmpBuf), rel, direction); ok && moved != rel {
		return frag.ByteOffsetStart + moved
	}
	if direction < 0 && visualRow > 0 {
		return we.VisualToLogical(visualRow-1, int(^uint(0)>>1))
	}
	if direction > 0 && visualRow+1 < we.GetTotalVisualRows() {
		return we.VisualToLogical(visualRow+1, 0)
	}
	return byteOffset
}

func (we *WrapEngine) SetTabSize(size int) {
	if size <= 0 {
		size = 8
	}
	if we.tabSize != size {
		we.tabSize = size
		we.InvalidateCache()
	}
}

func (we *WrapEngine) SetPointers(pt *piecetable.PieceTable, li *piecetable.LineIndex) {
	we.pt = pt
	we.li = li
	we.InvalidateCache()
}

// SetWidth устанавливает ширину для свертки. При изменении сбрасывает кэш.
func (we *WrapEngine) SetWidth(width int) {
	if width < 1 {
		width = 1
	} // Ширина не может быть меньше 1
	if width != we.wrapWidth {
		we.wrapWidth = width
		we.InvalidateCache()
	}
}

// ToggleWrap включает/выключает перенос по словам.
func (we *WrapEngine) ToggleWrap(wrap bool) {
	if wrap != we.wordWrap {
		we.wordWrap = wrap
		we.InvalidateCache()
	}
}

// InvalidateCache сбрасывает кэш фрагментов.
func (we *WrapEngine) InvalidateCache() {
	we.fragmentCache = nil
	we.validUntil = -1
	we.rowOffsets = nil
	we.totalRows = 0
	we.lastReadError = nil
}

func (we *WrapEngine) InvalidateFrom(logLineIdx int) {
	if logLineIdx < 0 {
		logLineIdx = 0
	}
	if we.fragmentCache != nil && logLineIdx < len(we.fragmentCache) {
		for i := logLineIdx; i < len(we.fragmentCache); i++ {
			we.fragmentCache[i] = lineLayout{}
		}
	}
	if logLineIdx <= we.validUntil {
		we.validUntil = logLineIdx - 1
	}
}

// GetFragments explicitly requests the complete logical line. Viewport callers
// use GetFragmentsThrough so a giant line cannot monopolize its first frame.
func (we *WrapEngine) GetFragments(logLineIdx int) []LineFragment {
	return we.GetFragmentsThrough(logLineIdx, int(^uint(0)>>1))
}

// GetFragmentsThrough resolves only the requested fragment prefix. A pending
// suffix is never returned as an authoritative fragment or visual row count.
func (we *WrapEngine) GetFragmentsThrough(logLineIdx, count int) []LineFragment {
	we.advanceLine(logLineIdx, max(1, count), int(^uint(0)>>1), -1, -1)
	if logLineIdx < 0 || logLineIdx >= len(we.fragmentCache) {
		return nil
	}
	layout := &we.fragmentCache[logLineIdx]
	if len(layout.fragments) != 0 {
		return layout.fragments
	}
	start := we.li.GetLineOffset(logLineIdx)
	return []LineFragment{{
		LogicalLineIdx: logLineIdx, ByteOffsetStart: start,
		ByteOffsetEnd: start, VisualWidth: 16, Loading: true,
	}}
}

// GetFragmentsForOffset resolves the fragment containing a source anchor, not
// the rest of that logical line. This also distinguishes a wrap boundary from
// the end of the line: its following fragment must exist before a hit is mapped.
func (we *WrapEngine) GetFragmentsForOffset(logLineIdx, offset int) []LineFragment {
	we.advanceLine(logLineIdx, int(^uint(0)>>1), int(^uint(0)>>1), offset, -1)
	if logLineIdx < 0 || logLineIdx >= len(we.fragmentCache) {
		return nil
	}
	fragments := we.projectedFragments(logLineIdx)
	if scan := we.fragmentCache[logLineIdx].pending; !we.wordWrap && scan != nil && !scan.stopped && scan.offset <= offset && len(fragments) > 0 {
		fragments[0].Loading = true
	}
	return fragments
}

// GetProjectionFragments resolves the visible horizontal span of an unwrapped
// row. Its remaining source width has no bearing on the number of rows. A
// partial row is ready only after every requested cell has been scanned.
func (we *WrapEngine) GetProjectionFragments(logLineIdx, count, columns int) []LineFragment {
	if we.wordWrap {
		return we.GetFragmentsThrough(logLineIdx, count)
	}
	we.advanceLine(logLineIdx, 1, int(^uint(0)>>1), -1, max(1, columns))
	fragments := we.projectedFragments(logLineIdx)
	if logLineIdx >= 0 && logLineIdx < len(we.fragmentCache) {
		if scan := we.fragmentCache[logLineIdx].pending; scan != nil && !scan.stopped && scan.width < max(1, columns) && len(fragments) > 0 {
			fragments[0].Loading = true
		}
	}
	return fragments
}

func (we *WrapEngine) projectedFragments(logLineIdx int) []LineFragment {
	if logLineIdx < 0 || logLineIdx >= len(we.fragmentCache) {
		return nil
	}
	layout := &we.fragmentCache[logLineIdx]
	if we.wordWrap || len(layout.fragments) != 0 {
		return layout.fragments
	}
	if scan := layout.pending; scan != nil {
		return []LineFragment{{
			LogicalLineIdx: logLineIdx, ByteOffsetStart: scan.start,
			ByteOffsetEnd: scan.offset, VisualWidth: scan.width,
			Loading: scan.width == 0 && !scan.stopped,
		}}
	}
	return nil
}

func (we *WrapEngine) runeWidth(r rune, column int) int {
	if r == '\t' {
		return we.tabSize - column%we.tabSize
	}
	if r >= 0x7f {
		return max(1, runewidth.RuneWidth(r))
	}
	return 1
}

// advanceLine scans sequential bounded reads and commits only complete visual
// rows. Word-wrap carry consists of coordinates/widths, so even a word crossing
// a read boundary is scanned once. UTF-8 and CRLF need at most three lookahead
// bytes; a chunk boundary is never interpreted as EOF or a new tab origin.
func (we *WrapEngine) advanceLine(logLineIdx, fragmentLimit, byteBudget, throughOffset, throughColumn int) (consumed int, progressed bool) {
	lineCount := we.li.LineCount()
	we.resizeFragmentCache(lineCount)
	if logLineIdx < 0 || logLineIdx >= lineCount {
		return 0, false
	}
	layout := &we.fragmentCache[logLineIdx]
	if layout.complete() {
		return 0, false
	}
	end := we.pt.Size()
	if logLineIdx+1 < lineCount {
		end = we.li.GetLineOffset(logLineIdx + 1)
	}
	start := we.li.GetLineOffset(logLineIdx)
	scan := lineScan{offset: start, start: start, spaceEnd: -1}
	if layout.pending != nil {
		scan = *layout.pending
	}
	if scan.stopped {
		return 0, false
	}
	finished := false
	defer func() {
		if finished {
			layout.pending = nil
			return
		}
		if layout.pending == nil {
			layout.pending = new(lineScan)
		}
		*layout.pending = scan
	}()
	appendFragment := func(end, width, runes int) {
		layout.fragments = append(layout.fragments, LineFragment{
			LogicalLineIdx: logLineIdx, ByteOffsetStart: scan.start,
			ByteOffsetEnd: end, VisualWidth: width,
			VisualColumnStart: scan.column, RuneStart: scan.runeStart,
		})
		scan.start = end
		scan.column += width
		scan.runeStart += runes
		scan.width -= width
		scan.runes -= runes
		scan.spaceEnd = -1
		progressed = true
	}
	finishLine := func(contentEnd int, indexedEnd bool) {
		if scan.start < contentEnd || len(layout.fragments) == 0 {
			appendFragment(contentEnd, scan.width, scan.runes)
		}
		finished = indexedEnd
		scan.stopped = !indexedEnd
		progressed = true
	}
	satisfied := func() bool {
		if !we.wordWrap {
			if throughOffset >= 0 {
				return scan.offset > throughOffset
			}
			if throughColumn >= 0 {
				return scan.width >= throughColumn
			}
		}
		if throughOffset >= 0 {
			return len(layout.fragments) > 0 && layout.fragments[len(layout.fragments)-1].ByteOffsetEnd > throughOffset
		}
		return len(layout.fragments) >= fragmentLimit
	}
	for !finished && !scan.stopped && !satisfied() {
		if scan.offset >= end {
			finishLine(end, true)
			break
		}
		if consumed >= max(1, byteBudget) {
			break
		}
		readLength := min(layoutReadBytes, end-scan.offset, min(layoutReadBytes, max(1, byteBudget)-consumed)+utf8.UTFMax-1)
		we.tmpBuf = we.tmpBuf[:0]
		var err error
		we.tmpBuf, err = we.pt.AppendRange(we.tmpBuf, scan.offset, readLength)
		if err != nil {
			if err != piecetable.ErrLoading {
				we.lastReadError = err
			}
			break
		}
		if len(we.tmpBuf) == 0 {
			we.lastReadError = io.ErrUnexpectedEOF
			break
		}
		we.lastReadError = nil
		data := we.tmpBuf
		readStart := scan.offset
		for len(data) > 0 && !finished && !scan.stopped && !satisfied() {
			if consumed >= max(1, byteBudget) {
				break
			}
			// Retain a partial rune/CR only as its source position; the next
			// bounded read supplies the remaining bytes.
			if !utf8.FullRune(data) && scan.offset+len(data) < end {
				break
			}
			if data[0] == '\r' && len(data) == 1 && scan.offset+1 < end {
				break
			}
			newline := 0
			if data[0] == '\n' {
				newline = 1
			} else if len(data) >= 2 && data[0] == '\r' && data[1] == '\n' {
				newline = 2
			}
			if newline != 0 {
				contentEnd := scan.offset
				scan.offset += newline
				consumed += newline
				finishLine(contentEnd, logLineIdx+1 < lineCount && scan.offset == end)
				break
			}
			r, size := utf8.DecodeRune(data)
			width := we.runeWidth(r, scan.column+scan.width)
			if we.wordWrap && scan.width+width > we.wrapWidth {
				if r == ' ' || scan.start == scan.offset {
					scan.width += width
					scan.runes++
					scan.offset += size
					consumed += size
					data = data[size:]
					appendFragment(scan.offset, scan.width, scan.runes)
				} else if scan.spaceEnd >= 0 {
					appendFragment(scan.spaceEnd, scan.spaceWidth, scan.spaceRunes)
				} else {
					appendFragment(scan.offset, scan.width, scan.runes)
				}
				continue
			}
			scan.width += width
			scan.runes++
			scan.offset += size
			consumed += size
			data = data[size:]
			if r == ' ' {
				scan.spaceEnd, scan.spaceWidth, scan.spaceRunes = scan.offset, scan.width, scan.runes
			}
		}
		if scan.offset >= end && !finished && !scan.stopped {
			finishLine(end, true)
		}
		if scan.offset == readStart && !satisfied() && !finished && !scan.stopped {
			// A short underlying read cannot manufacture an invalid rune or
			// spin forever while waiting for the rest of a source character.
			we.lastReadError = io.ErrUnexpectedEOF
			break
		}
	}
	return consumed, progressed || consumed > 0
}

// LastReadError distinguishes a source failure from a pending read. It is
// cleared when actual data arrives or a new layout/source is installed.
func (we *WrapEngine) LastReadError() error { return we.lastReadError }

// updateRowOffsets commits prefix sums only across complete logical lines,
// while preserving the ready row prefix of the one incomplete current line.
func (we *WrapEngine) updateRowOffsets() {
	lineCount := we.li.LineCount()
	if len(we.rowOffsets) > lineCount {
		we.InvalidateCache()
	}
	we.resizeFragmentCache(lineCount)
	we.resizeRowOffsets(lineCount)
	if !we.wordWrap {
		for i := we.validUntil + 1; i < lineCount; i++ {
			we.rowOffsets[i] = i
		}
		we.validUntil, we.totalRows = lineCount-1, lineCount
		return
	}
	current := 0
	if we.validUntil >= 0 {
		current = we.rowOffsets[we.validUntil] + len(we.fragmentCache[we.validUntil].fragments)
	}
	for line := we.validUntil + 1; line < lineCount; line++ {
		we.rowOffsets[line] = current
		if !we.fragmentCache[line].complete() {
			break
		}
		current += len(we.fragmentCache[line].fragments)
		we.validUntil = line
	}
	we.totalRows = current
}

func (we *WrapEngine) ensureRowCountCache(until int) {
	we.updateRowOffsets()
	until = min(until, we.li.LineCount()-1)
	for we.validUntil < until {
		line := we.validUntil + 1
		we.GetFragments(line)
		we.updateRowOffsets()
		if we.validUntil < line {
			break
		}
	}
}

// GetTotalVisualRows is an explicit full-layout request. First-frame and
// scrolling callers use KnownVisualRows / GetLogLineAtVisualRow instead.
func (we *WrapEngine) GetTotalVisualRows() int {
	if !we.wordWrap {
		return we.li.LineCount()
	}
	we.ensureRowCountCache(we.li.LineCount() - 1)
	rows, _ := we.KnownVisualRows()
	return rows
}

// KnownVisualRows reports only ready rows. The final line is not complete
// until its real end has been read, even if its prefix already fills a window.
func (we *WrapEngine) KnownVisualRows() (rows int, complete bool) {
	we.updateRowOffsets()
	lineCount := we.li.LineCount()
	if !we.wordWrap {
		return lineCount, true
	}
	rows = we.totalRows
	if line := we.validUntil + 1; line < lineCount {
		rows += len(we.fragmentCache[line].fragments)
	}
	return rows, we.validUntil == lineCount-1
}

// AdvanceVisualRows performs the next bounded portion of extent calculation
// on the owning UI thread. The byte budget applies inside a single giant line,
// not only between lines. At most one UTF-8 rune can cross the budget boundary.
func (we *WrapEngine) AdvanceVisualRows(maxLines, maxBytes int) (progressed, complete bool) {
	if _, complete = we.KnownVisualRows(); complete {
		return false, true
	}
	remaining := max(1, maxBytes)
	for count := 0; count < max(1, maxLines) && remaining > 0; count++ {
		line := we.validUntil + 1
		if line >= we.li.LineCount() {
			break
		}
		consumed, advanced := we.advanceLine(line, int(^uint(0)>>1), remaining, -1, -1)
		remaining -= consumed
		progressed = progressed || advanced
		we.updateRowOffsets()
		if !advanced || we.validUntil < line {
			break
		}
	}
	_, complete = we.KnownVisualRows()
	return progressed, complete
}

// GetRowOffset needs the preceding lines, not the remainder of this line.
func (we *WrapEngine) GetRowOffset(logLineIdx int) int {
	if logLineIdx < 0 {
		return 0
	}
	we.ensureRowCountCache(logLineIdx - 1)
	if logLineIdx >= len(we.rowOffsets) {
		return we.totalRows
	}
	return we.rowOffsets[logLineIdx]
}

// GetLogLineAtVisualRow advances only far enough to identify the requested
// row, including when all requested rows belong to one enormous logical line.
func (we *WrapEngine) GetLogLineAtVisualRow(visualRow int) (logLineIdx int, fragIdx int) {
	we.updateRowOffsets()
	lineCount := we.li.LineCount()
	if visualRow < 0 || lineCount == 0 {
		return 0, 0
	}
	if !we.wordWrap {
		return min(visualRow, lineCount-1), 0
	}
	for {
		known, complete := we.KnownVisualRows()
		if known > visualRow || complete {
			break
		}
		line := we.validUntil + 1
		_, advanced := we.advanceLine(line, visualRow-we.rowOffsets[line]+1, int(^uint(0)>>1), -1, -1)
		we.updateRowOffsets()
		if !advanced || (we.validUntil < line && we.fragmentCache[line].pending.stopped) {
			return line, max(0, visualRow-we.rowOffsets[line])
		}
	}
	// Include the current partial line in the searchable prefix.
	count := min(lineCount, we.validUntil+2)
	logLineIdx = sort.Search(count, func(i int) bool {
		return we.rowOffsets[i] > visualRow
	}) - 1
	logLineIdx = max(0, logLineIdx)
	fragIdx = max(0, visualRow-we.rowOffsets[logLineIdx])
	if we.validUntil == lineCount-1 {
		fragIdx = min(fragIdx, max(0, len(we.fragmentCache[logLineIdx].fragments)-1))
	}
	return
}

// LogicalToVisual переводит байтовый оффсет в документе в (строка, колонка) на экране.
func (we *WrapEngine) LogicalToVisual(byteOffset int) (visualRow, visualCol int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	logLineIdx := we.li.GetLineAtOffset(byteOffset)
	we.ensureRowCountCache(logLineIdx - 1)
	fragments := we.GetFragmentsForOffset(logLineIdx, byteOffset)
	if !we.wordWrap {
		fragments = we.GetProjectionFragments(logLineIdx, 1, max(we.wrapWidth, byteOffset-we.li.GetLineOffset(logLineIdx)+1))
	}
	totalRow := we.rowOffsets[logLineIdx]

	if len(fragments) > 0 {
		lastFrag := fragments[len(fragments)-1]
		if byteOffset > lastFrag.ByteOffsetEnd {
			// Newline bytes belong to the end of the final content fragment.
			byteOffset = lastFrag.ByteOffsetEnd
		}
	}

	for i, frag := range fragments {
		isLastFragOfLine := (i == len(fragments)-1)
		if byteOffset >= frag.ByteOffsetStart && (byteOffset < frag.ByteOffsetEnd || (isLastFragOfLine && byteOffset == frag.ByteOffsetEnd)) {
			we.tmpBuf = we.tmpBuf[:0]
			we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			return totalRow + i, fragmentLogicalToVisual(string(we.tmpBuf), byteOffset-frag.ByteOffsetStart, we.tabSize, frag.VisualColumnStart)
		}
	}
	return totalRow, 0
}

// VisualToLogical переводит (строка, колонка) на экране в байтовый оффсет документа.
func (we *WrapEngine) VisualToLogical(visualRow, visualCol int) int {
	if visualRow < 0 {
		return 0
	}
	logLineIdx, fragIdx := we.GetLogLineAtVisualRow(visualRow)
	fragments := we.GetProjectionFragments(logLineIdx, fragIdx+1, max(we.wrapWidth, visualCol+1))
	if fragments == nil {
		vtui.DebugLog("DEBUG_V2L_FAIL: No fragments for LogLine %d", logLineIdx)
		return 0
	}
	if fragIdx >= len(fragments) {
		fragIdx = len(fragments) - 1
	}
	frag := fragments[fragIdx]

	vtui.DebugLog("DEBUG_V2L_START: Row:%d Col:%d -> LogLine:%d Frag:%d StartOff:%d EndOff:%d", visualRow, visualCol, logLineIdx, fragIdx, frag.ByteOffsetStart, frag.ByteOffsetEnd)
	return we.FragmentColumnToLogical(frag, visualCol)
}

// FragmentColumnToLogical resolves a position in an already identified source
// fragment. Native pointer events use this to avoid reinterpreting a displayed
// row through a newer viewport's top row or terminal coordinates.
func (we *WrapEngine) FragmentColumnToLogical(frag LineFragment, visualCol int) int {
	if frag.ByteOffsetStart >= frag.ByteOffsetEnd {
		return frag.ByteOffsetStart
	}
	we.tmpBuf = we.tmpBuf[:0]
	we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
	return frag.ByteOffsetStart + fragmentVisualToLogical(string(we.tmpBuf), visualCol, we.tabSize, frag.VisualColumnStart)
}
