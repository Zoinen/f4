package viewer

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
	"io"
	"unicode/utf8"
)

const (
	viewerEndTailWindow = 192 * 1024
	viewerEndReadSlice  = 64 * 1024
	viewerEndWorkSlice  = 4 * 1024
)

// viewerEndRowScanner is a streaming form of scanViewerText. It retains only
// the final viewport's row positions while carrying UTF-8, tab-stop and
// wrapping state across bounded source reads.
type viewerEndRowScanner struct {
	ctx                       context.Context
	size                      int64
	width, height             int
	wrap, started             bool
	tabSize                   int
	nextOffset                int64
	logicalColumn, widthInRow int
	rows                      []viewerRowPosition
	rowHead, rowCount         int
	totalRows                 int64
	carry                     [utf8.UTFMax]byte
	carryLen                  int
	scratch                   []byte
}

type viewerEndResult struct {
	top     viewerRowPosition
	history []viewerRowPosition
}

func newViewerEndRowScanner(ctx context.Context, size, anchor int64,
	width, height int, wrap bool, tabSize int,
) *viewerEndRowScanner {
	scanner := &viewerEndRowScanner{
		ctx: ctx, size: size, width: width, height: height,
		wrap: wrap, tabSize: max(1, tabSize), nextOffset: anchor,
	}
	if height > 0 {
		// Keep the visible viewport plus the rows needed by semantic overscan.
		// This remains O(viewport) and lets the accepted End result publish
		// without resolving the same long logical line a second time.
		capacity := height + semantic.SemanticWindowBufferRows(height) + 1
		scanner.rows = make([]viewerRowPosition, capacity)
	}
	if anchor < size {
		scanner.appendRow(anchor, 0)
	}
	return scanner
}

func (scanner *viewerEndRowScanner) appendRow(offset int64, column int) {
	if len(scanner.rows) == 0 {
		return
	}
	position := viewerRowPosition{offset: offset, column: column}
	if scanner.rowCount < len(scanner.rows) {
		index := (scanner.rowHead + scanner.rowCount) % len(scanner.rows)
		scanner.rows[index] = position
		scanner.rowCount++
	} else {
		scanner.rows[scanner.rowHead] = position
		scanner.rowHead = (scanner.rowHead + 1) % len(scanner.rows)
	}
	scanner.totalRows++
}

func (scanner *viewerEndRowScanner) position(index int) viewerRowPosition {
	return scanner.rows[(scanner.rowHead+index)%len(scanner.rows)]
}

func (scanner *viewerEndRowScanner) result() (viewerEndResult, bool) {
	if scanner.rowCount == 0 {
		return viewerEndResult{}, false
	}
	topIndex := 0
	if scanner.totalRows >= int64(scanner.height) {
		topIndex = scanner.rowCount - scanner.height
	}
	buffer := semantic.SemanticWindowBufferRows(scanner.height)
	historyStart := max(0, topIndex-buffer)
	history := make([]viewerRowPosition, 0, topIndex-historyStart+1)
	for index := historyStart; index <= topIndex; index++ {
		history = append(history, scanner.position(index))
	}
	return viewerEndResult{top: scanner.position(topIndex), history: history}, true
}

func (scanner *viewerEndRowScanner) feed(base int64, data []byte, final bool) error {
	if scanner.started && base != scanner.nextOffset {
		return fmt.Errorf("non-contiguous viewer End scan at %d after %d", base, scanner.nextOffset)
	}
	scanner.started = true
	scanner.nextOffset = base + int64(len(data))

	scanData := data
	scanBase := base
	if scanner.carryLen > 0 {
		needed := scanner.carryLen + len(data)
		if cap(scanner.scratch) < needed {
			scanner.scratch = make([]byte, needed)
		} else {
			scanner.scratch = scanner.scratch[:needed]
		}
		copy(scanner.scratch, scanner.carry[:scanner.carryLen])
		copy(scanner.scratch[scanner.carryLen:], data)
		scanData = scanner.scratch
		scanBase = base - int64(scanner.carryLen)
		scanner.carryLen = 0
	}

	work := 0
	for pos := 0; pos < len(scanData); {
		if work >= viewerEndWorkSlice || pos == 0 {
			if err := scanner.ctx.Err(); err != nil {
				return err
			}
			work = 0
		}
		if !final && !utf8.FullRune(scanData[pos:]) {
			scanner.carryLen = copy(scanner.carry[:], scanData[pos:])
			break
		}
		r, size := utf8.DecodeRune(scanData[pos:])
		if size <= 0 {
			return io.ErrNoProgress
		}
		scanner.feedRune(scanBase+int64(pos), r, size)
		pos += size
		work += size
	}
	return nil
}

func (scanner *viewerEndRowScanner) feedRune(offset int64, r rune, runeSize int) {
	if r == '\n' {
		scanner.logicalColumn = 0
		scanner.widthInRow = 0
		next := offset + int64(runeSize)
		if next < scanner.size {
			scanner.appendRow(next, 0)
		}
		return
	}
	if r == '\r' {
		return
	}

	_, cells := vtui.SanitizeRune(r)
	if r == '\t' {
		cells = scanner.tabSize - scanner.logicalColumn%scanner.tabSize
	}
	cells = max(1, cells)
	if scanner.wrap && scanner.widthInRow > 0 && scanner.widthInRow+cells > scanner.width {
		scanner.appendRow(offset, scanner.logicalColumn)
		scanner.widthInRow = 0
	}
	scanner.widthInRow += cells
	scanner.logicalColumn += cells
}

func feedViewerEndData(scanner *viewerEndRowScanner, base int64, data []byte) error {
	if len(data) == 0 {
		return scanner.feed(base, nil, true)
	}
	for start := 0; start < len(data); start += viewerEndReadSlice {
		end := min(len(data), start+viewerEndReadSlice)
		if err := scanner.feed(base+int64(start), data[start:end], end == len(data)); err != nil {
			return err
		}
	}
	return nil
}

// viewerFastEndOffset handles the normal case entirely from the one tail
// window. A non-BOF window is laid out only after its first newline, so an
// arbitrary byte boundary can never become a synthetic visual row.
func viewerFastEndOffset(ctx context.Context, data []byte, dataStart, size int64,
	width, height int, wrap bool, tabSize int,
) (viewerEndResult, bool, error) {
	anchor := dataStart
	if dataStart > 0 {
		newline := -1
		for start := 0; start < len(data); start += viewerEndWorkSlice {
			if err := ctx.Err(); err != nil {
				return viewerEndResult{}, false, err
			}
			end := min(len(data), start+viewerEndWorkSlice)
			for index, value := range data[start:end] {
				if value == '\n' {
					newline = start + index
					break
				}
			}
			if newline >= 0 {
				break
			}
		}
		if newline < 0 {
			return viewerEndResult{}, false, nil
		}
		anchor += int64(newline + 1)
		data = data[newline+1:]
		if anchor >= size {
			return viewerEndResult{}, false, nil
		}
	}
	scanner := newViewerEndRowScanner(ctx, size, anchor, width, height, wrap, tabSize)
	if err := feedViewerEndData(scanner, anchor, data); err != nil {
		return viewerEndResult{}, false, err
	}
	if scanner.totalRows < int64(height) && dataStart > 0 {
		return viewerEndResult{}, false, nil
	}
	result, ok := scanner.result()
	return result, ok, nil
}

// viewerEndLogicalAnchor finds the first of the final height logical lines.
// It scans backward in cancellable fixed-size slices and returns only BOF or a
// byte immediately following a real newline.
func viewerEndLogicalAnchor(ctx context.Context, backend *ViewerBackend,
	size int64, height int, tailStart int64, tail []byte,
) (int64, error) {
	remaining := height
	scanBackward := func(data []byte, base int64) (int64, bool, error) {
		for end := len(data); end > 0; {
			start := max(0, end-viewerEndReadSlice)
			for index := end - 1; index >= start; index-- {
				if (end-1-index)%viewerEndWorkSlice == 0 {
					if err := ctx.Err(); err != nil {
						return 0, false, err
					}
				}
				absolute := base + int64(index)
				// A terminal newline ends the last real line; it does not
				// create an empty row at EOF.
				if data[index] == '\n' && absolute+1 < size {
					remaining--
					if remaining == 0 {
						return absolute + 1, true, nil
					}
				}
			}
			end = start
		}
		return 0, false, nil
	}

	if anchor, found, err := scanBackward(tail, tailStart); found || err != nil {
		return anchor, err
	}
	buffer := make([]byte, viewerEndReadSlice)
	for cursor := tailStart; cursor > 0; {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		start := max(int64(0), cursor-viewerEndReadSlice)
		length := int(cursor - start)
		n, err := fileops.ReadDocumentBytes(ctx, backend.File, buffer[:length], start)
		if err != nil {
			return 0, err
		}
		if n != length {
			return 0, io.ErrUnexpectedEOF
		}
		if anchor, found, err := scanBackward(buffer[:n], start); found || err != nil {
			return anchor, err
		}
		cursor = start
	}
	return 0, nil
}

func viewerEndOffsetFromAnchor(ctx context.Context, backend *ViewerBackend,
	anchor, tailStart, size int64, tail []byte, width, height int, wrap bool, tabSize int,
) (viewerEndResult, error) {
	if !wrap {
		return viewerEndResult{top: viewerRowPosition{offset: anchor}}, nil
	}
	scanner := newViewerEndRowScanner(ctx, size, anchor, width, height, true, tabSize)
	buffer := make([]byte, viewerEndReadSlice)
	for cursor := anchor; cursor < tailStart; {
		if err := ctx.Err(); err != nil {
			return viewerEndResult{}, err
		}
		length := int(min(int64(viewerEndReadSlice), tailStart-cursor))
		n, err := fileops.ReadDocumentBytes(ctx, backend.File, buffer[:length], cursor)
		if err != nil {
			return viewerEndResult{}, err
		}
		if n != length {
			return viewerEndResult{}, io.ErrUnexpectedEOF
		}
		if err := scanner.feed(cursor, buffer[:n], false); err != nil {
			return viewerEndResult{}, err
		}
		cursor += int64(n)
	}
	tailIndex := int(max(int64(0), anchor-tailStart))
	if err := feedViewerEndData(scanner, max(anchor, tailStart), tail[tailIndex:]); err != nil {
		return viewerEndResult{}, err
	}
	result, ok := scanner.result()
	if !ok {
		return viewerEndResult{}, io.ErrUnexpectedEOF
	}
	return result, nil
}

func (vv *ViewerView) seedViewerEndWrapSeek(result viewerEndResult, width int) {
	limit := semantic.SemanticWindowBufferRows(max(1, vv.viewportHeight())) + 1
	history := make([]viewerRowPosition, limit)
	seek := SemanticWrapSeekState{
		ready:          true,
		target:         result.top.offset,
		width:          width,
		curr:           result.top.offset,
		resolved:       result.top.offset,
		currColumn:     result.top.column,
		resolvedColumn: result.top.column,
		lineStartReady: true,
		history:        history,
	}
	start := max(0, len(result.history)-limit)
	for _, position := range result.history[start:] {
		seek.appendHistory(position.offset, position.column)
	}
	vv.SemanticWrapSeek = seek
}
