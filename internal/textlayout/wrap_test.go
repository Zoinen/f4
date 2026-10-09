package textlayout

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

func TestWrapEngine_SimpleWrap(t *testing.T) {
	Pt := piecetable.New([]byte("The quick brown fox jumps over the lazy dog"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.SetWidth(10)

	frags := we.GetFragments(0)

	// Пояснение: "The quick " (10), "brown fox " (10), "jumps over " (11, пробел в конце), "the lazy " (9), "dog" (3)
	expectedTexts := []string{"The quick ", "brown fox ", "jumps over ", "the lazy ", "dog"}
	if len(frags) != len(expectedTexts) {
		t.Fatalf("Expected %d fragments, got %d. Frags: %+v", len(expectedTexts), len(frags), frags)
	}

	for i, frag := range frags {
		data, _ := Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if text != expectedTexts[i] {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_BidiCaretUsesVisualClusterOrder(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	defer func() { vtui.DefaultBidiMode = oldMode }()

	text := "שלום"
	Pt := piecetable.New([]byte(text))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.ToggleWrap(false)

	if _, col := we.LogicalToVisual(0); col != 4 {
		t.Fatalf("logical start visual column = %d, want 4", col)
	}
	if _, col := we.LogicalToVisual(len(text)); col != 0 {
		t.Fatalf("logical end visual column = %d, want 0", col)
	}
	if got := we.VisualToLogical(0, 1); got != len(text)-2 {
		t.Fatalf("visual column 1 logical offset = %d, want %d", got, len(text)-2)
	}
}

func TestWrapEngine_NoWrap(t *testing.T) {
	Pt := piecetable.New([]byte("This is a very long line that should not be wrapped."))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.SetWidth(20)
	we.ToggleWrap(false)

	frags := we.GetFragments(0)
	if len(frags) != 1 {
		t.Fatalf("Expected 1 fragment when word wrap is off, got %d", len(frags))
	}

	data, _ := Pt.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart)
	text := string(data)
	if text != "This is a very long line that should not be wrapped." {
		t.Errorf("Fragment text mismatch: got %q", text)
	}
}

func TestWrapEngine_UnicodeWrap(t *testing.T) {
	text := "A世B世C D"
	Pt := piecetable.New([]byte(text))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.SetWidth(4)

	frags := we.GetFragments(0)

	// 1. "A世B" (4)
	// 2. "世C " (4) - пробел в конце
	// 3. "D"    (1)
	expectedTexts := []string{"A世B", "世C ", "D"}
	if len(frags) != 3 {
		t.Fatalf("Expected 3 fragments for unicode string, got %d. Fragments: %+v", len(frags), frags)
	}

	for i, frag := range frags {
		data, _ := Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if text != expectedTexts[i] {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_LongWord(t *testing.T) {
	Pt := piecetable.New([]byte("supercalifragilisticexpialidocious"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(10)

	frags := we.GetFragments(0)
	expectedTexts := []string{"supercalif", "ragilistic", "expialidoc", "ious"}

	if len(frags) != 4 {
		t.Fatalf("Expected 4 fragments for long word, got %d", len(frags))
	}

	for i, frag := range frags {
		data, _ := Pt.GetRange(frag.ByteOffsetStart, frag.ByteOffsetEnd-frag.ByteOffsetStart)
		text := string(data)
		if !reflect.DeepEqual(text, expectedTexts[i]) {
			t.Errorf("Frag %d: expected %q, got %q", i, expectedTexts[i], text)
		}
	}
}

func TestWrapEngine_Navigation(t *testing.T) {
	// Строка: "01234 67890", ширина 5.
	// Фрагменты: "01234 ", "67890"
	Pt := piecetable.New([]byte("01234 67890"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(5)

	// 1. Тест LogicalToVisual
	// Оффсет 6 (символ '6').
	row, col := we.LogicalToVisual(6)
	if row != 1 || col != 0 {
		t.Errorf("LogicalToVisual(6): expected (1, 0), got (%d, %d)", row, col)
	}

	// 2. Тест VisualToLogical
	// Вторая строка ("67890"), колонка 1 (символ '7')
	offset := we.VisualToLogical(1, 1)
	if offset != 7 {
		t.Errorf("VisualToLogical(1, 1): expected offset 7, got %d", offset)
	}
}

func TestWrapEngine_TabsVariableWidth(t *testing.T) {
	// TabSize = 4.
	// "1\t" -> '1' (col 0), '\t' starts at col 1. Width should be 4 - (1%4) = 3. Total width 4.
	// "\t"  -> starts at col 0. Width 4.
	Pt := piecetable.New([]byte("1\t\t"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetTabSize(4)
	we.ToggleWrap(false)

	frags := we.GetFragments(0)
	// '1' (1) + tab1 (3) + tab2 (4) = 8
	if frags[0].VisualWidth != 8 {
		t.Errorf("Variable tab width failed: expected 8, got %d", frags[0].VisualWidth)
	}

	// Test mapping inside the first tab (visual columns 1, 2, 3)
	// All should map to the same logical offset 1 (the first tab)
	for col := 1; col <= 3; col++ {
		off := we.VisualToLogical(0, col)
		if off != 1 {
			t.Errorf("VisualToLogical(col %d) inside tab: expected offset 1, got %d", col, off)
		}
	}
}

func TestWrapEngine_SetTabSize(t *testing.T) {
	Pt := piecetable.New([]byte("\t"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.ToggleWrap(false)

	we.SetTabSize(4)
	if we.GetFragments(0)[0].VisualWidth != 4 {
		t.Errorf("TabSize 4 failed: got %d", we.GetFragments(0)[0].VisualWidth)
	}

	we.SetTabSize(8)
	// Changing tab size must invalidate cache
	if we.GetFragments(0)[0].VisualWidth != 8 {
		t.Errorf("TabSize 8 failed: got %d", we.GetFragments(0)[0].VisualWidth)
	}
}

func TestWrapEngine_SetPointersSync(t *testing.T) {
	pt1 := piecetable.New([]byte("old"))
	li1 := piecetable.NewLineIndex()
	li1.Rebuild(pt1)
	we := NewWrapEngine(pt1, li1)
	we.GetFragments(0) // Warm up cache

	pt2 := piecetable.New([]byte("new data"))
	li2 := piecetable.NewLineIndex()
	li2.Rebuild(pt2)

	// Update pointers
	we.SetPointers(pt2, li2)

	frags := we.GetFragments(0)
	data, _ := pt2.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart)
	if string(data) != "new data" {
		t.Errorf("SetPointers failed to sync data: expected 'new data', got %q", string(data))
	}
}

func TestWrapEngine_WrappedTabAlignment(t *testing.T) {
	// Logical: "123\t" (Tab should occupy 1 cell to reach col 4)
	// Width: 2.
	// Frag 1: "12" (Width 2)
	// Frag 2: "3\t" -> '3' is at visual col 2. Tab starts at col 3.
	// Tab width should be 4 - (3%4) = 1.
	Pt := piecetable.New([]byte("123\t"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetTabSize(4)
	we.SetWidth(2)

	frags := we.GetFragments(0)
	if len(frags) < 2 {
		t.Fatal("Should wrap")
	}

	lastFrag := frags[len(frags)-1]
	// '3' (1) + tab (1) = 2
	if lastFrag.VisualWidth != 2 {
		t.Errorf("Tab on wrapped line misaligned. Width: %d, expected 2", lastFrag.VisualWidth)
	}
}

func TestWrapEngine_Performance10MB(t *testing.T) {
	// Создаем 10 МБ текста
	chunk := "The quick brown fox jumps over the lazy dog. " // 45 bytes
	count := (10 * 1024 * 1024) / len(chunk)
	data := make([]byte, 0, count*len(chunk))
	for i := 0; i < count; i++ {
		data = append(data, chunk...)
	}

	Pt := piecetable.New(data)
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	// Тест 1: С пробелами (Word Wrap)
	start := time.Now()
	frags := we.GetFragments(0)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Performance (with spaces) too slow: %v", elapsed)
	}
	t.Logf("10MB with spaces parsed into %d fragments in %v", len(frags), elapsed)

	// Тест 2: Без пробелов (Hard Wrap)
	we.InvalidateCache()
	we.Pt = piecetable.New(bytes.Repeat([]byte("A"), 10*1024*1024))
	we.Li.Rebuild(we.Pt)

	start = time.Now()
	frags = we.GetFragments(0)
	elapsed = time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Performance (hard wrap) too slow: %v", elapsed)
	}
	t.Logf("10MB without spaces parsed into %d fragments in %v", len(frags), elapsed)
}
func TestWrapEngine_ExtremeCorners(t *testing.T) {
	// 1. Окно шириной 1, символ шириной 2
	Pt := piecetable.New([]byte("世"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(1) // Меньше ширины символа

	frags := we.GetFragments(0)
	if len(frags) != 1 || frags[0].VisualWidth != 2 {
		t.Errorf("Narrow window CJK: expected width 2, got %v", frags[0].VisualWidth)
	}

	// 2. Сверхдлинное слово без пробелов
	pt2 := piecetable.New([]byte("1234567890"))
	li2 := piecetable.NewLineIndex()
	li2.Rebuild(pt2)
	we.Pt = pt2
	we.Li = li2
	we.SetWidth(3)

	frags2 := we.GetFragments(0)
	// Ожидаем: "123", "456", "789", "0"
	if len(frags2) != 4 {
		t.Errorf("Long word break: expected 4 frags, got %d", len(frags2))
	}

	// 3. Сохранение отступов (ведущих пробелов)
	pt3 := piecetable.New([]byte("    Line with indentation"))
	li3 := piecetable.NewLineIndex()
	li3.Rebuild(pt3)
	we.Pt = pt3
	we.Li = li3
	we.SetWidth(10)

	frags3 := we.GetFragments(0)
	// Ожидаем "    Line " (пробел после Line влезает в 10 символов)
	data3, _ := pt3.GetRange(frags3[0].ByteOffsetStart, frags3[0].ByteOffsetEnd-frags3[0].ByteOffsetStart)
	text := string(data3)
	if text != "    Line " {
		t.Errorf("Indentation preserved: expected '    Line ', got %q", text)
	}
}

func TestWrapEngine_MultipleSpaces(t *testing.T) {
	Pt := piecetable.New([]byte("word1    word2"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(10)

	frags := we.GetFragments(0)
	// 1. "word1     " (9 символов: "word1" + 4 пробела) - это влезает в 10.
	// 2. "word2"
	if len(frags) != 2 {
		t.Fatalf("Expected 2 fragments for multiple spaces, got %d", len(frags))
	}
	d1, _ := Pt.GetRange(frags[0].ByteOffsetStart, frags[0].ByteOffsetEnd-frags[0].ByteOffsetStart)
	d2, _ := Pt.GetRange(frags[1].ByteOffsetStart, frags[1].ByteOffsetEnd-frags[1].ByteOffsetStart)
	text1 := string(d1)
	text2 := string(d2)
	if text1 != "word1    " || text2 != "word2" {
		t.Errorf("Multiple spaces failed. Got %q and %q", text1, text2)
	}
}
func TestWrapEngine_EndOfLineCursor(t *testing.T) {
	Pt := piecetable.New([]byte("abc"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(10)

	// The cursor is often placed at offset == length(text) to type at the end.
	// We want to ensure LogicalToVisual correctly maps this to the end of the first row.
	row, col := we.LogicalToVisual(3)

	if row != 0 || col != 3 {
		t.Errorf("Cursor at EOF on first line: expected (0, 3), got (%d, %d)", row, col)
	}
}

type loadingBuffer struct{}

func (l *loadingBuffer) Size() int                               { return 100 }
func (l *loadingBuffer) Read(offset, length int) ([]byte, error) { return nil, piecetable.ErrLoading }

func TestWrapEngine_ErrLoading(t *testing.T) {
	Pt := piecetable.NewWithBuffer(&loadingBuffer{})
	Li := piecetable.NewLineIndex()
	// Rebuild will finish instantly with 1 line because of ErrLoading
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.SetWidth(20)

	frags := we.GetFragments(0)
	if len(frags) != 1 {
		t.Fatalf("Expected 1 loading fragment, got %d", len(frags))
	}

	if frags[0].VisualWidth != 16 { // Our constant for "[ Loading... ]" width
		t.Errorf("Expected loading fragment width 16, got %d", frags[0].VisualWidth)
	}
}
func TestWrapEngine_InvalidateFrom(t *testing.T) {
	Pt := piecetable.New([]byte("Line 0\nLine 1\nLine 2\nLine 3\nLine 4"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.SetWidth(20)

	// 1. Force full cache calculation
	total := we.GetTotalVisualRows()
	if total != 5 {
		t.Fatalf("Expected 5 visual rows, got %d", total)
	}
	if we.validUntil != 4 {
		t.Fatalf("Expected validUntil 4, got %d", we.validUntil)
	}

	// 2. Invalidate from middle (Line 2)
	we.InvalidateFrom(2)

	// validUntil should be 1 (lines 0 and 1 are still valid)
	if we.validUntil != 1 {
		t.Errorf("InvalidateFrom failed: expected validUntil 1, got %d", we.validUntil)
	}

	// Line 0 and 1 cache should still exist
	if we.fragmentCache[1] == nil {
		t.Error("Cache for Line 1 should not have been cleared")
	}

	// Line 2 and later cache should be nil
	if we.fragmentCache[2] != nil {
		t.Error("Cache for Line 2 should have been cleared")
	}

	// 3. Recalculate and verify it recovers correctly
	total = we.GetTotalVisualRows()
	if total != 5 {
		t.Errorf("Failed to recover total rows after invalidation, got %d", total)
	}
}
func TestWrapEngine_LazyCache_LargeJump(t *testing.T) {
	// Create 1000 lines, each wrapping into 2 visual rows
	line := "Word1 Word2 Word3 Word4 Word5\n"
	Pt := piecetable.New(bytes.Repeat([]byte(line), 1000))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	// Set width small enough to force 2 rows per line
	we.SetWidth(10)

	// 1. Request a visual row near the end without previous calculation
	// This triggers the lazy calculation loop in chunks of 100.
	targetRow := 1500
	logLine, frag := we.GetLogLineAtVisualRow(targetRow)

	if logLine < 0 || logLine >= 1000 {
		t.Errorf("Invalid logical line mapping: %d", logLine)
	}

	// Each logical line produces 5 fragments with width 10:
	// "Word1 ", "Word2 ", "Word3 ", "Word4 ", "Word5"
	// 1000 lines * 5 rows = 5000 rows.
	// The 1000th \n creates a 1001st empty line (1 row). Total = 5001.
	// Row 1500 should be logical line 300 (1500 / 5)
	expectedLine := 300
	if logLine != expectedLine {
		t.Errorf("Lazy cache jump failed: expected line %d, got %d (frag %d)", expectedLine, logLine, frag)
	}

	// 2. Request total rows
	total := we.GetTotalVisualRows()
	if total != 5001 {
		t.Errorf("Expected 5001 total rows, got %d", total)
	}
}
func TestWrapEngine_CJKBoundaryWrap(t *testing.T) {
	// Test that a CJK character (width 2) is moved to the next line
	// entirely if it doesn't fit at the end of the current one.
	// "ABC" (3) + "世" (2) = 5. Width = 4.
	Pt := piecetable.New([]byte("ABC世"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(4)

	frags := we.GetFragments(0)
	// Expected: "ABC" (row 0), "世" (row 1)
	if len(frags) != 2 {
		t.Fatalf("Expected 2 fragments, got %d", len(frags))
	}
	if frags[0].VisualWidth != 3 {
		t.Errorf("First frag width: expected 3, got %d", frags[0].VisualWidth)
	}
	if frags[1].VisualWidth != 2 {
		t.Errorf("Second frag width: expected 2, got %d", frags[1].VisualWidth)
	}
}

func TestWrapEngine_CacheResilience(t *testing.T) {
	// Tests if the engine handles a shortened LineIndex while having a high validUntil.
	Pt := piecetable.New([]byte("L1\nL2\nL3\nL4\nL5"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	// Fill cache
	we.GetTotalVisualRows()
	if we.validUntil != 4 {
		t.Fatalf("Setup fail: validUntil=%d", we.validUntil)
	}

	// Shorten the document and index
	Pt.Delete(0, 10) // Delete almost everything
	Li.Rebuild(Pt)   // Index now has fewer lines

	// This should not panic even though validUntil > li.LineCount()
	total := we.GetTotalVisualRows()
	if total > 5 {
		t.Errorf("Engine did not reset total rows after index change, got %d", total)
	}
}

func TestWrapEngine_IndicVisualClusters(t *testing.T) {
	text := "संस्कृतम्"
	Pt := piecetable.New([]byte(text))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)

	we := NewWrapEngine(Pt, Li)
	we.ToggleWrap(false)

	var offsets []int
	var columns []int
	offset, column := 0, 0
	for rest := text; len(rest) > 0; {
		cluster, width, size := NextVisualCluster(rest)
		if cluster == "" || size <= 0 {
			t.Fatalf("invalid cluster result for %q", rest)
		}
		offsets = append(offsets, offset)
		columns = append(columns, column)
		offset += size
		column += width
		rest = rest[size:]
	}
	offsets = append(offsets, offset)
	columns = append(columns, column)

	if got := len(offsets); got != 5 {
		t.Fatalf("expected four visual clusters plus EOF, got %d", got-1)
	}
	for i := range offsets {
		row, col := we.LogicalToVisual(offsets[i])
		if row != 0 || col != columns[i] {
			t.Errorf("logical offset %d: got visual (%d,%d), want (0,%d)", offsets[i], row, col, columns[i])
		}
		if got := we.VisualToLogical(0, columns[i]); got != offsets[i] {
			t.Errorf("visual column %d: got logical offset %d, want %d", columns[i], got, offsets[i])
		}
	}
}

func TestWrapEngine_BoundarySafety(t *testing.T) {
	Pt := piecetable.New([]byte("line1\nline2"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	t.Run("Negative offset mapping", func(t *testing.T) {
		// Should not panic, should return 0,0
		row, col := we.LogicalToVisual(-50)
		if row != 0 || col != 0 {
			t.Errorf("LogicalToVisual(-50) expected (0,0), got (%d,%d)", row, col)
		}
	})

	t.Run("Negative row mapping", func(t *testing.T) {
		// Should not panic, should return offset 0
		off := we.VisualToLogical(-10, 5)
		if off != 0 {
			t.Errorf("VisualToLogical(-10) expected offset 0, got %d", off)
		}
	})
}

func TestVisualClustersInVisualOrderPreservesTerminalClustersInBidiParagraph(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	defer func() { vtui.DefaultBidiMode = oldMode }()

	text := "संस्कृतम् ދިވެހިބަސް"
	logical := VisualClusters(text)
	visual := VisualClustersInVisualOrder(text)
	if len(visual) != len(logical) {
		t.Fatalf("visual cluster count = %d, want %d", len(visual), len(logical))
	}

	wantStarts := make(map[int]bool, len(logical))
	for _, cluster := range logical {
		wantStarts[cluster.Start] = true
	}
	seen := make(map[int]bool, len(visual))
	for _, cluster := range visual {
		if !wantStarts[cluster.Start] {
			t.Fatalf("visual cluster starts at unexpected byte %d", cluster.Start)
		}
		if seen[cluster.Start] {
			t.Fatalf("visual cluster start %d was emitted twice", cluster.Start)
		}
		seen[cluster.Start] = true
	}
	if len(seen) != len(wantStarts) {
		t.Fatalf("visual clusters covered %d logical starts, want %d", len(seen), len(wantStarts))
	}
}

func TestVisualClustersInVisualOrderKeepsLeadingThaanaBeforeLatinText(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })

	text := "ދިވެހިބަސް - Divehi (Maldivian) - BiDi"
	var got strings.Builder
	for _, cluster := range VisualClustersInVisualOrder(text) {
		got.WriteString(cluster.Text)
	}

	// A left to right line: the Thaana word is reversed in place, the rest
	// of the line stays where it is (unxed/f4#546, "f4 changed the word
	// order").
	want := "ސްބަހިވެދި - Divehi (Maldivian) - BiDi"
	if got.String() != want {
		t.Fatalf("visual text = %q, want %q", got.String(), want)
	}
}

func TestWrapEngine_BidiVisualMoveLeavesRTLRunInVisualDirection(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	t.Cleanup(func() { vtui.DefaultBidiMode = oldMode })

	text := "abc אבג def"
	logical := logicalTextClusters(text)
	caret := buildVisualCaretMap(text)
	if got, want := caret.LogicalToVisual[len(logical)-4], 4; got != want {
		t.Fatalf("caret at the end of RTL run = %d, want visual boundary %d", got, want)
	}

	tests := []struct {
		name      string
		pos       int
		direction int
		want      int
	}{
		{
			name:      "left leaves RTL run",
			pos:       len([]byte("abc אבג")),
			direction: -1,
			want:      len([]byte("abc")),
		},
		{
			name:      "right enters RTL run from its left edge",
			pos:       len([]byte("abc אבג")),
			direction: 1,
			want:      len([]byte("abc אב")),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := fragmentVisualMove(text, test.pos, test.direction)
			if !ok {
				t.Fatalf("visual move was rejected at byte offset %d", test.pos)
			}
			if got != test.want {
				t.Fatalf("visual move from %d in direction %d = %d, want %d", test.pos, test.direction, got, test.want)
			}
		})
	}
}

func TestWrapEngine_LogicalToVisual_CappedLine(t *testing.T) {
	// Tests safety when a logical line is massive (binary) and indexing is capped at 64KB.
	// Create 100KB of data with NO newlines.
	data := make([]byte, 100*1024)
	for i := range data {
		data[i] = 'a'
	}

	Pt := piecetable.New(data)
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	// LogicalToVisual for an offset far beyond the 64KB cap.
	// It should NOT crash and should return the end of the indexed fragment.
	row, col := we.LogicalToVisual(90 * 1024)

	if row < 0 || col < 0 {
		t.Errorf("LogicalToVisual returned negative coordinates for capped line: (%d, %d)", row, col)
	}
}

func BenchmarkWrapEngine_GetFragments_ASCII(b *testing.B) {
	line := []byte("The quick brown fox jumps over the lazy dog and runs across the wide fields 1234567890\n")
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		buf.Write(line)
	}
	Pt := piecetable.New(buf.Bytes())
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		we.InvalidateCache()
		_ = we.GetFragments(i % 1000)
	}
}

func BenchmarkWrapEngine_GetFragments_NoWrap(b *testing.B) {
	line := []byte("The quick brown fox jumps over the lazy dog and runs across the wide fields 1234567890\n")
	var buf bytes.Buffer
	for i := 0; i < 1000; i++ {
		buf.Write(line)
	}
	Pt := piecetable.New(buf.Bytes())
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)
	we.ToggleWrap(false)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = we.GetFragments(i % 1000)
	}
}

// TestWrapEngine_NoWrapCacheStaysBounded pins the memory ceiling of the
// unwrapped layout cache. Without a budget, every line the user scrolls past
// stays cached, so simply paging through a file grew the heap without limit.
func TestWrapEngine_NoWrapCacheStaysBounded(t *testing.T) {
	var buf bytes.Buffer
	for i := 0; i < 20000; i++ {
		buf.WriteString(strings.Repeat("a", 79))
		buf.WriteByte('\n')
	}
	Pt := piecetable.New(buf.Bytes())
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)
	we.ToggleWrap(false)

	for i := 0; i < Li.LineCount(); i++ {
		we.GetFragments(i)
	}

	if we.noWrapCached > noWrapCacheBudget {
		t.Errorf("cached %d cluster entries, budget is %d", we.noWrapCached, noWrapCacheBudget)
	}

	// The budget must not cost correctness: an evicted line is recomputed.
	frags := we.GetFragments(0)
	if len(frags) != 1 || frags[0].VisualWidth != 79 {
		t.Fatalf("line 0 after eviction: %+v", frags)
	}
	if _, col := we.LogicalToVisual(40); col != 40 {
		t.Errorf("column after eviction = %d, want 40", col)
	}
}

// TestWrapEngine_NoWrapCacheInvalidatedOnEdit guards the counter that backs the
// budget: a stale count would either leak or evict on every single lookup.
func TestWrapEngine_NoWrapCacheInvalidatedOnEdit(t *testing.T) {
	Pt := piecetable.New([]byte("hello world\nsecond line\n"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)
	we.ToggleWrap(false)

	we.GetFragments(0)
	we.GetFragments(1)
	cachedBefore := we.noWrapCached

	insert := []byte("XX")
	Pt.Insert(0, insert)
	Li.UpdateAfterInsert(0, insert)
	we.InvalidateFrom(0)

	if we.noWrapCached != 0 {
		t.Errorf("cluster count after full invalidation = %d, want 0", we.noWrapCached)
	}
	if cachedBefore == 0 {
		t.Error("expected the unwrapped lines to be cached in the first place")
	}
	if got := we.GetFragments(0)[0].VisualWidth; got != 13 {
		t.Errorf("width after edit = %d, want 13", got)
	}
}

func TestBuildVisualCaretMapIdentityWhenNotRTL(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiOff
	defer func() { vtui.DefaultBidiMode = oldMode }()

	// With bidi reordering off, buildVisualCaretMap must take its early
	// identity-mapping branch and never consult the bidi layout, even for
	// text that is itself RTL.
	want := []int{0, 1, 2, 3}

	caret := buildVisualCaretMap("abc")
	if !reflect.DeepEqual(caret.LogicalToVisual, want) {
		t.Errorf("LogicalToVisual(%q) = %v, want %v", "abc", caret.LogicalToVisual, want)
	}
	if !reflect.DeepEqual(caret.VisualToLogical, want) {
		t.Errorf("VisualToLogical(%q) = %v, want %v", "abc", caret.VisualToLogical, want)
	}

	caretRTL := buildVisualCaretMap("אבג")
	if !reflect.DeepEqual(caretRTL.LogicalToVisual, want) {
		t.Errorf("LogicalToVisual(RTL, BidiOff) = %v, want %v", caretRTL.LogicalToVisual, want)
	}
	if !reflect.DeepEqual(caretRTL.VisualToLogical, want) {
		t.Errorf("VisualToLogical(RTL, BidiOff) = %v, want %v", caretRTL.VisualToLogical, want)
	}
}

func TestVisualClusterWidthsDefaultsAndClamps(t *testing.T) {
	clusters := []visualCluster{
		{text: "\t", width: 0},
		{text: "x", width: 0},
		{text: "世", width: 2},
	}

	// tabSize <= 0 must fall back to 8, and a non-positive cluster width
	// (whether from a tab at column 0 or a stray zero-width cluster) must
	// be clamped up to at least 1 so the caret can never get stuck.
	widths := visualClusterWidths(clusters, 0)
	want := []int{8, 1, 2}
	if !reflect.DeepEqual(widths, want) {
		t.Errorf("visualClusterWidths(tabSize=0) = %v, want %v", widths, want)
	}
}

func TestFragmentLogicalToVisualClampsNegativeOffset(t *testing.T) {
	got := fragmentLogicalToVisual("abc", -5, 8)
	if got != 0 {
		t.Errorf("fragmentLogicalToVisual(-5) = %d, want 0 (clamped like offset 0)", got)
	}
}

func TestFragmentVisualToLogicalRTLBeforeFirstVisualColumn(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	defer func() { vtui.DefaultBidiMode = oldMode }()

	text := "שלום" // pure RTL, 4 clusters, 2 bytes each = 8 bytes
	// Visual column 0 is the screen's leftmost column. For a pure RTL run
	// that is logically *past* the last character, so the caret map has no
	// cluster to return and the function must fall back to len(text).
	got := fragmentVisualToLogical(text, 0, 8)
	if got != len(text) {
		t.Errorf("fragmentVisualToLogical(col 0) = %d, want %d (end of text)", got, len(text))
	}
}

func TestFragmentVisualMoveEmptyText(t *testing.T) {
	got, ok := fragmentVisualMove("", 0, 1)
	if ok {
		t.Fatalf("fragmentVisualMove(\"\") should refuse to move, got ok=true offset=%d", got)
	}
	if got != 0 {
		t.Errorf("fragmentVisualMove(\"\") offset = %d, want 0", got)
	}
}

func TestFragmentVisualMoveNonRTLBranches(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		byteOffset int
		direction  int
		wantOff    int
		wantOK     bool
	}{
		{"move right lands on next cluster", "abc", 1, 1, 2, true},
		{"move right past the end returns len(text)", "abc", 2, 1, 3, true},
		{"move left past the start is refused", "abc", 0, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := fragmentVisualMove(tt.text, tt.byteOffset, tt.direction)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (offset %d)", ok, tt.wantOK, got)
			}
			if got != tt.wantOff {
				t.Errorf("offset = %d, want %d", got, tt.wantOff)
			}
		})
	}
}

func TestFragmentVisualMoveRTLReachesLogicalEnd(t *testing.T) {
	oldMode := vtui.DefaultBidiMode
	vtui.DefaultBidiMode = vtui.BidiFull
	defer func() { vtui.DefaultBidiMode = oldMode }()

	text := "אבג" // 3 pure-RTL letters, 2 bytes each = 6 bytes
	// Byte offset 4 sits right before the last logical letter, which is
	// visual position 1. Moving one more step in the visual direction of
	// "left" (-1) reaches visual position 0 - the far edge of the run -
	// which has no logical cluster of its own, so it must fall back to the
	// end of the text.
	got, ok := fragmentVisualMove(text, 4, -1)
	if !ok {
		t.Fatalf("fragmentVisualMove refused the move")
	}
	if got != len(text) {
		t.Errorf("fragmentVisualMove(RTL, 4, -1) = %d, want %d (end of text)", got, len(text))
	}
}

func TestWrapEngine_MoveVisual(t *testing.T) {
	newEngine := func() *WrapEngine {
		Pt := piecetable.New([]byte("ab\ncd"))
		Li := piecetable.NewLineIndex()
		Li.Rebuild(Pt)
		we := NewWrapEngine(Pt, Li)
		we.SetWidth(80)
		we.ToggleWrap(false)
		return we
	}

	t.Run("direction zero is a no-op", func(t *testing.T) {
		we := newEngine()
		if got := we.MoveVisual(1, 0); got != 1 {
			t.Errorf("MoveVisual(1, 0) = %d, want 1", got)
		}
	})

	t.Run("moves right within the same line", func(t *testing.T) {
		we := newEngine()
		if got := we.MoveVisual(1, 1); got != 2 {
			t.Errorf("MoveVisual(1, 1) = %d, want 2", got)
		}
	})

	t.Run("moving right off the end crosses into the next line", func(t *testing.T) {
		we := newEngine()
		if got := we.MoveVisual(2, 1); got != 3 {
			t.Errorf("MoveVisual(2, 1) = %d, want 3 (start of \"cd\")", got)
		}
	})

	t.Run("moving left off the start crosses into the previous line", func(t *testing.T) {
		we := newEngine()
		if got := we.MoveVisual(3, -1); got != 2 {
			t.Errorf("MoveVisual(3, -1) = %d, want 2 (end of \"ab\")", got)
		}
	})

	t.Run("moving left at the very start of the document is a no-op", func(t *testing.T) {
		we := newEngine()
		if got := we.MoveVisual(0, -1); got != 0 {
			t.Errorf("MoveVisual(0, -1) = %d, want 0", got)
		}
	})
}

func TestWrapEngine_SetTabSizeNonPositiveDefaultsToEight(t *testing.T) {
	Pt := piecetable.New([]byte("\t"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.ToggleWrap(false)

	we.SetTabSize(4)
	if got := we.GetFragments(0)[0].VisualWidth; got != 4 {
		t.Fatalf("setup: TabSize 4 gave width %d, want 4", got)
	}

	we.SetTabSize(0)
	if got := we.GetFragments(0)[0].VisualWidth; got != 8 {
		t.Errorf("SetTabSize(0) width = %d, want 8 (default)", got)
	}

	we.SetTabSize(-3)
	if we.tabSize != 8 {
		t.Errorf("SetTabSize(-3) tabSize = %d, want 8 (default)", we.tabSize)
	}
}

func TestWrapEngine_GetFragmentsOutOfRange(t *testing.T) {
	Pt := piecetable.New([]byte("only line"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)

	if frags := we.GetFragments(-1); frags != nil {
		t.Errorf("GetFragments(-1) = %v, want nil", frags)
	}
	if frags := we.GetFragments(5); frags != nil {
		t.Errorf("GetFragments(5) = %v, want nil (only 1 line)", frags)
	}
}

func TestWrapEngine_EnsureRowCountCacheNoOpWithoutWordWrap(t *testing.T) {
	Pt := piecetable.New([]byte("a\nb\nc"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.ToggleWrap(false)

	we.ensureRowCountCache(10)
	if we.rowOffsets != nil {
		t.Errorf("rowOffsets = %v, want nil (must stay untouched when word wrap is off)", we.rowOffsets)
	}
	if we.validUntil != -1 {
		t.Errorf("validUntil = %d, want -1", we.validUntil)
	}
}

func TestWrapEngine_GetRowOffsetBoundaries(t *testing.T) {
	Pt := piecetable.New([]byte("L0\nL1\nL2"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	t.Run("wrap off, negative index clamps to zero", func(t *testing.T) {
		we.ToggleWrap(false)
		if got := we.GetRowOffset(-5); got != 0 {
			t.Errorf("GetRowOffset(-5) = %d, want 0", got)
		}
	})

	t.Run("wrap off, index beyond line count clamps to line count", func(t *testing.T) {
		we.ToggleWrap(false)
		if got := we.GetRowOffset(100); got != 3 {
			t.Errorf("GetRowOffset(100) = %d, want 3 (line count)", got)
		}
	})

	t.Run("wrap on, negative index clamps to zero", func(t *testing.T) {
		we.ToggleWrap(true)
		if got := we.GetRowOffset(-1); got != 0 {
			t.Errorf("GetRowOffset(-1) = %d, want 0", got)
		}
	})

	t.Run("wrap on, index beyond line count returns total rows", func(t *testing.T) {
		we.ToggleWrap(true)
		total := we.GetTotalVisualRows()
		if got := we.GetRowOffset(999); got != total {
			t.Errorf("GetRowOffset(999) = %d, want total rows %d", got, total)
		}
	})
}

func TestWrapEngine_GetLogLineAtVisualRowBoundaries(t *testing.T) {
	Pt := piecetable.New([]byte("L0\nL1\nL2"))
	Li := piecetable.NewLineIndex()
	Li.Rebuild(Pt)
	we := NewWrapEngine(Pt, Li)
	we.SetWidth(80)

	t.Run("negative row clamps to the start", func(t *testing.T) {
		logLine, frag := we.GetLogLineAtVisualRow(-1)
		if logLine != 0 || frag != 0 {
			t.Errorf("GetLogLineAtVisualRow(-1) = (%d,%d), want (0,0)", logLine, frag)
		}
	})

	t.Run("wrap off, row beyond line count clamps to the last line", func(t *testing.T) {
		we.ToggleWrap(false)
		logLine, frag := we.GetLogLineAtVisualRow(100)
		if logLine != 2 || frag != 0 {
			t.Errorf("GetLogLineAtVisualRow(100) with wrap off = (%d,%d), want (2,0)", logLine, frag)
		}
	})

	t.Run("wrap on, row beyond total after full cache clamps to the last line", func(t *testing.T) {
		we.ToggleWrap(true)
		total := we.GetTotalVisualRows()
		logLine, frag := we.GetLogLineAtVisualRow(total + 50)
		if logLine != 2 || frag != 0 {
			t.Errorf("GetLogLineAtVisualRow(total+50) = (%d,%d), want (2,0)", logLine, frag)
		}
	})
}
