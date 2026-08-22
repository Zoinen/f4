package textlayout

import (
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

func visualClusters(text string) []visualCluster {
	logical := logicalTextClusters(text)
	if vtui.DefaultBidiMode != vtui.BidiFull || !vtui.HasRTL(text) {
		return logical
	}

	byLogical := make(map[int]visualCluster, len(logical))
	for _, cluster := range logical {
		byLogical[cluster.logicalPos] = cluster
	}
	visualText, logicalPositions := vtui.VisualStringWithRuneMap(text)
	visual := make([]visualCluster, 0, len(logical))
	index := 0
	vtui.ForEachClusterAt(visualText, func(cluster string, width, _, _ int) {
		if index >= len(logicalPositions) {
			return
		}
		original, ok := byLogical[logicalPositions[index]]
		if !ok {
			index++
			return
		}
		original.text = cluster
		original.width = width
		visual = append(visual, original)
		index++
	})
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

func byteOffsetAtRune(text string, runeIndex int) int {
	if runeIndex <= 0 {
		return 0
	}
	if runeIndex >= utf8.RuneCountInString(text) {
		return len(text)
	}
	return len(string([]rune(text)[:runeIndex]))
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
	runeIndex := utf8.RuneCountInString(text[:byteOffset])
	logical := logicalTextClusters(text)
	clusters := visualClusters(text)
	visualPos := 0
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		for _, cluster := range logical {
			if byteOffset < cluster.byteEnd {
				runeIndex = cluster.logicalPos
				break
			}
		}
		caret := vtui.BuildCaretMap(text)
		if runeIndex < len(caret.LogicalToVisual) {
			visualPos = caret.LogicalToVisual[runeIndex]
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
	runeIndex := 0
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		caret := vtui.BuildCaretMap(text)
		if visualPos < len(caret.VisualToLogical) {
			runeIndex = caret.VisualToLogical[visualPos]
		} else {
			runeIndex = utf8.RuneCountInString(text)
		}
	} else if visualPos < len(clusters) {
		runeIndex = clusters[visualPos].logicalPos
	} else {
		runeIndex = utf8.RuneCountInString(text)
	}
	return byteOffsetAtRune(text, runeIndex)
}

func fragmentVisualMove(text string, byteOffset, direction int) (int, bool) {
	clusters := visualClusters(text)
	if len(clusters) == 0 {
		return byteOffset, false
	}
	runeIndex := utf8.RuneCountInString(text[:byteOffset])
	visualPos := 0
	if vtui.DefaultBidiMode == vtui.BidiFull && vtui.HasRTL(text) {
		for _, cluster := range logicalTextClusters(text) {
			if byteOffset < cluster.byteEnd {
				runeIndex = cluster.logicalPos
				break
			}
		}
		caret := vtui.BuildCaretMap(text)
		if runeIndex < len(caret.LogicalToVisual) {
			visualPos = caret.LogicalToVisual[runeIndex]
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
		caret := vtui.BuildCaretMap(text)
		if target < len(caret.VisualToLogical) {
			runeIndex = caret.VisualToLogical[target]
		} else {
			runeIndex = utf8.RuneCountInString(text)
		}
	} else if target < len(clusters) {
		runeIndex = clusters[target].logicalPos
	} else {
		runeIndex = utf8.RuneCountInString(text)
	}
	return byteOffsetAtRune(text, runeIndex), true
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
}

// GetFragments возвращает визуальные фрагменты для одной логической строки.
func (we *WrapEngine) GetFragments(logLineIdx int) []LineFragment {
	lineCount := we.li.LineCount()
	if logLineIdx < 0 || logLineIdx >= lineCount {
		return nil
	}

	if we.wordWrap && we.fragmentCache != nil && logLineIdx < len(we.fragmentCache) && we.fragmentCache[logLineIdx] != nil {
		return we.fragmentCache[logLineIdx]
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
		clusters := visualClusters(string(lineData))
		widths := visualClusterWidths(clusters, we.tabSize)
		width := 0
		for _, clusterWidth := range widths {
			width += clusterWidth
		}

		frag := LineFragment{
			LogicalLineIdx:  logLineIdx,
			ByteOffsetStart: startOffset,
			ByteOffsetEnd:   startOffset + len(lineData),
			VisualWidth:     width,
		}
		return []LineFragment{frag}
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

	if we.rowOffsets == nil || len(we.rowOffsets) != lineCount {
		oldOffsets := we.rowOffsets
		we.rowOffsets = make([]int, lineCount)
		if oldOffsets != nil {
			numToCopy := len(oldOffsets)
			if numToCopy > lineCount {
				numToCopy = lineCount
			}
			copy(we.rowOffsets, oldOffsets[:numToCopy])
			if we.validUntil >= numToCopy {
				we.validUntil = numToCopy - 1
			}
		} else {
			we.validUntil = -1
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

func (we *WrapEngine) logNav(msg string, offset int, row int, col int) {
	// Only log if specifically requested to avoid flooding
	// vtui.DebugLog("LAYOUT_NAV: %s Offset:%d -> VRow:%d VCol:%d", msg, offset, row, col)
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
