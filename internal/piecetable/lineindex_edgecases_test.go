package piecetable

import "testing"

// This file targets the corners of lineindex.go that the equivalence and
// stress tests in lineindex_equivalence_test.go and lineindex_test.go do not
// reach on their own: the defensive branches (empty ranges, out-of-range
// arguments, no-op edits) and the block-splice mechanics (rebase, the wide
// fallback, splitIfLarge) that only trigger under specific, hard-to-randomize
// layouts. Several tests build a *LineIndex directly from a literal instead of
// through Rebuild/AppendOffsets, since some of these states — a block whose
// first entry is not yet zero, a range that falls entirely between two blocks
// — are transient and never observable through the exported API alone.

func TestLineIndex_rebase(t *testing.T) {
	tests := []struct {
		name        string
		blk         lineBlock
		wantBase    int
		wantEntries []uint32
	}{
		{
			name:        "empty block is left alone",
			blk:         lineBlock{firstLine: 0, base: 7, entries: nil},
			wantBase:    7,
			wantEntries: nil,
		},
		{
			name:        "first entry already zero is a no-op",
			blk:         lineBlock{firstLine: 0, base: 10, entries: []uint32{0, 4, 9}},
			wantBase:    10,
			wantEntries: []uint32{0, 4, 9},
		},
		{
			name:        "non-zero first entry shifts into the base",
			blk:         lineBlock{firstLine: 0, base: 100, entries: []uint32{5, 12, 30}},
			wantBase:    105,
			wantEntries: []uint32{0, 7, 25},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Li := &LineIndex{blocks: []lineBlock{tt.blk}}
			if ok := Li.rebase(0); !ok {
				t.Fatal("rebase reported failure")
			}
			got := Li.blocks[0]
			if got.base != tt.wantBase {
				t.Errorf("base = %d, want %d", got.base, tt.wantBase)
			}
			if len(got.entries) != len(tt.wantEntries) {
				t.Fatalf("entries = %v, want %v", got.entries, tt.wantEntries)
			}
			for i, w := range tt.wantEntries {
				if got.entries[i] != w {
					t.Errorf("entries[%d] = %d, want %d", i, got.entries[i], w)
				}
			}
		})
	}
}

// TestLineIndex_shiftFrom_negativeLineClampsAndMovesBase covers the two
// branches shiftFrom takes only through direct, out-of-range calls: no public
// entry point ever passes it a negative line, and every incremental edit that
// lands exactly on a block's first line goes through UpdateAfterInsert/Delete,
// which the stress tests happen never to align on a boundary during 20 runs.
func TestLineIndex_shiftFrom_negativeLineClampsAndMovesBase(t *testing.T) {
	Li := &LineIndex{
		blocks: []lineBlock{{firstLine: 0, base: 0, entries: []uint32{0, 5, 10}}},
		count:  3,
	}

	Li.shiftFrom(-4, 3)

	want := []int{3, 8, 13}
	for i, w := range want {
		if got := Li.GetLineOffset(i); got != w {
			t.Errorf("GetLineOffset(%d) = %d, want %d", i, got, w)
		}
	}
}

func TestLineIndex_insertLines_edgeCases(t *testing.T) {
	t.Run("no values is a no-op", func(t *testing.T) {
		Li := NewLineIndex()
		before := Li.count
		Li.insertLines(0, nil)
		if Li.count != before {
			t.Errorf("count changed on an empty insert: got %d, want %d", Li.count, before)
		}
	})

	t.Run("wide layout splices into the flat slice", func(t *testing.T) {
		Li := &LineIndex{useWide: true, wide: []int{0, 10, 20}, count: 3}
		Li.insertLines(1, []int{3, 6})

		want := []int{0, 3, 6, 10, 20}
		if Li.count != len(want) {
			t.Fatalf("count = %d, want %d", Li.count, len(want))
		}
		for i, w := range want {
			if got := Li.wide[i]; got != w {
				t.Errorf("wide[%d] = %d, want %d", i, got, w)
			}
		}
	})

	t.Run("position past the block's entries clamps to the end", func(t *testing.T) {
		Li := &LineIndex{
			blocks: []lineBlock{{firstLine: 0, base: 0, entries: []uint32{0, 5}}},
			count:  2,
		}
		Li.insertLines(10, []int{7})

		want := []int{0, 5, 7}
		for i, w := range want {
			if got := Li.GetLineOffset(i); got != w {
				t.Errorf("GetLineOffset(%d) = %d, want %d", i, got, w)
			}
		}
	})

	t.Run("value too far from the block base falls back to wide", func(t *testing.T) {
		// Built rather than declared as a literal so this still compiles where
		// int is 32 bits, mirroring TestLineIndex_FallsBackPastFourGigabytes.
		twoToThe32 := 1
		for i := 0; i < 32; i++ {
			twoToThe32 *= 2
		}
		if twoToThe32 <= 0 {
			t.Skip("needs 64-bit ints")
		}
		big := twoToThe32 + 1000 // past maxRelativeOffset relative to base 0

		Li := &LineIndex{
			blocks: []lineBlock{{firstLine: 0, base: 0, entries: []uint32{0}}},
			count:  1,
		}
		Li.insertLines(1, []int{big})

		if !Li.useWide {
			t.Fatal("expected the insert to switch the index to the wide layout")
		}
		want := []int{0, big}
		if Li.count != len(want) {
			t.Fatalf("count = %d, want %d", Li.count, len(want))
		}
		for i, w := range want {
			if got := Li.wide[i]; got != w {
				t.Errorf("wide[%d] = %d, want %d", i, got, w)
			}
		}
	})
}

// TestLineIndex_UpdateAfterInsert_SplitsOversizedBlock drives splitIfLarge
// through the public API: pasting a run of blank lines longer than
// lineBlockMax in one go is exactly the real-world trigger the function's own
// comment describes ("a block that repeated inserts have grown"), just done in
// a single splice instead of many.
func TestLineIndex_UpdateAfterInsert_SplitsOversizedBlock(t *testing.T) {
	Li := NewLineIndex()

	const newlines = lineBlockMax + 50
	data := make([]byte, newlines)
	for i := range data {
		data[i] = '\n'
	}
	Li.UpdateAfterInsert(0, data)

	if got, want := Li.LineCount(), newlines+1; got != want {
		t.Fatalf("LineCount() = %d, want %d", got, want)
	}
	if len(Li.blocks) < 2 {
		t.Fatalf("expected splitIfLarge to split the oversized block, got %d block(s)", len(Li.blocks))
	}
	for _, line := range []int{0, 1, lineBlockMax / 2, lineBlockMax, lineBlockMax + 49, newlines} {
		if got, want := Li.GetLineOffset(line), line; got != want {
			t.Errorf("GetLineOffset(%d) = %d, want %d", line, got, want)
		}
	}
}

func TestLineIndex_removeLines_edgeCases(t *testing.T) {
	t.Run("empty range is a no-op", func(t *testing.T) {
		Li := &LineIndex{
			blocks: []lineBlock{{firstLine: 0, base: 0, entries: []uint32{0, 5, 10}}},
			count:  3,
		}
		Li.removeLines(2, 2)
		if Li.count != 3 || len(Li.blocks[0].entries) != 3 {
			t.Errorf("removeLines(2, 2) mutated the index")
		}
	})

	t.Run("a block emptied by the removal is dropped", func(t *testing.T) {
		Li := &LineIndex{
			blocks: []lineBlock{
				{firstLine: 0, base: 0, entries: []uint32{0, 10, 20}},  // lines 0-2
				{firstLine: 3, base: 30, entries: []uint32{0, 10}},     // lines 3-4
				{firstLine: 5, base: 60, entries: []uint32{0, 10, 20}}, // lines 5-7
			},
			count: 8,
		}

		// [3, 5) covers exactly the middle block and nothing else, so that
		// block has to disappear from Li.blocks rather than end up empty.
		Li.removeLines(3, 5)

		if Li.count != 6 {
			t.Fatalf("count = %d, want 6", Li.count)
		}
		if len(Li.blocks) != 2 {
			t.Fatalf("expected the emptied block to be dropped, got %d block(s)", len(Li.blocks))
		}
		want := []int{0, 10, 20, 60, 70, 80}
		for i, w := range want {
			if got := Li.GetLineOffset(i); got != w {
				t.Errorf("GetLineOffset(%d) = %d, want %d", i, got, w)
			}
		}
	})

	t.Run("a start past every block is skipped without panicking", func(t *testing.T) {
		Li := &LineIndex{
			blocks: []lineBlock{{firstLine: 0, base: 0, entries: []uint32{0, 5, 10}}},
			count:  3,
		}
		Li.removeLines(50, 60)
		if Li.count != 3 || len(Li.blocks[0].entries) != 3 {
			t.Errorf("removeLines with an out-of-range start mutated the index")
		}
	})
}

// TestLineIndex_getLineAtOffset_beforeFirstBlock covers the defensive branch
// for an offset that lands before the first block's base — unreachable
// through the public API, where the first block always starts at zero, but
// worth guarding since getLineAtOffset is also reached from AppendOffsets on
// an index built by something other than Rebuild.
func TestLineIndex_getLineAtOffset_beforeFirstBlock(t *testing.T) {
	Li := &LineIndex{
		blocks: []lineBlock{{firstLine: 0, base: 100, entries: []uint32{0, 5}}},
		count:  2,
	}
	if got := Li.getLineAtOffset(50); got != 0 {
		t.Errorf("getLineAtOffset(50) = %d, want 0", got)
	}
}

func TestLineIndex_appendOffset_wideMode(t *testing.T) {
	Li := &LineIndex{useWide: true, wide: []int{0}, count: 1}

	Li.appendOffset(100)
	Li.appendOffset(250)

	if Li.count != 3 {
		t.Fatalf("count = %d, want 3", Li.count)
	}
	want := []int{0, 100, 250}
	for i, w := range want {
		if got := Li.wide[i]; got != w {
			t.Errorf("wide[%d] = %d, want %d", i, got, w)
		}
	}
	if len(Li.blocks) != 0 {
		t.Errorf("appendOffset in wide mode must not touch blocks, got %d", len(Li.blocks))
	}
}

func TestLineIndex_UpdateAfterInsert_EmptyDataIsNoOp(t *testing.T) {
	Pt := New([]byte("Line 1\nLine 2"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	before := Li.LineCount()
	Li.UpdateAfterInsert(3, nil)
	if Li.LineCount() != before {
		t.Errorf("LineCount changed after inserting no data: got %d, want %d", Li.LineCount(), before)
	}
}

func TestLineIndex_UpdateAfterDelete_ZeroLengthIsNoOp(t *testing.T) {
	Pt := New([]byte("Line 1\nLine 2"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	before := Li.LineCount()
	Li.UpdateAfterDelete(3, 0)
	if Li.LineCount() != before {
		t.Errorf("LineCount changed after deleting zero bytes: got %d, want %d", Li.LineCount(), before)
	}
}
