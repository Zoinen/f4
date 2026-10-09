package editor

import "testing"

// TestSplitSortableLines_Empty covers the nil-input fast path.
func TestSplitSortableLines_Empty(t *testing.T) {
	if got := splitSortableLines(nil); got != nil {
		t.Errorf("splitSortableLines(nil) = %#v, want nil", got)
	}
	if got := splitSortableLines([]byte{}); got != nil {
		t.Errorf("splitSortableLines([]byte{}) = %#v, want nil", got)
	}
}

// TestSplitSortableLines_LFTerminated is the simple case: every line ends
// with LF, and the terminator is kept in raw but stripped from key.
func TestSplitSortableLines_LFTerminated(t *testing.T) {
	got := splitSortableLines([]byte("a\nb\nc\n"))
	want := []sortableLine{
		{raw: []byte("a\n"), key: "a", terminated: true},
		{raw: []byte("b\n"), key: "b", terminated: true},
		{raw: []byte("c\n"), key: "c", terminated: true},
	}
	assertSortableLinesEqual(t, got, want)
}

// TestSplitSortableLines_UnterminatedLastLine is the common "no newline at
// EOF" file: the last logical line carries no terminator.
func TestSplitSortableLines_UnterminatedLastLine(t *testing.T) {
	got := splitSortableLines([]byte("a\nb"))
	want := []sortableLine{
		{raw: []byte("a\n"), key: "a", terminated: true},
		{raw: []byte("b"), key: "b", terminated: false},
	}
	assertSortableLinesEqual(t, got, want)
}

// TestSplitSortableLines_CRLF checks that a CRLF terminator is recognised and
// stripped as a whole from the key, not just the trailing LF.
func TestSplitSortableLines_CRLF(t *testing.T) {
	got := splitSortableLines([]byte("a\r\nb\r\n"))
	want := []sortableLine{
		{raw: []byte("a\r\n"), key: "a", terminated: true},
		{raw: []byte("b\r\n"), key: "b", terminated: true},
	}
	assertSortableLinesEqual(t, got, want)
}

// TestSplitSortableLines_LoneCRIsNotASeparator documents the function's
// actual boundary behaviour: it only splits on LF (bare or as part of CRLF).
// A file that uses only classic-Mac CR line breaks, with no LF anywhere,
// comes back as a single logical "line" holding the whole buffer — and a CR
// that is not immediately followed by an LF and not immediately preceding
// one either stays in the key as ordinary content, as in the "c\rd" line
// below.
func TestSplitSortableLines_LoneCRIsNotASeparator(t *testing.T) {
	got := splitSortableLines([]byte("a\rb\rc"))
	want := []sortableLine{
		{raw: []byte("a\rb\rc"), key: "a\rb\rc", terminated: false},
	}
	assertSortableLinesEqual(t, got, want)

	got = splitSortableLines([]byte("a\r\nb\nc\rd\n"))
	want = []sortableLine{
		{raw: []byte("a\r\n"), key: "a", terminated: true},
		{raw: []byte("b\n"), key: "b", terminated: true},
		{raw: []byte("c\rd\n"), key: "c\rd", terminated: true},
	}
	assertSortableLinesEqual(t, got, want)
}

func assertSortableLinesEqual(t *testing.T, got, want []sortableLine) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %#v", len(got), len(want), got)
	}
	for i := range got {
		if string(got[i].raw) != string(want[i].raw) || got[i].key != want[i].key || got[i].terminated != want[i].terminated {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestSortedLinesData_ShortInputReturnsCopy covers the "fewer than two
// lines" fast path: nothing to reorder, but the caller still gets its own
// copy of the bytes.
func TestSortedLinesData_ShortInputReturnsCopy(t *testing.T) {
	if got := sortedLinesData(nil, true, true); len(got) != 0 {
		t.Errorf("sortedLinesData(nil, ...) = %q, want empty", got)
	}
	in := []byte("solo")
	got := sortedLinesData(in, true, true)
	if string(got) != "solo" {
		t.Errorf("sortedLinesData(%q, ...) = %q, want unchanged", in, got)
	}
	if len(got) > 0 && &got[0] == &in[0] {
		t.Error("sortedLinesData returned the caller's own backing array")
	}
}

// TestSortedLinesData_AscendingAndDescending is the basic case-sensitive
// sort in both directions.
func TestSortedLinesData_AscendingAndDescending(t *testing.T) {
	data := []byte("banana\napple\ncherry\n")
	if got, want := sortedLinesData(data, true, true), "apple\nbanana\ncherry\n"; string(got) != want {
		t.Errorf("ascending = %q, want %q", got, want)
	}
	if got, want := sortedLinesData(data, false, true), "cherry\nbanana\napple\n"; string(got) != want {
		t.Errorf("descending = %q, want %q", got, want)
	}
}

// TestSortedLinesData_CaseInsensitiveOrdersByLowercaseButKeepsOriginalCase
// checks that caseSensitive=false only affects the comparison, not the
// bytes written back out.
func TestSortedLinesData_CaseInsensitiveOrdersByLowercaseButKeepsOriginalCase(t *testing.T) {
	data := []byte("Cherry\napple\nBanana\n")
	got := sortedLinesData(data, true, false)
	want := "apple\nBanana\nCherry\n"
	if string(got) != want {
		t.Errorf("case-insensitive ascending = %q, want %q", got, want)
	}
}

// TestSortedLinesData_CaseSensitiveOrdersUppercaseFirst is the counterpart:
// with caseSensitive=true, ASCII uppercase sorts before lowercase.
func TestSortedLinesData_CaseSensitiveOrdersUppercaseFirst(t *testing.T) {
	data := []byte("banana\nApple\ncherry\n")
	got := sortedLinesData(data, true, true)
	want := "Apple\nbanana\ncherry\n"
	if string(got) != want {
		t.Errorf("case-sensitive ascending = %q, want %q", got, want)
	}
}

// TestSortedLinesData_StableForEqualKeys checks that lines whose comparison
// keys tie (here, after lowercasing) keep their original relative order.
func TestSortedLinesData_StableForEqualKeys(t *testing.T) {
	data := []byte("B\nb\na\n")
	got := sortedLinesData(data, true, false)
	want := "a\nB\nb\n"
	if string(got) != want {
		t.Errorf("stable sort = %q, want %q", got, want)
	}
}

// TestSortedLinesData_UnterminatedLastLineTerminatorMoves is the file that
// does not end in a newline: when sorting moves that line away from the
// end, its missing terminator has to move with the new last line instead,
// so the buffer as a whole still ends without a trailing newline.
func TestSortedLinesData_UnterminatedLastLineTerminatorMoves(t *testing.T) {
	data := []byte("banana\ncherry\napple") // "apple" has no trailing \n
	got := sortedLinesData(data, true, true)
	want := "apple\nbanana\ncherry" // "cherry" lost its \n instead
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Descending, so the originally-unterminated line ends up first, not
	// last, and still gets a synthesized terminator.
	got = sortedLinesData(data, false, true)
	want = "cherry\nbanana\napple"
	if string(got) != want {
		t.Errorf("descending: got %q, want %q", got, want)
	}
}

// TestSortedLinesData_UnterminatedLastLineStaysLast is the no-transfer-
// needed case: the file already sorts with its unterminated line last, so
// nothing about the terminators should change.
func TestSortedLinesData_UnterminatedLastLineStaysLast(t *testing.T) {
	data := []byte("apple\nbanana\ncherry") // already sorted, "cherry" bare
	got := sortedLinesData(data, true, true)
	if string(got) != string(data) {
		t.Errorf("got %q, want unchanged %q", got, data)
	}
}

// TestSortLines_EmptyBuffer covers the "nothing to sort" guard.
func TestSortLines_EmptyBuffer(t *testing.T) {
	ev := newDuplicateLineEditor(t, "")
	if err := ev.SortLines(true, true); err != nil {
		t.Fatalf("SortLines on an empty buffer returned %v", err)
	}
	if ev.Modified {
		t.Error("empty buffer got marked modified")
	}
}

// TestSortLines_WholeBufferAscending is the common case: no selection,
// sort every line, and check that the cursor stays on its logical line and
// column rather than following its old byte offset.
func TestSortLines_WholeBufferAscending(t *testing.T) {
	ev := newDuplicateLineEditor(t, "banana\napple\ncherry\n")
	ev.CursorLine = 1
	ev.CursorPos = 3 // inside "apple", 4th column

	if err := ev.SortLines(true, true); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if got, want := ev.Pt.String(), "apple\nbanana\ncherry\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.Modified {
		t.Error("buffer not marked modified")
	}
	// Line 1 used to be "apple"; after the sort it is "banana", and the
	// cursor should have followed the line number, landing at column 3 of
	// the new occupant rather than staying glued to "apple".
	if ev.CursorLine != 1 || ev.CursorPos != 3 {
		t.Errorf("cursor = line %d pos %d, want line 1 pos 3", ev.CursorLine, ev.CursorPos)
	}

	ev.Undo()
	if got, want := ev.Pt.String(), "banana\napple\ncherry\n"; got != want {
		t.Errorf("buffer after undo = %q, want %q", got, want)
	}
}

// TestSortLines_WholeBufferDescending is the same whole-buffer path with
// the direction flipped.
func TestSortLines_WholeBufferDescending(t *testing.T) {
	ev := newDuplicateLineEditor(t, "apple\nbanana\ncherry\n")
	if err := ev.SortLines(false, true); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if got, want := ev.Pt.String(), "cherry\nbanana\napple\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

// TestSortLines_CaseInsensitive exercises the caseSensitive=false path
// through the whole method, not just the helper.
func TestSortLines_CaseInsensitive(t *testing.T) {
	ev := newDuplicateLineEditor(t, "Cherry\napple\nBanana\n")
	if err := ev.SortLines(true, false); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if got, want := ev.Pt.String(), "apple\nBanana\nCherry\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
}

// TestSortLines_NoOpWhenAlreadySorted checks the bytes.Equal short circuit:
// a buffer that is already in the requested order is left untouched, not
// rewritten byte-for-byte, and the editor is not marked modified.
func TestSortLines_NoOpWhenAlreadySorted(t *testing.T) {
	ev := newDuplicateLineEditor(t, "apple\nbanana\ncherry\n")
	if err := ev.SortLines(true, true); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if ev.Modified {
		t.Error("already-sorted buffer got marked modified")
	}
	if got, want := ev.Pt.String(), "apple\nbanana\ncherry\n"; got != want {
		t.Errorf("buffer = %q, want unchanged %q", got, want)
	}
}

// TestSortLines_StreamSelectionSortsOnlySelectedLines checks the selection
// path: only the lines the (stream) selection touches are reordered, the
// rest of the buffer is untouched, and the selection follows its block.
func TestSortLines_StreamSelectionSortsOnlySelectedLines(t *testing.T) {
	ev := newDuplicateLineEditor(t, "zeta\nbanana\napple\nomega\n")
	// Select "banana" and "apple" (lines 1 and 2): anchor in the middle of
	// "banana", cursor in the middle of "apple", same pattern the move-line
	// tests use for a two-line block selection.
	ev.SelActive = true
	ev.SelAnchorOffset = 6 // line 1 ("banana"), column 1
	ev.CursorLine = 2      // "apple"
	ev.CursorPos = 2

	if err := ev.SortLines(true, true); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if got, want := ev.Pt.String(), "zeta\napple\nbanana\nomega\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.Modified {
		t.Error("buffer not marked modified")
	}
	if !ev.SelActive {
		t.Error("selection was dropped")
	}
	if ev.SelAnchorOffset != 6 {
		t.Errorf("selection anchor = %d, want 6 (the block's byte range is unchanged in size)", ev.SelAnchorOffset)
	}
	// The cursor followed its logical line/column: line 2 used to be
	// "apple", now it is "banana", and column 2 is still inside it.
	if ev.CursorLine != 2 || ev.CursorPos != 2 {
		t.Errorf("cursor = line %d pos %d, want line 2 pos 2", ev.CursorLine, ev.CursorPos)
	}
}

// TestSortLines_RectangularSelectionSortsOnlySelectedLines is the same
// selected-block behaviour through a rectangular (column-block) selection
// instead of a stream one.
func TestSortLines_RectangularSelectionSortsOnlySelectedLines(t *testing.T) {
	ev := newDuplicateLineEditor(t, "zeta\nbanana\napple\nomega\n")
	ev.RectSelActive = true
	ev.rectSelStartLine = 1
	ev.rectSelStartCol = 0
	ev.CursorLine = 2
	ev.CursorPos = 0

	if err := ev.SortLines(true, true); err != nil {
		t.Fatalf("SortLines returned %v", err)
	}
	if got, want := ev.Pt.String(), "zeta\napple\nbanana\nomega\n"; got != want {
		t.Fatalf("buffer = %q, want %q", got, want)
	}
	if !ev.RectSelActive {
		t.Error("rectangular selection was dropped")
	}
	if ev.rectSelStartLine != 1 || ev.rectSelStartCol != 0 {
		t.Errorf("rect selection start = line %d col %d, want line 1 col 0", ev.rectSelStartLine, ev.rectSelStartCol)
	}
}
