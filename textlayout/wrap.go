package textlayout

import (
	"math"
	"sort"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"
)

// LineFragment описывает один визуальный кусок логической строки после свертки.
type LineFragment struct {
	LogicalLineIdx  int // Номер оригинальной строки (до \n)
	ByteOffsetStart int // Смещение начала фрагмента (от начала всего файла/буфера)
	ByteOffsetEnd   int // Смещение конца фрагмента
	VisualWidth     int // Ширина фрагмента в колонках терминала (учитывая CJK)
}

// WrapEngine отвечает за вычисление визуальной разметки текста.
type WrapEngine struct {
	pt            *piecetable.PieceTable
	li            *piecetable.LineIndex
	wrapWidth     int
	wordWrap      bool
	fragmentCache [][]LineFragment
	tabSize       int

	// rowOffsets[i] хранит общее количество визуальных строк во всех
	// логических строках ПЕРЕД строкой i.
	rowOffsets []int
	totalRows  int
	validUntil int // Index of the last logical line with a valid calculated row offset

	tmpBuf []byte // Reusable buffer for avoiding allocations

	// noWrapCache is deliberately a map rather than fragmentCache: edits in a
	// huge file must not walk or allocate a slice for every logical line.
	noWrapCache map[int]noWrapLayout
	// noWrapCached counts the cluster entries held across noWrapCache so the
	// cache cannot grow with the number of lines the user has scrolled past.
	noWrapCached int
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
	grown := make([][]LineFragment, lineCount,
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

func visualClusterWidths(clusters []visualCluster, tabSize int) []int {
	if tabSize <= 0 {
		tabSize = 8
	}
	widths := make([]int, len(clusters))
	column := 0
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

func fragmentLogicalToVisual(text string, byteOffset, tabSize int) int {
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
	widths := visualClusterWidths(clusters, tabSize)
	width := 0
	for _, clusterWidth := range widths[:visualPos] {
		width += clusterWidth
	}
	return width
}

func fragmentVisualToLogical(text string, visualCol, tabSize int) int {
	clusters := visualClusters(text)
	widths := visualClusterWidths(clusters, tabSize)
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
	we.noWrapCache = nil
	we.noWrapCached = 0
}

func (we *WrapEngine) InvalidateFrom(logLineIdx int) {
	if logLineIdx < 0 {
		logLineIdx = 0
	}
	if we.fragmentCache != nil && logLineIdx < len(we.fragmentCache) {
		for i := logLineIdx; i < len(we.fragmentCache); i++ {
			we.fragmentCache[i] = nil
		}
	}
	if logLineIdx <= we.validUntil {
		we.validUntil = logLineIdx - 1
	}
	for idx, cached := range we.noWrapCache {
		if idx >= logLineIdx {
			we.noWrapCached -= len(cached.clusterEnds)
			delete(we.noWrapCache, idx)
		}
	}
}

// GetFragments возвращает визуальные фрагменты для одной логической строки.
func (we *WrapEngine) GetFragments(logLineIdx int) []LineFragment {
	lineCount := we.li.LineCount()
	we.resizeFragmentCache(lineCount)

	if logLineIdx < 0 || logLineIdx >= lineCount {
		return nil
	}

	if we.wordWrap {
		if we.fragmentCache != nil && logLineIdx < len(we.fragmentCache) && we.fragmentCache[logLineIdx] != nil {
			return we.fragmentCache[logLineIdx]
		}
	} else if cached, ok := we.noWrapCache[logLineIdx]; ok {
		return cached.fragments
	}

	startOffset := we.li.GetLineOffset(logLineIdx)
	endOffset := we.pt.Size()
	if logLineIdx+1 < we.li.LineCount() {
		endOffset = we.li.GetLineOffset(logLineIdx + 1)
	} else {
		// If this is the unindexed tail, cap the processing to prevent loading gigabytes
		if endOffset-startOffset > 64*1024 {
			endOffset = startOffset + 64*1024
		}
	}

	we.tmpBuf = we.tmpBuf[:0]
	var err error
	we.tmpBuf, err = we.pt.AppendRange(we.tmpBuf, startOffset, endOffset-startOffset)

	// If data is not ready, return a dummy visual fragment
	if err == piecetable.ErrLoading {
		frag := LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset,
			ByteOffsetEnd:   endOffset,
			VisualWidth:     16, // Width of "[ Loading... ]"
		}
		return []LineFragment{frag} // DO NOT CACHE LOADING STUBS
	}

	lineData := we.tmpBuf
	truncated := false

	// Убираем \n или \r\n с конца
	if len(lineData) > 0 && lineData[len(lineData)-1] == '\n' {
		lineData = lineData[:len(lineData)-1]
		if len(lineData) > 0 && lineData[len(lineData)-1] == '\r' {
			lineData = lineData[:len(lineData)-1]
		}
	}

	// PREVENT LONG-LINE FLASH:
	// If lineData STILL contains '\n' in the middle (because LineIndex hasn't indexed it yet),
	// truncate it to the first '\n' to prevent rendering multiple lines as one continuous paragraph.
	for i, b := range lineData {
		if b == '\n' {
			lineData = lineData[:i]
			if i > 0 && lineData[i-1] == '\r' {
				lineData = lineData[:i-1]
			}
			truncated = true
			break
		}
	}

	if !we.wordWrap || we.wrapWidth <= 0 {
		text := string(lineData)
		clusters := visualClusters(text)
		clusterEnds := make([]int, len(clusters))
		prefixWidths := make([]int, len(clusters)+1)
		for i, cluster := range clusters {
			width := cluster.width
			if cluster.text == "\t" {
				width = we.tabSize - (int(prefixWidths[i]) % we.tabSize)
			}
			if width <= 0 {
				width = 1
			}
			clusterEnds[i] = cluster.byteEnd
			prefixWidths[i+1] = prefixWidths[i] + width
		}
		frag := LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset,
			ByteOffsetEnd:   startOffset + len(lineData),
			VisualWidth:     prefixWidths[len(prefixWidths)-1],
		}
		fragments := []LineFragment{frag}
		if !truncated && len(text) <= math.MaxInt32 {
			if we.noWrapCache == nil {
				we.noWrapCache = make(map[int]noWrapLayout)
			}
			if previous, ok := we.noWrapCache[logLineIdx]; ok {
				we.noWrapCached -= len(previous.clusterEnds)
			} else if we.noWrapCached+len(clusterEnds) > noWrapCacheBudget {
				we.noWrapCache = make(map[int]noWrapLayout)
				we.noWrapCached = 0
			}
			we.noWrapCache[logLineIdx] = noWrapLayout{
				fragments:    fragments,
				clusterEnds:  clusterEnds,
				prefixWidths: prefixWidths,
				hasRTL:       vtui.HasRTL(text),
			}
			we.noWrapCached += len(clusterEnds)
		}
		return fragments
	}

	if we.fragmentCache == nil || len(we.fragmentCache) != lineCount {
		we.fragmentCache = make([][]LineFragment, lineCount)
	}

	var fragments []LineFragment
	bytePos := 0
	dataLen := len(lineData)
	logicalClusters := logicalTextClusters(string(lineData))
	clusterPos := 0

	cumulativeVisualWidth := 0
	for bytePos < dataLen && clusterPos < len(logicalClusters) {
		visualWidth := 0
		fragStartByte := bytePos
		lastSpaceEnd := -1
		lastSpaceWidth := 0
		lastSpaceClusterPos := -1

		scanPos := bytePos
		scanClusterPos := clusterPos
		for scanClusterPos < len(logicalClusters) {
			cluster := logicalClusters[scanClusterPos]
			clusterText := cluster.text
			w := cluster.width
			if clusterText == "\t" {
				w = we.tabSize - ((cumulativeVisualWidth + visualWidth) % we.tabSize)
			}
			if w <= 0 {
				w = 1
			}

			if visualWidth+w > we.wrapWidth {
				if clusterText == " " {
					// Пробел не влезает, но мы его забираем в конец этой строки
					scanPos = cluster.byteEnd
					scanClusterPos++
					visualWidth += w
				} else if lastSpaceEnd != -1 {
					// Word Wrap: откатываемся к последнему пробелу
					scanPos = lastSpaceEnd
					scanClusterPos = lastSpaceClusterPos
					visualWidth = lastSpaceWidth
				} else if scanPos == fragStartByte {
					// Даже один символ не влез (CJK в узком окне) - поглощаем его
					scanPos = cluster.byteEnd
					scanClusterPos++
					visualWidth = w
				}
				break
			}

			visualWidth += w
			scanPos = cluster.byteEnd
			scanClusterPos++
			if clusterText == " " {
				lastSpaceEnd = scanPos
				lastSpaceWidth = visualWidth
				lastSpaceClusterPos = scanClusterPos
			}
		}

		fragments = append(fragments, LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset + fragStartByte,
			ByteOffsetEnd:   startOffset + scanPos,
			VisualWidth:     visualWidth,
		})
		cumulativeVisualWidth += visualWidth
		bytePos = scanPos
		clusterPos = scanClusterPos
	}

	if len(fragments) == 0 {
		fragments = append(fragments, LineFragment{LogicalLineIdx: logLineIdx, ByteOffsetStart: startOffset, ByteOffsetEnd: startOffset})
	}

	if !truncated && logLineIdx < len(we.fragmentCache) {
		we.fragmentCache[logLineIdx] = fragments
	}
	return fragments
}

func (we *WrapEngine) ensureRowCountCache(until int) {
	if !we.wordWrap {
		return
	}
	lineCount := we.li.LineCount()
	if until >= lineCount {
		until = lineCount - 1
	}
	if we.validUntil >= until && we.rowOffsets != nil && len(we.rowOffsets) == lineCount {
		return
	}

	if len(we.rowOffsets) != lineCount {
		oldLength := len(we.rowOffsets)
		we.resizeRowOffsets(lineCount)
		if oldLength == 0 {
			we.validUntil = -1
		} else if we.validUntil >= lineCount {
			we.validUntil = lineCount - 1
		}
	}

	currentOffset := 0
	start := we.validUntil + 1
	if start > 0 {
		currentOffset = we.rowOffsets[start-1] + len(we.GetFragments(start-1))
	}

	for i := start; i <= until; i++ {
		we.rowOffsets[i] = currentOffset
		currentOffset += len(we.GetFragments(i))
	}
	if until > we.validUntil {
		we.validUntil = until
	}
	if we.validUntil == lineCount-1 {
		we.totalRows = currentOffset
	}
}

// GetTotalVisualRows возвращает общее количество визуальных строк в документе.
func (we *WrapEngine) GetTotalVisualRows() int {
	if !we.wordWrap {
		return we.li.LineCount()
	}
	we.ensureRowCountCache(we.li.LineCount() - 1)
	return we.totalRows
}

// GetRowOffset возвращает индекс первой визуальной строки для данной логической строки.
func (we *WrapEngine) GetRowOffset(logLineIdx int) int {
	if !we.wordWrap {
		if logLineIdx < 0 {
			return 0
		}
		lineCount := we.li.LineCount()
		if logLineIdx >= lineCount {
			return lineCount
		}
		return logLineIdx
	}
	we.ensureRowCountCache(logLineIdx)
	if logLineIdx < 0 {
		return 0
	}
	if logLineIdx >= len(we.rowOffsets) {
		we.ensureRowCountCache(we.li.LineCount() - 1)
		return we.totalRows
	}
	return we.rowOffsets[logLineIdx]
}

// GetLogLineAtVisualRow переводит абсолютный индекс визуальной строки в индекс
// логической строки и порядковый номер фрагмента внутри неё.
func (we *WrapEngine) GetLogLineAtVisualRow(visualRow int) (logLineIdx int, fragIdx int) {
	if visualRow < 0 {
		return 0, 0
	}
	if !we.wordWrap {
		lineCount := we.li.LineCount()
		if visualRow >= lineCount {
			if lineCount <= 0 {
				return 0, 0
			}
			return lineCount - 1, 0
		}
		return visualRow, 0
	}

	// Lazy calculation until we find the row or hit EOF
	lineCount := we.li.LineCount()
	for we.validUntil < lineCount-1 {
		var lastCalculatedRow int
		if we.validUntil >= 0 {
			lastCalculatedRow = we.rowOffsets[we.validUntil] + len(we.GetFragments(we.validUntil))
		}
		if lastCalculatedRow > visualRow {
			break
		}
		// Expand cache in chunks
		nextTarget := we.validUntil + 500
		if nextTarget >= lineCount {
			nextTarget = lineCount - 1
		}
		we.ensureRowCountCache(nextTarget)
	}

	if visualRow >= we.totalRows && we.validUntil == lineCount-1 {
		return lineCount - 1, 0
	}

	// Binary search on the valid portion of the cache
	logLineIdx = sort.Search(we.validUntil+1, func(i int) bool {
		return we.rowOffsets[i] > visualRow
	}) - 1

	if logLineIdx < 0 {
		logLineIdx = 0
	}
	fragIdx = visualRow - we.rowOffsets[logLineIdx]
	return
}

// LogicalToVisual переводит байтовый оффсет в документе в (строка, колонка) на экране.
func (we *WrapEngine) LogicalToVisual(byteOffset int) (visualRow, visualCol int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	logLineIdx := we.li.GetLineAtOffset(byteOffset)
	totalRow := logLineIdx
	if we.wordWrap {
		we.ensureRowCountCache(logLineIdx)
		totalRow = we.rowOffsets[logLineIdx]
	}
	fragments := we.GetFragments(logLineIdx)

	if len(fragments) > 0 {
		lastFrag := fragments[len(fragments)-1]
		if byteOffset > lastFrag.ByteOffsetEnd {
			// Safety for capped binary lines: if offset is beyond indexed fragments,
			// snap to the end of the last visible fragment.
			byteOffset = lastFrag.ByteOffsetEnd
		}
	}
	if !we.wordWrap {
		if cached, ok := we.noWrapCache[logLineIdx]; ok && !cached.hasRTL {
			frag := cached.fragments[0]
			relative := byteOffset - frag.ByteOffsetStart
			if relative < 0 {
				relative = 0
			} else if relative > frag.ByteOffsetEnd-frag.ByteOffsetStart {
				relative = frag.ByteOffsetEnd - frag.ByteOffsetStart
			}
			clusterIndex := sort.Search(len(cached.clusterEnds), func(i int) bool {
				return relative < cached.clusterEnds[i]
			})
			return totalRow, cached.prefixWidths[clusterIndex]
		}
	}

	for i, frag := range fragments {
		isLastFragOfLine := (i == len(fragments)-1)
		if byteOffset >= frag.ByteOffsetStart && (byteOffset < frag.ByteOffsetEnd || (isLastFragOfLine && byteOffset == frag.ByteOffsetEnd)) {
			we.tmpBuf = we.tmpBuf[:0]
			we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
			return totalRow + i, fragmentLogicalToVisual(string(we.tmpBuf), byteOffset-frag.ByteOffsetStart, we.tabSize)
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
	fragments := we.GetFragments(logLineIdx)
	if fragments == nil {
		vtui.DebugLog("DEBUG_V2L_FAIL: No fragments for LogLine %d", logLineIdx)
		return 0
	}
	if fragIdx >= len(fragments) {
		fragIdx = len(fragments) - 1
	}
	frag := fragments[fragIdx]

	vtui.DebugLog("DEBUG_V2L_START: Row:%d Col:%d -> LogLine:%d Frag:%d StartOff:%d EndOff:%d", visualRow, visualCol, logLineIdx, fragIdx, frag.ByteOffsetStart, frag.ByteOffsetEnd)

	if frag.ByteOffsetStart >= frag.ByteOffsetEnd {
		return frag.ByteOffsetStart
	}

	we.tmpBuf = we.tmpBuf[:0]
	we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
	return frag.ByteOffsetStart + fragmentVisualToLogical(string(we.tmpBuf), visualCol, we.tabSize)
}
