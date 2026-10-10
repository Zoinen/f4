package terminal

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

// termRowText renders a TerminalView row as a plain string, for assertions
// that read better as "AB   " than as a slice of CharInfo.
func termRowText(tv *TerminalView, row int) string {
	var sb strings.Builder
	for _, c := range tv.Lines[row] {
		sb.WriteRune(rune(c.Char))
	}
	return sb.String()
}

func setRowText(tv *TerminalView, row int, text string) {
	for i, ch := range text {
		tv.Lines[row][i] = vtui.CharInfo{Char: uint64(ch), Attributes: DefaultTermAttr}
	}
}

// TestTerminalView_MutedGuardSuppressesStateMutation exercises the "if
// tv.Muted { return }" guard present at the top of every grid-mutating
// method on TerminalView. Each of these guards is normally invisible: every
// other test in the package drives the terminal unmuted, so the branch
// where the guard actually fires had no coverage at all. A regression here
// (e.g. a new method forgetting the guard, or an existing one losing it)
// would let a backgrounded/detached session keep painting over a grid a
// caller expected to stay frozen -- see TestTerminalView_MutedStateAndOSC133
// for why muting exists in the first place.
func TestTerminalView_MutedGuardSuppressesStateMutation(t *testing.T) {
	type snapshot struct {
		buf              [][]vtui.CharInfo
		cursorX, cursorY int
		savedX, savedY   int
	}
	snap := func(tv *TerminalView) snapshot {
		buf := tv.GetBuffer()
		cp := make([][]vtui.CharInfo, len(buf))
		for i, row := range buf {
			cp[i] = append([]vtui.CharInfo(nil), row...)
		}
		return snapshot{buf: cp, cursorX: tv.CursorX, cursorY: tv.CursorY, savedX: tv.decSavedX, savedY: tv.decSavedY}
	}
	equal := func(a, b snapshot) bool {
		if a.cursorX != b.cursorX || a.cursorY != b.cursorY || a.savedX != b.savedX || a.savedY != b.savedY {
			return false
		}
		if len(a.buf) != len(b.buf) {
			return false
		}
		for i := range a.buf {
			if len(a.buf[i]) != len(b.buf[i]) {
				return false
			}
			for j := range a.buf[i] {
				if a.buf[i][j] != b.buf[i][j] {
					return false
				}
			}
		}
		return true
	}

	tests := []struct {
		name string
		run  func(tv *TerminalView)
	}{
		{"PutChar", func(tv *TerminalView) { tv.PutChar('Z', DefaultTermAttr) }},
		{"Index", func(tv *TerminalView) { tv.Index() }},
		{"ReverseIndex", func(tv *TerminalView) { tv.ReverseIndex() }},
		{"NextLine", func(tv *TerminalView) { tv.NextLine() }},
		{"ScrollDown", func(tv *TerminalView) { tv.ScrollDown(0, tv.Height-1, 1) }},
		{"DeleteCharacters", func(tv *TerminalView) { tv.DeleteCharacters(2, DefaultTermAttr) }},
		{"InsertBlankCharacters", func(tv *TerminalView) { tv.InsertBlankCharacters(2, DefaultTermAttr) }},
		{"SetCursor", func(tv *TerminalView) { tv.SetCursor(1, 1) }},
		// SaveCursor while muted must not overwrite the previously saved
		// position -- observed indirectly, since decSavedX/decSavedY are
		// unexported, via the snapshot equality check above.
		{"SaveCursor", func(tv *TerminalView) { tv.SaveCursor() }},
		// RestoreCursor while muted must not move the cursor back to
		// whatever was last saved (which differs from the current
		// position here, since no SaveCursor call preceded this one).
		{"RestoreCursor", func(tv *TerminalView) { tv.RestoreCursor() }},
		{"EraseCharacter", func(tv *TerminalView) { tv.EraseCharacter(3, DefaultTermAttr) }},
		{"EraseDisplay", func(tv *TerminalView) { tv.EraseDisplay(2, DefaultTermAttr) }},
		{"EraseLine", func(tv *TerminalView) { tv.EraseLine(2, DefaultTermAttr) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tv := NewTerminalView(10, 4)
			defer tv.Close()
			tv.SetCursor(3, 2)
			tv.Lines[2][3] = vtui.CharInfo{Char: 'M', Attributes: DefaultTermAttr}

			before := snap(tv)
			tv.SetMuted(true)
			tc.run(tv)
			after := snap(tv)

			if !equal(before, after) {
				t.Errorf("%s mutated state while Muted: before=%+v after=%+v", tc.name, before, after)
			}
		})
	}
}

// TestTerminalView_IndexReverseIndexStepWithinMargins covers the plain
// cursor-move branch of Index/ReverseIndex -- the case where the cursor is
// not sitting on the scroll margin, so the function only has to increment
// or decrement CursorY without touching the grid at all. Every existing
// test for these two methods (TestAnsiParser_HandleEsc_IndexScrollsAtBottomMargin
// and its ReverseIndex counterpart) places the cursor exactly on the
// margin, which only exercises the scrolling branch.
func TestTerminalView_IndexReverseIndexStepWithinMargins(t *testing.T) {
	t.Run("Index", func(t *testing.T) {
		tv := NewTerminalView(5, 4)
		defer tv.Close()
		setRowText(tv, 0, "A")
		setRowText(tv, 1, "B")
		setRowText(tv, 2, "C")
		setRowText(tv, 3, "D")
		tv.SetCursor(0, 1) // ScrollBottom is 3, one row below is still inside the margin

		tv.Index()

		if tv.CursorY != 2 {
			t.Fatalf("Index within margin: CursorY=%d, want 2", tv.CursorY)
		}
		for y, want := range []string{"A", "B", "C", "D"} {
			if got := string(rune(tv.Lines[y][0].Char)); got != want {
				t.Fatalf("Index within margin scrolled row %d unexpectedly: got %q, want %q", y, got, want)
			}
		}
	})

	t.Run("ReverseIndex", func(t *testing.T) {
		tv := NewTerminalView(5, 4)
		defer tv.Close()
		setRowText(tv, 0, "A")
		setRowText(tv, 1, "B")
		setRowText(tv, 2, "C")
		setRowText(tv, 3, "D")
		tv.SetCursor(0, 2) // ScrollTop is 0, one row above is still inside the margin

		tv.ReverseIndex()

		if tv.CursorY != 1 {
			t.Fatalf("ReverseIndex within margin: CursorY=%d, want 1", tv.CursorY)
		}
		for y, want := range []string{"A", "B", "C", "D"} {
			if got := string(rune(tv.Lines[y][0].Char)); got != want {
				t.Fatalf("ReverseIndex within margin scrolled row %d unexpectedly: got %q, want %q", y, got, want)
			}
		}
	})
}

// TestTerminalView_SetCursorClampsOutOfRangeCoordinates covers SetCursor's
// four boundary clamps. Nothing else in the package calls SetCursor with an
// out-of-range coordinate (every caller either passes sane fixed values or
// values already clamped by a CSI parameter parser), so the clamps
// themselves -- as opposed to the values they let through -- had no test.
func TestTerminalView_SetCursorClampsOutOfRangeCoordinates(t *testing.T) {
	tests := []struct {
		name         string
		x, y         int
		wantX, wantY int
	}{
		{"NegativeX", -5, 2, 0, 2},
		{"NegativeY", 2, -5, 2, 0},
		{"BothNegative", -1, -1, 0, 0},
		{"XTooLarge", 999, 2, 9, 2},
		{"YTooLarge", 2, 999, 2, 3},
		{"BothTooLarge", 999, 999, 9, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tv := NewTerminalView(10, 4)
			defer tv.Close()

			tv.SetCursor(tc.x, tc.y)

			if tv.CursorX != tc.wantX || tv.CursorY != tc.wantY {
				t.Errorf("SetCursor(%d,%d) = (%d,%d), want (%d,%d)", tc.x, tc.y, tv.CursorX, tv.CursorY, tc.wantX, tc.wantY)
			}
		})
	}
}

// TestTerminalView_EraseLineModes covers all three ECH... erm, EL (CSI K)
// modes directly against TerminalView.EraseLine. Nothing else in the
// package -- not a single test file -- sends CSI K through the parser or
// calls EraseLine directly, even though it is one of the most common
// operations a real shell prompt issues.
func TestTerminalView_EraseLineModes(t *testing.T) {
	const row = 1
	setup := func() *TerminalView {
		tv := NewTerminalView(5, 3)
		setRowText(tv, row, "ABCDE")
		tv.WrapFlags[row] = true
		tv.SetCursor(2, row)
		return tv
	}

	t.Run("Mode0_CursorToEndOfLine", func(t *testing.T) {
		tv := setup()
		defer tv.Close()

		tv.EraseLine(0, DefaultTermAttr)

		if got := termRowText(tv, row); got != "AB   " {
			t.Fatalf("EraseLine(0): row=%q, want %q", got, "AB   ")
		}
		if !tv.WrapFlags[row] {
			t.Fatalf("EraseLine(0) away from column 0 must not clear WrapFlags")
		}
	})

	t.Run("Mode1_StartOfLineToCursor", func(t *testing.T) {
		tv := setup()
		defer tv.Close()

		tv.EraseLine(1, DefaultTermAttr)

		if got := termRowText(tv, row); got != "   DE" {
			t.Fatalf("EraseLine(1): row=%q, want %q", got, "   DE")
		}
		if !tv.WrapFlags[row] {
			t.Fatalf("EraseLine(1) must not clear WrapFlags")
		}
	})

	t.Run("Mode2_WholeLine", func(t *testing.T) {
		tv := setup()
		defer tv.Close()

		tv.EraseLine(2, DefaultTermAttr)

		if got := termRowText(tv, row); got != "     " {
			t.Fatalf("EraseLine(2): row=%q, want blank", got)
		}
		if tv.WrapFlags[row] {
			t.Fatalf("EraseLine(2) must clear WrapFlags")
		}
	})

	t.Run("Mode0_AtColumnZeroClearsWrapFlag", func(t *testing.T) {
		tv := setup()
		defer tv.Close()
		tv.SetCursor(0, row)

		tv.EraseLine(0, DefaultTermAttr)

		if got := termRowText(tv, row); got != "     " {
			t.Fatalf("EraseLine(0) at column 0: row=%q, want blank", got)
		}
		if tv.WrapFlags[row] {
			t.Fatalf("EraseLine(0) at column 0 must clear WrapFlags")
		}
	})
}

// TestTerminalView_EraseLineGuardsOutOfRangeCursorY covers the defensive
// bounds check that keeps EraseLine from indexing the grid with a CursorY
// that has drifted outside [0, len(buf)). Normal callers never produce such
// a CursorY (SetCursor clamps it), but the guard exists precisely for the
// case where something else does, and a regression there is a panic, not a
// silently wrong screen.
func TestTerminalView_EraseLineGuardsOutOfRangeCursorY(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	setRowText(tv, 0, "ABCDE")

	tv.CursorY = -1
	tv.EraseLine(2, DefaultTermAttr)
	if got := termRowText(tv, 0); got != "ABCDE" {
		t.Fatalf("EraseLine with CursorY=-1 mutated row 0: got %q", got)
	}

	tv.CursorY = tv.Height
	tv.EraseLine(2, DefaultTermAttr)
	if got := termRowText(tv, 0); got != "ABCDE" {
		t.Fatalf("EraseLine with CursorY=Height mutated row 0: got %q", got)
	}
}

// TestTerminalView_EraseDisplayModeZero covers ED mode 0 (erase from the
// cursor to the end of the screen): the current row is blanked from the
// cursor column onward, every row below is blanked in full, and everything
// above and to the left of the cursor is left alone. The only existing
// EraseDisplay coverage in the package drives mode 2 (full-screen clear).
func TestTerminalView_EraseDisplayModeZero(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	setRowText(tv, 0, "ABCDE")
	setRowText(tv, 1, "FGHIJ")
	setRowText(tv, 2, "KLMNO")
	tv.SetCursor(2, 1)

	tv.EraseDisplay(0, DefaultTermAttr)

	if got := termRowText(tv, 0); got != "ABCDE" {
		t.Fatalf("EraseDisplay(0) touched a row above the cursor: got %q", got)
	}
	if got := termRowText(tv, 1); got != "FG   " {
		t.Fatalf("EraseDisplay(0) cursor row: got %q, want %q", got, "FG   ")
	}
	if got := termRowText(tv, 2); got != "     " {
		t.Fatalf("EraseDisplay(0) row below the cursor: got %q, want blank", got)
	}
}

// TestTerminalView_EraseDisplayModeZeroGuardsOutOfRangeCursorY covers the
// boundary check on the "rows below the cursor" loop: with a CursorY past
// the last row, the loop bound (CursorY+1) exceeds Height and the loop body
// never runs, so mode 0 becomes a correctly-behaving no-op instead of
// indexing past the grid.
func TestTerminalView_EraseDisplayModeZeroGuardsOutOfRangeCursorY(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	setRowText(tv, 0, "ABCDE")
	setRowText(tv, 1, "FGHIJ")
	setRowText(tv, 2, "KLMNO")
	tv.CursorY = tv.Height // one past the last valid row

	tv.EraseDisplay(0, DefaultTermAttr)

	if got := termRowText(tv, 0); got != "ABCDE" {
		t.Fatalf("EraseDisplay(0) with out-of-range CursorY mutated row 0: got %q", got)
	}
	if got := termRowText(tv, 1); got != "FGHIJ" {
		t.Fatalf("EraseDisplay(0) with out-of-range CursorY mutated row 1: got %q", got)
	}
	if got := termRowText(tv, 2); got != "KLMNO" {
		t.Fatalf("EraseDisplay(0) with out-of-range CursorY mutated row 2: got %q", got)
	}
}

// TestTerminalView_EraseDisplayModeOneErasesAboveCursor covers ED mode 1
// ("Erase Above"): everything from the top-left corner through the cursor,
// inclusive, is blanked, and rows below the cursor are untouched.
//
// This is the bug this step found and fixed: EraseDisplay's switch had
// cases for mode 2 (full screen) and mode 0 (cursor to end), but no case
// for mode 1 at all, so a program sending CSI 1 J saw no effect whatsoever
// -- silently dropped instead of erasing anything. EraseLine (CSI K)
// already implements the equivalent "start of line to cursor" rule for
// mode 1, which is what the fix mirrors.
func TestTerminalView_EraseDisplayModeOneErasesAboveCursor(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	setRowText(tv, 0, "ABCDE")
	setRowText(tv, 1, "FGHIJ")
	setRowText(tv, 2, "KLMNO")
	tv.SetCursor(2, 1)

	tv.EraseDisplay(1, DefaultTermAttr)

	if got := termRowText(tv, 0); got != "     " {
		t.Fatalf("EraseDisplay(1) row above the cursor: got %q, want blank", got)
	}
	if got := termRowText(tv, 1); got != "   IJ" {
		t.Fatalf("EraseDisplay(1) cursor row: got %q, want %q", got, "   IJ")
	}
	if got := termRowText(tv, 2); got != "KLMNO" {
		t.Fatalf("EraseDisplay(1) touched a row below the cursor: got %q", got)
	}
}

// TestTerminalView_DeleteInsertCharactersGuardOutOfRangeCursor covers the
// defensive bounds checks in DeleteCharacters and InsertBlankCharacters.
// The happy path for both is already exercised through CSI P/@ (see
// ansi_test.go), but always with a cursor SetCursor already clamped into
// range, so the "CursorY/CursorX drifted out of range" early-return guards
// were never actually taken.
func TestTerminalView_DeleteInsertCharactersGuardOutOfRangeCursor(t *testing.T) {
	const row = 1
	ops := []struct {
		name string
		call func(tv *TerminalView)
	}{
		{"DeleteCharacters", func(tv *TerminalView) { tv.DeleteCharacters(2, DefaultTermAttr) }},
		{"InsertBlankCharacters", func(tv *TerminalView) { tv.InsertBlankCharacters(2, DefaultTermAttr) }},
	}
	cursors := []struct {
		name string
		x, y int
	}{
		{"CursorYNegative", 2, -1},
		{"CursorYTooLarge", 2, 100},
		{"CursorXNegative", -1, row},
		{"CursorXTooLarge", 100, row},
	}

	for _, op := range ops {
		for _, c := range cursors {
			t.Run(op.name+"_"+c.name, func(t *testing.T) {
				tv := NewTerminalView(5, 3)
				defer tv.Close()
				setRowText(tv, row, "ABCDE")
				tv.CursorX, tv.CursorY = c.x, c.y

				op.call(tv) // must not panic

				if got := termRowText(tv, row); got != "ABCDE" {
					t.Fatalf("%s with out-of-range cursor (%d,%d) mutated row %d: got %q", op.name, c.x, c.y, row, got)
				}
			})
		}
	}
}

// TestTerminalView_EraseCharacterGuardsOutOfRangeCursorY covers the same
// kind of defensive bounds check in EraseCharacter (CSI X). The happy path
// is covered by TestAnsiParser's CSI 'X' case in ansi_test.go, always with
// an in-range cursor.
func TestTerminalView_EraseCharacterGuardsOutOfRangeCursorY(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	setRowText(tv, 1, "ABCDE")

	tv.CursorX, tv.CursorY = 2, -1
	tv.EraseCharacter(3, DefaultTermAttr)
	if got := termRowText(tv, 1); got != "ABCDE" {
		t.Fatalf("EraseCharacter with CursorY=-1 mutated row 1: got %q", got)
	}

	tv.CursorY = tv.Height
	tv.EraseCharacter(3, DefaultTermAttr)
	if got := termRowText(tv, 1); got != "ABCDE" {
		t.Fatalf("EraseCharacter with CursorY=Height mutated row 1: got %q", got)
	}
}
