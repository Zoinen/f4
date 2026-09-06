package textlayout

import (
	"io"
	"sort"
	"unicode/utf8"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtui"

	"github.com/mattn/go-runewidth"
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
			// Вычисляем колонку без аллокаций
			width := 0
			if byteOffset > frag.ByteOffsetStart {
				we.tmpBuf = we.tmpBuf[:0]
				we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, byteOffset-frag.ByteOffsetStart)
				data := we.tmpBuf
				for len(data) > 0 {
					r, size := utf8.DecodeRune(data)
					rw := 1
					if r == '\t' {
						rw = we.tabSize - ((frag.VisualColumnStart + width) % we.tabSize)
					} else if r >= 0x7F {
						rw = runewidth.RuneWidth(r)
					}
					if rw <= 0 {
						rw = 1
					}
					width += rw
					data = data[size:]
				}
			}
			return totalRow + i, width
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
	fragments := we.GetProjectionFragments(logLineIdx, fragIdx+1, max(1, visualCol+1))
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
	if frag.ByteOffsetStart >= frag.ByteOffsetEnd || visualCol <= 0 {
		return frag.ByteOffsetStart
	}

	we.tmpBuf = we.tmpBuf[:0]
	readLength := frag.ByteOffsetEnd - frag.ByteOffsetStart
	if visualCol < readLength/utf8.UTFMax {
		readLength = visualCol * utf8.UTFMax
	}
	we.tmpBuf, _ = we.pt.AppendRange(we.tmpBuf, frag.ByteOffsetStart, readLength)
	lineData := we.tmpBuf
	offset := frag.ByteOffsetStart
	currentCol := 0

	for len(lineData) > 0 {
		r, size := utf8.DecodeRune(lineData)
		rw := 1
		if r == '\t' {
			rw = we.tabSize - ((frag.VisualColumnStart + currentCol) % we.tabSize)
		} else if r >= 0x7F {
			rw = runewidth.RuneWidth(r)
		}
		if rw <= 0 {
			rw = 1
		}
		if currentCol+rw > visualCol {
			return offset
		}
		currentCol += rw
		offset += size
		lineData = lineData[size:]
	}
	return offset
}
