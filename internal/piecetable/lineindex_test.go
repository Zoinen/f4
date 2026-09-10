package piecetable

import (
	"sync"
	"testing"
)

func TestLineIndex_Build(t *testing.T) {
	// Text:
	// Line 1 (6 bytes: L,i,n,e,1,\n)
	// Line 2 (6 bytes: L,i,n,e,2,\n)
	// Line 3 (5 bytes: L,i,n,e,3)
	Pt := New([]byte("Line 1\nLine 2\nLine 3"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	if Li.LineCount() != 3 {
		t.Errorf("Expected 3 lines, got %d", Li.LineCount())
	}

	// Check offsets
	if Li.GetLineOffset(0) != 0 {
		t.Errorf("Line 0 offset: expected 0, got %d", Li.GetLineOffset(0))
	}
	if Li.GetLineOffset(1) != 7 { // "Line 1\n" -> 7 bytes
		t.Errorf("Line 1 offset: expected 7, got %d", Li.GetLineOffset(1))
	}
	if Li.GetLineOffset(2) != 14 { // "Line 1\nLine 2\n" -> 14 bytes
		t.Errorf("Line 2 offset: expected 14, got %d", Li.GetLineOffset(2))
	}
}
func TestLineIndex_UpdateAfterDelete_SingleNewline(t *testing.T) {
	// Tests merging two lines by deleting the newline between them
	Pt := New([]byte("Line1\nLine2"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	// Delete '\n' at offset 5
	Pt.Delete(5, 1)
	Li.UpdateAfterDelete(5, 1)

	if Li.LineCount() != 1 {
		t.Errorf("Expected 1 line after merging, got %d", Li.LineCount())
	}
	if Li.GetLineOffset(0) != 0 {
		t.Errorf("Line 0 offset should be 0, got %d", Li.GetLineOffset(0))
	}
}

func TestLineIndex_GetLineAtOffset(t *testing.T) {
	Pt := New([]byte("AAA\nBBB\nCCC"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)
	// Offsets: [0, 4, 8]

	tests := []struct {
		offset int
		want   int
	}{
		{0, 0}, {1, 0}, {3, 0},
		{4, 1}, {5, 1}, {7, 1},
		{8, 2}, {10, 2},
	}

	for _, tt := range tests {
		got := Li.GetLineAtOffset(tt.offset)
		if got != tt.want {
			t.Errorf("At offset %d: expected line %d, got %d", tt.offset, tt.want, got)
		}
	}
}
func TestLineIndex_AppendAtEOF(t *testing.T) {
	// Check insertion at EOF without \n
	Pt := New([]byte("NoNewline"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	insertData := []byte(" + More")
	Pt.Insert(9, insertData)
	Li.UpdateAfterInsert(9, insertData)

	if Li.LineCount() != 1 {
		t.Errorf("Expected 1 line, got %d", Li.LineCount())
	}

	// Insert \n into the middle
	newline := []byte("\n")
	Pt.Insert(2, newline)
	Li.UpdateAfterInsert(2, newline)

	if Li.LineCount() != 2 {
		t.Errorf("Expected 2 lines after inserting newline, got %d", Li.LineCount())
	}
}

func TestLineIndex_Empty(t *testing.T) {
	Pt := New([]byte(""))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	if Li.LineCount() != 1 {
		t.Errorf("Empty file should have 1 line, got %d", Li.LineCount())
	}
	if Li.GetLineOffset(0) != 0 {
		t.Error("Line 0 offset should be 0 even for empty file")
	}
}

func TestLineIndex_DeepConsistency(t *testing.T) {
	// Check that a series of incremental updates gives the same result
	// as a full Rebuild.
	text := []byte("Line 1\nLine 2\nLine 3")
	Pt := New(text)
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	// 1. Insertion in the middle with a break
	insertData := []byte("New\nData")
	offset := 7 // Start of "Line 2"
	Pt.Insert(offset, insertData)
	Li.UpdateAfterInsert(offset, insertData)

	// 2. Deleting part of the text
	Pt.Delete(2, 10)
	Li.UpdateAfterDelete(2, 10)

	// Compare with the reference
	liExpected := NewLineIndex()
	liExpected.Rebuild(Pt)

	if Li.LineCount() != liExpected.LineCount() {
		t.Errorf("Consistency fail: LineCount %d != %d", Li.LineCount(), liExpected.LineCount())
	}

	for i := 0; i < Li.LineCount(); i++ {
		if Li.GetLineOffset(i) != liExpected.GetLineOffset(i) {
			t.Errorf("Consistency fail at line %d: offset %d != %d", i, Li.GetLineOffset(i), liExpected.GetLineOffset(i))
		}
	}
}
func TestLineIndex_IncrementalStress(t *testing.T) {
	// Pseudo-random index endurance test
	Pt := New([]byte("Initial Text\nLine 2\nLine 3"))
	Li := NewLineIndex()
	Li.Rebuild(Pt)

	ops := []struct {
		insert bool
		off    int
		data   string
	}{
		{true, 5, "!!!\n!!!"},
		{false, 2, "12345"}, // deleting 5 bytes from offset 2
		{true, 0, "\nStart\n"},
		{false, 10, "1"},
		{true, 15, "End"},
	}

	for i, op := range ops {
		if op.insert {
			data := []byte(op.data)
			Pt.Insert(op.off, data)
			Li.UpdateAfterInsert(op.off, data)
		} else {
			length := len(op.data)
			Pt.Delete(op.off, length)
			Li.UpdateAfterDelete(op.off, length)
		}

		// Comparison with honest Rebuild at each step
		liRef := NewLineIndex()
		liRef.Rebuild(Pt)

		if Li.LineCount() != liRef.LineCount() {
			t.Fatalf("Stress step %d: LineCount mismatch. Got %d, want %d", i, Li.LineCount(), liRef.LineCount())
		}
		for j := 0; j < Li.LineCount(); j++ {
			if Li.GetLineOffset(j) != liRef.GetLineOffset(j) {
				t.Fatalf("Stress step %d: Offset mismatch at line %d", i, j)
			}
		}
	}
}

func TestLineIndex_ConcurrentAccess(t *testing.T) {
	// Verifies thread safety and absence of self-deadlock.
	Li := NewLineIndex()
	var wg sync.WaitGroup

	// 1. Writer: periodically adds new offsets (simulates background indexer)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			Li.AppendOffsets([]int{i * 10}, 20000)
		}
	}()

	// 2. Reader: constantly queries the index
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = Li.LineCount()
			_ = Li.GetLineAtOffset(i * 5)
			_ = Li.GetLineOffset(0)
		}
	}()

	// 3. Mutator: performs edits (simulates UI thread)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			// This tests UpdateAfterInsert which used to cause self-deadlock
			Li.UpdateAfterInsert(0, []byte("data\n"))
		}
	}()

	wg.Wait()
}
