package piecetable

import (
	"reflect"
	"testing"
)

func TestPieceTable_Basic(t *testing.T) {
	Pt := New([]byte("Hello"))

	if Pt.String() != "Hello" {
		t.Errorf("Expected 'Hello', got '%s'", Pt.String())
	}
	if Pt.Size() != 5 {
		t.Errorf("Expected size 5, got %d", Pt.Size())
	}
}

func TestPieceTable_Insert(t *testing.T) {
	Pt := New([]byte("Hello"))

	// Insert at end (Append)
	Pt.Insert(5, []byte(" World"))
	if Pt.String() != "Hello World" {
		t.Errorf("Insert end failed: %s", Pt.String())
	}

	// Addition optimization: adding character at end, pieces should merge
	Pt.Insert(11, []byte("!"))
	if Pt.String() != "Hello World!" {
		t.Errorf("Insert optimization failed: %s", Pt.String())
	}
	// We should have exactly 2 pieces: [Hello] and [ World!]
	if len(Pt.pieces) != 2 {
		t.Errorf("Optimization failed, expected 2 pieces, got %d", len(Pt.pieces))
	}

	// Insert at start
	Pt.Insert(0, []byte("Say "))
	if Pt.String() != "Say Hello World!" {
		t.Errorf("Insert start failed: %s", Pt.String())
	}

	// Insert in middle (splitting original buffer)
	Pt.Insert(6, []byte("o "))
	if Pt.String() != "Say Heo llo World!" {
		t.Errorf("Insert middle failed: %s", Pt.String())
	}
}

func TestPieceTable_Delete(t *testing.T) {
	Pt := New([]byte("Hello World!"))

	// Deleting from middle of one piece
	Pt.Delete(5, 6) // Remove " World"
	if Pt.String() != "Hello!" {
		t.Errorf("Delete middle failed: %s", Pt.String())
	}
	// After middle deletion, 1 piece should become 2
	if len(Pt.pieces) != 2 {
		t.Errorf("Expected 2 pieces after middle delete, got %d", len(Pt.pieces))
	}

	// Deleting on boundary (capturing end of left and start of right piece)
	Pt.Insert(5, []byte(" World")) // Restored: "Hello World!" -> pieces: ["Hello"], [" World"], ["!"]

	Pt.Delete(4, 3) // Remove "o W" -> Should leave "Hellorld!"
	if Pt.String() != "Hellorld!" {
		t.Errorf("Delete across boundary failed: %s", Pt.String())
	}

	// Deleting all text
	Pt.Delete(0, Pt.Size())
	if Pt.String() != "" {
		t.Errorf("Delete all failed: '%s'", Pt.String())
	}
	if Pt.Size() != 0 {
		t.Errorf("Expected size 0, got %d", Pt.Size())
	}
}

func TestPieceTable_Complex(t *testing.T) {
	Pt := New([]byte("The quick brown fox jumps over the lazy dog"))

	Pt.Delete(16, 4)                  // "The quick brown jumps over the lazy dog"
	Pt.Insert(16, []byte("cat "))     // "The quick brown cat jumps over the lazy dog"
	Pt.Delete(0, 4)                   // "quick brown cat jumps over the lazy dog"
	Pt.Insert(Pt.Size(), []byte(".")) // "quick brown cat jumps over the lazy dog."

	expected := "quick brown cat jumps over the lazy dog."
	if Pt.String() != expected {
		t.Errorf("Complex test failed:\nExpected: %s\nGot:      %s", expected, Pt.String())
	}
}

func TestPieceTable_GetRange(t *testing.T) {
	Pt := New([]byte("0123456789"))
	Pt.Insert(5, []byte("abc")) // "01234abc56789"

	// 1. Range from original buffer
	r1, _ := Pt.GetRange(1, 3)
	if string(r1) != "123" {
		t.Error("GetRange failed on original buffer")
	}

	// 2. Range from add buffer
	r2, _ := Pt.GetRange(6, 1)
	if string(r2) != "b" {
		t.Error("GetRange failed on add buffer")
	}

	// 3. Range spanning multiple pieces
	r3, _ := Pt.GetRange(4, 4)
	if string(r3) != "4abc" {
		t.Error("GetRange failed on spanning pieces")
	}

	// 4. Edge cases
	r4, _ := Pt.GetRange(0, Pt.Size())
	if string(r4) != "01234abc56789" {
		t.Error("GetRange failed on full range")
	}
	rErr1, _ := Pt.GetRange(-1, 5)
	rErr2, _ := Pt.GetRange(0, 100)
	if rErr1 != nil || rErr2 != nil {
		t.Error("GetRange should return nil for invalid ranges")
	}
}
func TestPieceTable_MergeOptimization(t *testing.T) {
	Pt := New([]byte("Start"))

	// 1. Insert at end (creates Add piece 1)
	Pt.Insert(Pt.Size(), []byte(" One"))
	if len(Pt.pieces) != 2 {
		t.Fatalf("Expected 2 pieces, got %d", len(Pt.pieces))
	}

	// 2. Insert at end again (should merge into Add piece 1)
	Pt.Insert(Pt.Size(), []byte(" Two"))
	if len(Pt.pieces) != 2 {
		t.Errorf("Optimization failed: expected pieces to merge, got %d pieces", len(Pt.pieces))
	}

	// 3. Insert in middle.
	// Note: Offset 5 is exactly between 'Start' and ' One Two'.
	// PieceTable inserts between pieces without splitting if offset is on boundary.
	Pt.Insert(5, []byte(" Middle"))
	if len(Pt.pieces) != 3 {
		t.Errorf("Expected 3 pieces after boundary insert, got %d", len(Pt.pieces))
	}

	expected := "Start Middle One Two"
	if Pt.String() != expected {
		t.Errorf("Data corrupted during merge test. Expected %q, got %q", expected, Pt.String())
	}
}

func TestPieceTable_AppendRange_Boundary(t *testing.T) {
	Pt := New([]byte("0123456789"))
	Pt.Insert(5, []byte("XXX")) // 01234 XXX 56789

	dest := make([]byte, 0, 10)

	// Read across all three pieces: "4" (Orig), "XXX" (Add), "5" (Orig)
	dest, err := Pt.AppendRange(dest, 4, 5)
	if err != nil {
		t.Fatalf("AppendRange failed: %v", err)
	}

	if string(dest) != "4XXX5" {
		t.Errorf("AppendRange across boundaries failed. Expected '4XXX5', got %q", string(dest))
	}

	// Ensure no data was overwritten improperly
	dest = append(dest, []byte("Tail")...)
	if string(dest) != "4XXX5Tail" {
		t.Errorf("AppendRange modified slice capacity/length improperly: %q", string(dest))
	}
}
func TestPieceTable_EmptyOperations(t *testing.T) {
	Pt := New([]byte("abc"))

	// 1. Zero length insert
	Pt.Insert(1, []byte(""))
	if Pt.Size() != 3 || Pt.String() != "abc" {
		t.Error("Zero length insert modified data")
	}

	// 2. Zero length delete
	Pt.Delete(1, 0)
	if Pt.Size() != 3 || Pt.String() != "abc" {
		t.Error("Zero length delete modified data")
	}

	// 3. Out of bounds delete
	Pt.Delete(1, 10)
	if Pt.Size() != 3 {
		t.Error("Out of bounds delete should be ignored")
	}
}

func TestPieceTable_BoundaryInsert(t *testing.T) {
	// Original: [AA][BB]
	Pt := New([]byte("AABB"))
	// Insert at 2 (between AA and BB)
	Pt.Insert(2, []byte("XX"))
	// Now pieces: [AA][XX][BB]

	// Delete [XX] exactly
	Pt.Delete(2, 2)

	if Pt.String() != "AABB" {
		t.Errorf("Boundary delete failed, got %q", Pt.String())
	}
	if len(Pt.pieces) != 2 {
		t.Errorf("Expected pieces to collapse/stay clean, got %d pieces", len(Pt.pieces))
	}
}
func TestPieceTable_FragmentationStress(t *testing.T) {
	// Create a document and perform many tiny operations to force piece splitting.
	Pt := New([]byte("INITIAL"))

	// Interleaved inserts
	for i := 0; i < 100; i++ {
		Pt.Insert(Pt.Size()/2, []byte("x"))
	}

	// Interleaved deletes
	for i := 0; i < 50; i++ {
		Pt.Delete(i, 1)
	}

	expectedLen := 7 + 100 - 50
	if Pt.Size() != expectedLen {
		t.Errorf("Stress size mismatch: expected %d, got %d", expectedLen, Pt.Size())
	}

	// Verify we can still read the whole document without errors
	_, err := Pt.Bytes()
	if err != nil {
		t.Errorf("Fragmentation caused corruption: %v", err)
	}
}
func TestPieceTable_StreamingIntegrity(t *testing.T) {
	// Tests ForEachRange which is used for saving files to disk.
	// We need to ensure it yields exactly the same bytes as a full memory dump.
	content := "The quick brown fox jumps over the lazy dog"
	Pt := New([]byte(content))

	// Fragment the table with multiple operations
	Pt.Delete(4, 6)               // "The brown fox..."
	Pt.Insert(4, []byte("lazy ")) // "The lazy brown fox..."
	Pt.Insert(Pt.Size(), []byte("!"))

	memBytes, _ := Pt.Bytes()
	var streamBytes []byte

	err := Pt.ForEachRange(func(data []byte) error {
		streamBytes = append(streamBytes, data...)
		return nil
	})

	if err != nil {
		t.Fatalf("ForEachRange failed: %v", err)
	}

	if !reflect.DeepEqual(memBytes, streamBytes) {
		t.Errorf("Streaming integrity failed.\nMem:    %q\nStream: %q", string(memBytes), string(streamBytes))
	}

	if string(streamBytes) != "The lazy brown fox jumps over the lazy dog!" {
		t.Errorf("Resulting text is wrong: %q", string(streamBytes))
	}
}
func TestPieceTable_ExtremeFragmentation(t *testing.T) {
	// Forces hundreds of pieces and tests reading across many of them.
	Pt := New([]byte("A"))
	expected := "A"

	// 1. Create many pieces via interleaved insertions.
	// We insert at position 0 to avoid the 'append-at-end' merge optimization.
	for i := 0; i < 500; i++ {
		Pt.Insert(0, []byte("X"))
		expected = "X" + expected
	}

	if len(Pt.pieces) < 500 {
		t.Errorf("Expected fragmentation, got %d pieces", len(Pt.pieces))
	}

	// 2. Test GetRange spanning many pieces
	// Read everything except the first and last chars
	res, _ := Pt.GetRange(1, 499)
	if len(res) != 499 {
		t.Errorf("GetRange length mismatch: expected 499, got %d", len(res))
	}

	// 3. Test multi-piece deletion
	// Remove middle 400 'X' chars
	Pt.Delete(50, 400)
	if Pt.Size() != 101 {
		t.Errorf("Size after multi-piece delete mismatch: expected 101, got %d", Pt.Size())
	}

	// Verify content remains valid
	if Pt.String() != expected[:50]+expected[450:] {
		t.Error("Content corrupted after multi-piece delete")
	}
}
func TestPieceTable_ReadAtBoundary(t *testing.T) {
	// Specifically targets the logic that stitches data from multiple pieces.
	// Piece 1: [0..9] (Original), Piece 2: [10..14] (Add), Piece 3: [15..24] (Original)
	Pt := New([]byte("012345678956789")) // "0123456789" then "56789"
	Pt.Insert(10, []byte("ABCDE"))       // Result: "0123456789ABCDE56789"

	// Pieces are:
	// 0: Original, Start 0, Len 10 ("0123456789")
	// 1: Add,      Start 0, Len 5  ("ABCDE")
	// 2: Original, Start 10, Len 5 ("56789")

	tests := []struct {
		off, len int
		expected string
	}{
		{9, 2, "9A"},                    // Spans Piece 0 and 1
		{14, 2, "E5"},                   // Spans Piece 1 and 2
		{10, 5, "ABCDE"},                // Exactly Piece 1
		{0, 20, "0123456789ABCDE56789"}, // All pieces
		{8, 9, "89ABCDE56"},             // Spans all three pieces
	}

	for _, tt := range tests {
		data, err := Pt.GetRange(tt.off, tt.len)
		if err != nil {
			t.Errorf("GetRange(%d, %d) error: %v", tt.off, tt.len, err)
			continue
		}
		if string(data) != tt.expected {
			t.Errorf("GetRange(%d, %d): expected %q, got %q", tt.off, tt.len, tt.expected, string(data))
		}

		// Also test AppendRange (zero-allocation variant)
		buf := make([]byte, 0, tt.len)
		buf, _ = Pt.AppendRange(buf, tt.off, tt.len)
		if string(buf) != tt.expected {
			t.Errorf("AppendRange(%d, %d) failed", tt.off, tt.len)
		}
	}
}

func TestPieceTable_MixedBuffer_ComplexRead(t *testing.T) {
	// Create a piece table with highly interleaved pieces from both buffers
	// [Orig:0-2][Add:0-2][Orig:2-4][Add:2-4]
	// "AB" + "12" + "CD" + "34" = "AB12CD34"
	Pt := New([]byte("ABCD"))
	Pt.Insert(2, []byte("12")) // "AB12CD"
	Pt.Insert(6, []byte("34")) // "AB12CD34"

	tests := []struct {
		off, len int
		want     string
	}{
		{0, 8, "AB12CD34"}, // Full
		{1, 4, "B12C"},     // Crosses 3 pieces
		{3, 2, "2C"},       // Crosses Add and Orig
		{7, 1, "4"},        // Tail
		{0, 0, ""},         // Zero length
	}

	for _, tt := range tests {
		got, err := Pt.GetRange(tt.off, tt.len)
		if err != nil {
			t.Errorf("GetRange(%d,%d) err: %v", tt.off, tt.len, err)
		}
		if string(got) != tt.want {
			t.Errorf("GetRange(%d,%d) = %q, want %q", tt.off, tt.len, string(got), tt.want)
		}
	}
}

func TestPieceTable_Delete_PieceRemoval(t *testing.T) {
	// Verifies that deleting a range that exactly matches one or more pieces
	// correctly removes them from the table.
	Pt := New([]byte("AAA"))
	Pt.Insert(0, []byte("CCC")) // [CCC][AAA]
	Pt.Insert(3, []byte("BBB")) // [CCC][BBB][AAA]

	// We inserted at 0 and then in the middle, preventing the "append-at-end" merge optimization.
	if len(Pt.pieces) != 3 {
		t.Fatalf("Expected 3 pieces, got %d", len(Pt.pieces))
	}

	// Delete "BBB" (offset 3, length 3)
	Pt.Delete(3, 3)

	if Pt.String() != "CCCAAA" {
		t.Errorf("Delete failed: %q", Pt.String())
	}
	if len(Pt.pieces) != 2 {
		t.Errorf("Piece was not removed from table, count: %d", len(Pt.pieces))
	}
}

// TestPieceTable_OriginalRange covers the promise the indexer leans on: a
// position in the original buffer is only handed out for text that is still
// the original's, so a caller reading the file at that position reads the
// bytes of that range and not something that was typed over them.
func TestPieceTable_OriginalRange(t *testing.T) {
	Pt := New([]byte("one\ntwo\nthree\n"))

	off, ok := Pt.OriginalRange(4, 4)
	if !ok || off != 4 {
		t.Fatalf("OriginalRange(4,4) on an unedited table = %d,%v, want 4,true", off, ok)
	}
	if _, ok := Pt.OriginalRange(0, Pt.Size()+1); ok {
		t.Error("a range past the end was accepted")
	}
	if _, ok := Pt.OriginalRange(0, 0); ok {
		t.Error("an empty range was accepted")
	}

	// "one\nXX" + "two\nthree\n": the inserted text is not in the original at
	// all, and the tail has moved away from the offset it reads at.
	Pt.Insert(4, []byte("XX"))
	if _, ok := Pt.OriginalRange(4, 2); ok {
		t.Error("inserted text was reported as living in the original")
	}
	if _, ok := Pt.OriginalRange(3, 4); ok {
		t.Error("a range crossing into the inserted text was accepted")
	}
	off, ok = Pt.OriginalRange(6, 4)
	if !ok || off != 4 {
		t.Fatalf("OriginalRange(6,4) after the insert = %d,%v, want 4,true", off, ok)
	}

	// What the caller would read there is what the range says it is.
	view, viewOK := Pt.View(6, 4)
	if !viewOK || string(view) != "two\n" {
		t.Fatalf("View(6,4) = %q,%v", string(view), viewOK)
	}
	if got := string([]byte("one\ntwo\nthree\n")[off : off+4]); got != string(view) {
		t.Errorf("the original at %d holds %q, but the range there is %q", off, got, string(view))
	}
}
