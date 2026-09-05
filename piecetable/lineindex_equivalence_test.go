package piecetable

import (
	"math/rand"
	"testing"
)

// denseLineIndex is the index as it was before the blocked layout: one int per
// line, absolute, spliced with append. It is the oracle the real one is checked
// against, so a rewrite of the storage cannot quietly change an answer.
type denseLineIndex struct {
	offsets []int
}

func newDenseLineIndex() *denseLineIndex {
	return &denseLineIndex{offsets: []int{0}}
}

func (d *denseLineIndex) lineCount() int { return len(d.offsets) }

func (d *denseLineIndex) lineOffset(line int) int {
	if line < 0 || line >= len(d.offsets) {
		return -1
	}
	return d.offsets[line]
}

func (d *denseLineIndex) lineAtOffset(offset int) int {
	if offset <= 0 {
		return 0
	}
	idx := 0
	for idx < len(d.offsets) && d.offsets[idx] <= offset {
		idx++
	}
	return idx - 1
}

func (d *denseLineIndex) appendOffsets(offsets []int, maxAllowed int) {
	last := d.offsets[len(d.offsets)-1]
	for _, off := range offsets {
		if off > last && off <= maxAllowed {
			d.offsets = append(d.offsets, off)
			last = off
		}
	}
}

func (d *denseLineIndex) updateAfterInsert(offset int, data []byte) {
	if len(data) == 0 {
		return
	}
	lineIdx := d.lineAtOffset(offset)

	var newOffsets []int
	currentOffset := offset
	for _, b := range data {
		currentOffset++
		if b == '\n' {
			newOffsets = append(newOffsets, currentOffset)
		}
	}
	for i := lineIdx + 1; i < len(d.offsets); i++ {
		d.offsets[i] += len(data)
	}
	if len(newOffsets) > 0 {
		tail := append(newOffsets, d.offsets[lineIdx+1:]...)
		d.offsets = append(d.offsets[:lineIdx+1], tail...)
	}
}

func (d *denseLineIndex) updateAfterDelete(offset, length int) {
	if length == 0 {
		return
	}
	startLine := d.lineAtOffset(offset)
	endLine := d.lineAtOffset(offset + length)
	linesRemoved := endLine - startLine
	for i := endLine + 1; i < len(d.offsets); i++ {
		d.offsets[i] -= length
	}
	if linesRemoved > 0 {
		d.offsets = append(d.offsets[:startLine+1], d.offsets[endLine+1:]...)
	}
}

// assertSameIndex compares every line offset — that is the cheap direction —
// and probes the reverse mapping around the places it can go wrong: the ends,
// the block boundaries, and each line's own edges, plus a random sample. Walking
// every byte offset of a 100 KB file on every step would be exhaustive and take
// a minute and a half to say the same thing.
func assertSameIndex(t *testing.T, rng *rand.Rand, step int, got *LineIndex, want *denseLineIndex) {
	t.Helper()
	if got.LineCount() != want.lineCount() {
		t.Fatalf("step %d: line count = %d, want %d", step, got.LineCount(), want.lineCount())
	}
	for i := -1; i <= want.lineCount(); i++ {
		if g, w := got.GetLineOffset(i), want.lineOffset(i); g != w {
			t.Fatalf("step %d: GetLineOffset(%d) = %d, want %d", step, i, g, w)
		}
	}

	last := want.lineOffset(want.lineCount() - 1)
	probes := []int{-2, -1, 0, 1, last - 1, last, last + 1, last + 32}
	for _, line := range []int{1, lineBlockTarget - 1, lineBlockTarget, lineBlockTarget + 1, 2 * lineBlockTarget} {
		if off := want.lineOffset(line); off >= 0 {
			probes = append(probes, off-1, off, off+1)
		}
	}
	for i := 0; i < 200; i++ {
		probes = append(probes, rng.Intn(last+8))
	}
	for _, off := range probes {
		if g, w := got.GetLineAtOffset(off), want.lineAtOffset(off); g != w {
			t.Fatalf("step %d: GetLineAtOffset(%d) = %d, want %d", step, off, g, w)
		}
	}
}

// TestLineIndex_MatchesDenseImplementation drives both indexes through the same
// random edit stream. Insertions and deletions with and without newlines are
// what the blocked layout has to get right: an insert shifts the bases of every
// later block, and one that adds lines renumbers them as well.
func TestLineIndex_MatchesDenseImplementation(t *testing.T) {
	rng := rand.New(rand.NewSource(20260815)) // #nosec G404 -- a fixed seed makes this randomized equivalence test reproducible; no security decision uses it.

	for round := 0; round < 20; round++ {
		li := NewLineIndex()
		dense := newDenseLineIndex()

		// Start from a file of a few thousand lines, so several blocks exist
		// and edits land in the middle of one.
		size := 0
		var seed []int
		for i := 0; i < 5000; i++ {
			size += 1 + rng.Intn(40)
			seed = append(seed, size)
		}
		li.AppendOffsets(seed, size+1)
		dense.appendOffsets(seed, size+1)
		assertSameIndex(t, rng, -1, li, dense)

		total := size + 1
		for step := 0; step < 60; step++ {
			switch rng.Intn(3) {
			case 0: // insert without a newline
				at := rng.Intn(total)
				data := make([]byte, 1+rng.Intn(8))
				for i := range data {
					data[i] = 'a'
				}
				li.UpdateAfterInsert(at, data)
				dense.updateAfterInsert(at, data)
				total += len(data)
			case 1: // insert with newlines
				at := rng.Intn(total)
				data := []byte("one\ntwo\nthree\n")
				if rng.Intn(2) == 0 {
					data = []byte("\n")
				}
				li.UpdateAfterInsert(at, data)
				dense.updateAfterInsert(at, data)
				total += len(data)
			default: // delete
				if total < 4 {
					continue
				}
				at := rng.Intn(total - 2)
				length := 1 + rng.Intn(min(64, total-at-1))
				li.UpdateAfterDelete(at, length)
				dense.updateAfterDelete(at, length)
				total -= length
			}
			assertSameIndex(t, rng, step, li, dense)
		}
	}
}

// TestLineIndex_SpansManyBlocks exercises the block boundaries directly: the
// interesting lines are the ones at the edges, where a lookup crosses from one
// base to the next.
func TestLineIndex_SpansManyBlocks(t *testing.T) {
	li := NewLineIndex()

	const lines = lineBlockTarget*3 + 7
	offsets := make([]int, 0, lines)
	for i := 1; i <= lines; i++ {
		offsets = append(offsets, i*10)
	}
	li.AppendOffsets(offsets, lines*10)

	if got := li.LineCount(); got != lines+1 {
		t.Fatalf("line count = %d, want %d", got, lines+1)
	}
	for _, line := range []int{0, 1, lineBlockTarget - 1, lineBlockTarget, lineBlockTarget + 1, 2*lineBlockTarget - 1, 2 * lineBlockTarget, lines} {
		want := line * 10
		if got := li.GetLineOffset(line); got != want {
			t.Errorf("GetLineOffset(%d) = %d, want %d", line, got, want)
		}
		if got := li.GetLineAtOffset(want); got != line {
			t.Errorf("GetLineAtOffset(%d) = %d, want %d", want, got, line)
		}
	}

	// An insert in the first block has to move every later block's base.
	li.UpdateAfterInsert(5, []byte("xx"))
	if got, want := li.GetLineOffset(lines), lines*10+2; got != want {
		t.Errorf("after insert, GetLineOffset(%d) = %d, want %d", lines, got, want)
	}
	// And a delete has to move them back, including entries that end up before
	// the base their block started with.
	li.UpdateAfterDelete(5, 2)
	if got, want := li.GetLineOffset(lines), lines*10; got != want {
		t.Errorf("after delete, GetLineOffset(%d) = %d, want %d", lines, got, want)
	}
}

// TestLineIndex_FallsBackPastFourGigabytes covers the layout switch: an offset
// too far from its block's base to fit in 32 bits moves the whole index to
// absolute ints, and every answer has to survive the move.
func TestLineIndex_FallsBackPastFourGigabytes(t *testing.T) {
	// Built rather than declared as a constant: converting one that large to
	// int does not compile at all where int is 32 bits, and there the fallback
	// is unreachable anyway.
	huge := 1
	for i := 0; i < 33; i++ {
		huge *= 2
	}
	if huge <= 0 {
		t.Skip("needs 64-bit ints")
	}

	li := NewLineIndex()
	offsets := []int{10, 20, huge, huge + 10}
	li.AppendOffsets(offsets, huge+1000)

	if got := li.LineCount(); got != 5 {
		t.Fatalf("line count = %d, want 5", got)
	}
	for i, want := range []int{0, 10, 20, huge, huge + 10} {
		if got := li.GetLineOffset(i); got != want {
			t.Errorf("GetLineOffset(%d) = %d, want %d", i, got, want)
		}
	}
	if got := li.GetLineAtOffset(huge + 5); got != 3 {
		t.Errorf("GetLineAtOffset past 4 GB = %d, want 3", got)
	}

	// Edits keep working in the wide layout.
	li.UpdateAfterInsert(0, []byte("ab"))
	if got, want := li.GetLineOffset(4), huge+12; got != want {
		t.Errorf("after insert, GetLineOffset(4) = %d, want %d", got, want)
	}
}

// TestLineIndex_DeleteAcrossManyBlocks covers the case the random walk rarely
// reaches: a deletion that collapses whole blocks rather than a few lines
// inside one, which has to drop those blocks and renumber what follows.
func TestLineIndex_DeleteAcrossManyBlocks(t *testing.T) {
	li := NewLineIndex()
	dense := newDenseLineIndex()

	const lines = lineBlockTarget * 5
	offsets := make([]int, 0, lines)
	for i := 1; i <= lines; i++ {
		offsets = append(offsets, i*8)
	}
	li.AppendOffsets(offsets, lines*8)
	dense.appendOffsets(offsets, lines*8)

	// Remove the middle three blocks' worth of text in one go.
	from := lineBlockTarget * 8
	length := lineBlockTarget * 3 * 8
	li.UpdateAfterDelete(from, length)
	dense.updateAfterDelete(from, length)

	if got, want := li.LineCount(), dense.lineCount(); got != want {
		t.Fatalf("line count = %d, want %d", got, want)
	}
	for i := 0; i < dense.lineCount(); i++ {
		if got, want := li.GetLineOffset(i), dense.lineOffset(i); got != want {
			t.Fatalf("GetLineOffset(%d) = %d, want %d", i, got, want)
		}
	}
	last := dense.lineOffset(dense.lineCount() - 1)
	for off := 0; off <= last+16; off += 3 {
		if got, want := li.GetLineAtOffset(off), dense.lineAtOffset(off); got != want {
			t.Fatalf("GetLineAtOffset(%d) = %d, want %d", off, got, want)
		}
	}
}
