package piecetable

import (
	"bytes"
	"sort"
	"sync"
)

const (
	// lineBlockTarget is how many line offsets a block holds when it is filled
	// or split. Small enough that rewriting one is cheap, large enough that a
	// file of millions of lines has thousands of blocks rather than millions.
	lineBlockTarget = 4096
	// lineBlockMax is the size a block is split at, so that repeated inserts
	// into the same place cannot grow one block without bound.
	lineBlockMax = 2 * lineBlockTarget

	// maxRelativeOffset is what fits in the uint32 an entry is stored as.
	maxRelativeOffset = 1<<32 - 1
)

// fitsRelativeOffset reports whether a distance from a block's base can be
// stored in the uint32 an entry is. The comparison goes through uint64 because
// the limit does not fit in an int on a 32-bit build, where it is also
// unreachable: no offset there can exceed 2 GB in the first place.
func fitsRelativeOffset(rel int) bool {
	return rel >= 0 && uint64(rel) <= maxRelativeOffset
}

// lineBlock holds the offsets of a run of consecutive lines. Each offset is
// stored as a distance from base — the absolute offset of the block's first
// line — so entries[0] is always zero and an edit before the block moves all
// of it by moving base alone.
type lineBlock struct {
	firstLine int
	base      int
	entries   []uint32
}

// LineIndex stores the byte offset each line starts at.
//
// The offsets live in blocks of a few thousand, each entry held relative to the
// offset its block begins at. Three things follow, and all of them matter on a
// file with millions of lines:
//
//   - four bytes per line instead of eight, and filling the index allocates one
//     more block instead of reallocating and copying the whole array — 21M
//     lines cost 80 MB where a doubling []int cost 330 MB;
//   - an edit that adds no lines shifts everything after it by adjusting one
//     base per block, touching thousands of values instead of millions;
//   - an edit that does add lines splices into a single block and renumbers the
//     blocks after it, rather than moving every offset in the file.
//
// Offsets past 4 GB cannot be described in 32 bits relative to anything, so a
// file that large falls back to the plain []int this used to be for every file.
// Both layouts answer through the same helpers, and the exported behaviour is
// identical either way.
type LineIndex struct {
	mu     sync.RWMutex
	blocks []lineBlock
	count  int

	// wide is the fallback layout: absolute offsets, one int each.
	wide    []int
	useWide bool
}

// NewLineIndex creates a new empty index.
func NewLineIndex() *LineIndex {
	Li := &LineIndex{}
	Li.reset()
	return Li
}

// reset returns the index to a single line starting at zero.
func (Li *LineIndex) reset() {
	Li.blocks = nil
	Li.wide = nil
	Li.useWide = false
	Li.count = 0
	Li.appendOffset(0)
}

// blockForLine returns the index of the block holding the given line, which the
// caller has already bounds-checked.
func (Li *LineIndex) blockForLine(line int) int {
	idx := sort.Search(len(Li.blocks), func(i int) bool {
		return Li.blocks[i].firstLine > line
	})
	return idx - 1
}

// at returns the absolute offset of line i.
func (Li *LineIndex) at(i int) int {
	if Li.useWide {
		return Li.wide[i]
	}
	b := Li.blockForLine(i)
	blk := &Li.blocks[b]
	return blk.base + int(blk.entries[i-blk.firstLine])
}

// appendOffset adds one offset to the end, opening a new block when the last
// one is full or when the offset is too far from its base to encode.
func (Li *LineIndex) appendOffset(off int) {
	if Li.useWide {
		Li.wide = append(Li.wide, off)
		Li.count++
		return
	}

	if len(Li.blocks) > 0 {
		blk := &Li.blocks[len(Li.blocks)-1]
		rel := off - blk.base
		if len(blk.entries) < lineBlockTarget && fitsRelativeOffset(rel) {
			// #nosec G115 -- fitsRelativeOffset proves rel is in the uint32 range.
			blk.entries = append(blk.entries, uint32(rel))
			Li.count++
			return
		}
	}

	// A fresh block starts at the offset it is given, so its first entry is
	// always representable.
	Li.blocks = append(Li.blocks, lineBlock{
		firstLine: Li.count,
		base:      off,
		entries:   append(make([]uint32, 0, lineBlockTarget), 0),
	})
	Li.count++
}

// switchToWide flattens the blocks into absolute offsets. It runs at most once
// per index, for a file no relative encoding can describe.
func (Li *LineIndex) switchToWide() {
	wide := make([]int, Li.count)
	for b := range Li.blocks {
		blk := &Li.blocks[b]
		for i, rel := range blk.entries {
			wide[blk.firstLine+i] = blk.base + int(rel)
		}
	}
	Li.wide = wide
	Li.blocks = nil
	Li.useWide = true
}

// rebase recomputes a block's base from its first entry, which is what keeps
// entries[0] at zero after a splice or a shift has moved it.
func (Li *LineIndex) rebase(b int) bool {
	blk := &Li.blocks[b]
	if len(blk.entries) == 0 || blk.entries[0] == 0 {
		return true
	}
	shift := blk.entries[0]
	for i := range blk.entries {
		blk.entries[i] -= shift
	}
	blk.base += int(shift)
	return true
}

// shiftFrom adds delta to every offset from line onwards. Whole blocks move by
// their base; only the block the line falls inside has its entries touched.
func (Li *LineIndex) shiftFrom(line, delta int) {
	if delta == 0 || line >= Li.count {
		return
	}
	if line < 0 {
		line = 0
	}
	if Li.useWide {
		for i := line; i < Li.count; i++ {
			Li.wide[i] += delta
		}
		return
	}

	b := Li.blockForLine(line)
	blk := &Li.blocks[b]
	pos := line - blk.firstLine
	if pos == 0 {
		blk.base += delta
	} else {
		for i := pos; i < len(blk.entries); i++ {
			rel := int(blk.entries[i]) + delta
			if !fitsRelativeOffset(rel) {
				Li.switchToWide()
				Li.shiftFrom(line, delta)
				return
			}
			// #nosec G115 -- fitsRelativeOffset above proves rel is in the uint32 range.
			blk.entries[i] = uint32(rel)
		}
	}
	for j := b + 1; j < len(Li.blocks); j++ {
		Li.blocks[j].base += delta
	}
}

// insertLines splices offsets in before the given line. Only the block that
// receives them is rewritten; the blocks after it just learn that their lines
// are numbered higher than they were.
func (Li *LineIndex) insertLines(at int, vals []int) {
	if len(vals) == 0 {
		return
	}
	if Li.useWide {
		Li.wide = append(Li.wide, make([]int, len(vals))...)
		copy(Li.wide[at+len(vals):], Li.wide[at:Li.count])
		copy(Li.wide[at:], vals)
		Li.count += len(vals)
		return
	}

	b := Li.blockForLine(min(at, Li.count-1))
	blk := &Li.blocks[b]
	pos := at - blk.firstLine
	if pos > len(blk.entries) {
		pos = len(blk.entries)
	}

	rel := make([]uint32, len(vals))
	for i, v := range vals {
		r := v - blk.base
		if !fitsRelativeOffset(r) {
			Li.switchToWide()
			Li.insertLines(at, vals)
			return
		}
		// #nosec G115 -- fitsRelativeOffset above proves r is in the uint32 range.
		rel[i] = uint32(r)
	}

	blk.entries = append(blk.entries, make([]uint32, len(vals))...)
	copy(blk.entries[pos+len(vals):], blk.entries[pos:])
	copy(blk.entries[pos:], rel)

	Li.count += len(vals)
	for j := b + 1; j < len(Li.blocks); j++ {
		Li.blocks[j].firstLine += len(vals)
	}
	Li.rebase(b)
	Li.splitIfLarge(b)
}

// splitIfLarge halves a block that repeated inserts have grown, so that the
// per-insert cost stays bounded by the block size.
func (Li *LineIndex) splitIfLarge(b int) {
	blk := &Li.blocks[b]
	if len(blk.entries) <= lineBlockMax {
		return
	}
	half := len(blk.entries) / 2
	tail := lineBlock{
		firstLine: blk.firstLine + half,
		base:      blk.base + int(blk.entries[half]),
		entries:   append(make([]uint32, 0, len(blk.entries)-half), blk.entries[half:]...),
	}
	shift := tail.base - blk.base
	for i := range tail.entries {
		// #nosec G115 -- shift is the non-negative difference between two offsets encoded in this uint32 block.
		tail.entries[i] -= uint32(shift)
	}
	blk.entries = blk.entries[:half]

	Li.blocks = append(Li.blocks, lineBlock{})
	copy(Li.blocks[b+2:], Li.blocks[b+1:])
	Li.blocks[b+1] = tail
}

// removeLines drops the lines in [from, to), touching only the blocks they fall
// in and renumbering the ones after.
func (Li *LineIndex) removeLines(from, to int) {
	if to <= from {
		return
	}
	if Li.useWide {
		copy(Li.wide[from:], Li.wide[to:Li.count])
		Li.count -= to - from
		Li.wide = Li.wide[:Li.count]
		return
	}

	removed := 0
	first := Li.blockForLine(from)
	for b := first; b < len(Li.blocks) && removed < to-from; {
		blk := &Li.blocks[b]
		start := max(from-blk.firstLine, 0)
		end := min(to-blk.firstLine, len(blk.entries))
		if start >= end {
			b++
			continue
		}
		blk.entries = append(blk.entries[:start], blk.entries[end:]...)
		removed += end - start
		if len(blk.entries) == 0 {
			Li.blocks = append(Li.blocks[:b], Li.blocks[b+1:]...)
			continue
		}
		Li.rebase(b)
		b++
	}

	Li.count -= removed
	// Every block after the first one touched has lost the same lines, so a
	// single pass fixes the numbering.
	line := 0
	for b := range Li.blocks {
		Li.blocks[b].firstLine = line
		line += len(Li.blocks[b].entries)
	}
}

// AppendNewlineOffsets appends the offset just past every newline in data to
// dst, counting from base. That offset is where the next line starts, which is
// what a line index holds — and finding it is the one loop every scanner in
// the editor runs, so they all run this one.
func AppendNewlineOffsets(dst []int, data []byte, base int) []int {
	for pos := 0; pos < len(data); {
		idx := bytes.IndexByte(data[pos:], '\n')
		if idx < 0 {
			break
		}
		pos += idx + 1
		dst = append(dst, base+pos)
	}
	return dst
}

// Rebuild reconstructs the line index from the piece table, and reports
// whether it managed to walk the whole buffer.
//
// It can fail to: the walk goes through the piece table, which on a lazily
// loaded file answers "still loading" for a chunk that has not arrived, and
// stops there. The index is then correct as far as it goes and short after
// that, and the caller is the only one in a position to know the difference —
// hence the return value. Claiming a short index is complete is worse than
// having one, because everything downstream believes it.
func (Li *LineIndex) Rebuild(Pt *PieceTable) bool {
	Li.mu.Lock()
	defer Li.mu.Unlock()
	// Reset index, first line always starts at 0
	Li.reset()

	if Pt.Size() == 0 {
		return true
	}

	absPos := 0
	offsets := make([]int, 0, 4096)
	err := Pt.ForEachRange(func(data []byte) error {
		offsets = AppendNewlineOffsets(offsets[:0], data, absPos)
		for _, off := range offsets {
			Li.appendOffset(off)
		}
		absPos += len(data)
		return nil
	})
	return err == nil && absPos >= Pt.Size()
}

// AppendOffsets adds pre-calculated line offsets (used by background indexer).
// It performs a safety check to ensure offsets are within reasonable bounds.
func (Li *LineIndex) AppendOffsets(offsets []int, maxAllowed int) {
	Li.mu.Lock()
	defer Li.mu.Unlock()
	lastOffset := Li.at(Li.count - 1)
	for _, off := range offsets {
		if off > lastOffset && off <= maxAllowed {
			Li.appendOffset(off)
			lastOffset = off
		}
	}
}

// LineCount returns total number of lines.
func (Li *LineIndex) LineCount() int {
	Li.mu.RLock()
	defer Li.mu.RUnlock()
	return Li.count
}

// GetLineOffset returns byte offset of the specified line start (0-based).
func (Li *LineIndex) GetLineOffset(line int) int {
	Li.mu.RLock()
	defer Li.mu.RUnlock()
	if line < 0 || line >= Li.count {
		return -1
	}
	return Li.at(line)
}

// GetLineAtOffset returns the line number (0-based) to which specified offset belongs.
// Uses binary search for O(log N) speed.
func (Li *LineIndex) getLineAtOffset(offset int) int {
	if offset <= 0 {
		return 0
	}
	if Li.useWide {
		idx := sort.Search(Li.count, func(i int) bool {
			return Li.wide[i] > offset
		})
		return idx - 1
	}

	// Find the block the offset falls in first, then the line inside it: two
	// short searches instead of one over every line in the file.
	b := sort.Search(len(Li.blocks), func(i int) bool {
		return Li.blocks[i].base > offset
	}) - 1
	if b < 0 {
		return 0
	}
	blk := &Li.blocks[b]
	rel := offset - blk.base
	idx := sort.Search(len(blk.entries), func(i int) bool {
		return int(blk.entries[i]) > rel
	})
	return blk.firstLine + idx - 1
}

func (Li *LineIndex) GetLineAtOffset(offset int) int {
	Li.mu.RLock()
	defer Li.mu.RUnlock()
	return Li.getLineAtOffset(offset)
}

// UpdateAfterInsert incrementally updates the index after data insertion.
func (Li *LineIndex) UpdateAfterInsert(offset int, data []byte) {
	Li.mu.Lock()
	defer Li.mu.Unlock()
	lenData := len(data)
	if lenData == 0 {
		return
	}

	// 1. Find the line where insertion occurred
	lineIdx := Li.getLineAtOffset(offset)

	// 2. Search for new line breaks in the inserted fragment
	var newOffsets []int
	currentOffset := offset
	for _, b := range data {
		currentOffset++
		if b == '\n' {
			newOffsets = append(newOffsets, currentOffset)
		}
	}

	// 3. Shift all subsequent offsets
	Li.shiftFrom(lineIdx+1, lenData)

	// 4. Insert new line offsets if any
	if len(newOffsets) > 0 {
		Li.insertLines(lineIdx+1, newOffsets)
	}
}

// UpdateAfterDelete incrementally updates the index after data deletion.
func (Li *LineIndex) UpdateAfterDelete(offset, length int) {
	Li.mu.Lock()
	defer Li.mu.Unlock()
	if length == 0 {
		return
	}

	startLine := Li.getLineAtOffset(offset)
	endLine := Li.getLineAtOffset(offset + length)

	// 1. Determine how many lines were removed
	linesRemoved := endLine - startLine

	// 2. Shift all subsequent offsets
	Li.shiftFrom(endLine+1, -length)

	// 3. Remove offsets of "collapsed" lines
	if linesRemoved > 0 {
		Li.removeLines(startLine+1, endLine+1)
	}
}
