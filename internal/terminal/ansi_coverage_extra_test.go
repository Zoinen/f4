package terminal

import (
	"fmt"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/unxed/vtui"
)

// This file targets branches of internal/terminal/ansi.go that the existing
// ansi_test.go / ansi_sync_test.go suites do not reach: the kitty keyboard
// protocol's push/pop/query forms of CSI u, XTSMGRAPHICS (CSI ? Pi ; Pa ; Pv
// S), several SGR style bits and extended-color combinations, malformed OSC
// payloads, ESC dispatch (index/reverse-index/next-line/RIS/keypad), DECSTBM,
// the insert/delete-line and scroll-down CSI commands, and the parser's
// intermediate-byte / DCS state transitions. managed_exec.go/test are
// untouched, per the current moratorium on that file.

// charQuote renders a ViewCell.Char (vtui.CharInfo.Char, a uint64 by field
// type but always a decoded Unicode code point in practice) as a quoted rune
// for Fatalf messages, without an unchecked uint64->rune conversion.
func charQuote(c uint64) string {
	if c > utf8.MaxRune {
		return strconv.FormatUint(c, 10)
	}
	// #nosec G115 -- range-checked above: c <= utf8.MaxRune fits in a rune.
	return strconv.QuoteRune(rune(c))
}

// --- Kitty keyboard protocol progressive enhancement flags (CSI ... u) ---
//
// TestKittyEnableDisambiguateSeq_SetsDisambiguateFlag (ansi_test.go) only
// exercises the "=1;1u" plain-assignment form. The OR-in (mode 2), AND-NOT
// (mode 3), push (">"), pop ("<"), query ("?") and the numeric-only fallback
// to RestoreCursor were entirely unexercised.

func TestAnsiParser_KittyFlags_SetMasks(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Mode 1: plain assignment.
	p.Process([]byte("\x1b[=5;1u"))
	if got := tv.KittyFlags.Load(); got != 5 {
		t.Fatalf("mode 1 (assign): flags=%d, want 5", got)
	}

	// Mode 2: OR the new bits in.
	p.Process([]byte("\x1b[=2;2u"))
	if got := tv.KittyFlags.Load(); got != 7 { // 5|2
		t.Fatalf("mode 2 (OR): flags=%d, want 7", got)
	}

	// Mode 3: AND-NOT clears exactly the named bits, leaving the rest.
	p.Process([]byte("\x1b[=2;3u"))
	if got := tv.KittyFlags.Load(); got != 5 { // 7 &^ 2
		t.Fatalf("mode 3 (AND-NOT): flags=%d, want 5", got)
	}
}

func TestAnsiParser_KittyFlags_PushPopQuery(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &mockPty{}
	p := NewAnsiParser(tv, Pty)

	p.Process([]byte("\x1b[=2;1u")) // flags = 2, stack empty

	p.Process([]byte("\x1b[>5u")) // push (saves 2), flags = 5
	if got := tv.KittyFlags.Load(); got != 5 {
		t.Fatalf("push: flags=%d, want 5", got)
	}
	if len(tv.KittyFlagsStack) != 1 || tv.KittyFlagsStack[0] != 2 {
		t.Fatalf("push: stack=%v, want [2]", tv.KittyFlagsStack)
	}

	p.Process([]byte("\x1b[>9u")) // push (saves 5), flags = 9
	if got := tv.KittyFlags.Load(); got != 9 {
		t.Fatalf("second push: flags=%d, want 9", got)
	}
	if len(tv.KittyFlagsStack) != 2 || tv.KittyFlagsStack[1] != 5 {
		t.Fatalf("second push: stack=%v, want [2 5]", tv.KittyFlagsStack)
	}

	// Query answers with the flags currently in effect.
	Pty.Reset()
	p.Process([]byte("\x1b[?u"))
	if Pty.String() != "\x1b[?9u" {
		t.Fatalf("query: got %q, want %q", Pty.String(), "\x1b[?9u")
	}

	p.Process([]byte("\x1b[<u")) // pop one (default count = 1) -> back to 5
	if got := tv.KittyFlags.Load(); got != 5 {
		t.Fatalf("pop default count: flags=%d, want 5", got)
	}
	if len(tv.KittyFlagsStack) != 1 {
		t.Fatalf("pop default count: stack=%v, want length 1", tv.KittyFlagsStack)
	}

	p.Process([]byte("\x1b[<5u")) // pop more than the stack holds -> resets to 0
	if got := tv.KittyFlags.Load(); got != 0 {
		t.Fatalf("pop past empty stack: flags=%d, want 0", got)
	}
	if len(tv.KittyFlagsStack) != 0 {
		t.Fatalf("pop past empty stack: stack=%v, want empty", tv.KittyFlagsStack)
	}

	// Popping again with nothing left must not panic or go negative.
	p.Process([]byte("\x1b[<u"))
	if got := tv.KittyFlags.Load(); got != 0 {
		t.Fatalf("pop on empty stack: flags=%d, want 0", got)
	}
}

func TestAnsiParser_KittyFlags_StackIsCappedAt32(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	for i := 1; i <= 33; i++ {
		p.Process([]byte(fmt.Sprintf("\x1b[>%du", i)))
	}

	if got := tv.KittyFlags.Load(); got != 33 {
		t.Fatalf("after 33 pushes: flags=%d, want 33", got)
	}
	if len(tv.KittyFlagsStack) != 32 {
		t.Fatalf("stack size=%d, want capped at 32", len(tv.KittyFlagsStack))
	}
	// The oldest saved value (0, from the very first push) must have been
	// evicted; the next-oldest surviving one is 1.
	if tv.KittyFlagsStack[0] != 1 {
		t.Fatalf("oldest surviving stack entry=%d, want 1 (0 should have been evicted)", tv.KittyFlagsStack[0])
	}
}

func TestAnsiParser_KittyFlags_PlainUFallsBackToRestoreCursor(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(5, 5)
	p.Process([]byte("\x1b[s")) // DECSC save at (5,5)
	tv.SetCursor(0, 0)

	// A numeric-only "u" (no =, >, <, ? kitty marker) falls through to the
	// same restore xterm gives plain "u".
	p.Process([]byte("\x1b[3u"))
	if tv.CursorX != 5 || tv.CursorY != 5 {
		t.Fatalf("plain CSI u: cursor=(%d,%d), want (5,5) restored", tv.CursorX, tv.CursorY)
	}
}

// --- XTSMGRAPHICS (CSI ? Pi ; Pa ; Pv S) ---

func TestAnsiParser_GraphicsAttributes_ColorRegisters(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &mockPty{}
	p := NewAnsiParser(tv, Pty)

	// item=1 (colour registers), action in {1,2,3,4} reports success with
	// the register count.
	p.Process([]byte("\x1b[?1;1S"))
	if Pty.String() != "\x1b[?1;0;256S" {
		t.Fatalf("item 1 action 1: got %q", Pty.String())
	}
	Pty.Reset()

	// Any other action code is an unsupported request.
	p.Process([]byte("\x1b[?1;9S"))
	if Pty.String() != "\x1b[?1;2S" {
		t.Fatalf("item 1 unsupported action: got %q", Pty.String())
	}
}

func TestAnsiParser_GraphicsAttributes_RasterGeometry(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &mockPty{}
	p := NewAnsiParser(tv, Pty)

	// Fresh TerminalView has no host-reported cell size, so CellSize()
	// falls back to 8x16: 80*8=640, 24*16=384.
	p.Process([]byte("\x1b[?2;1S"))
	if Pty.String() != "\x1b[?2;0;640;384S" {
		t.Fatalf("item 2 action 1: got %q", Pty.String())
	}
	Pty.Reset()

	p.Process([]byte("\x1b[?2;9S"))
	if Pty.String() != "\x1b[?2;2S" {
		t.Fatalf("item 2 unsupported action: got %q", Pty.String())
	}
}

func TestAnsiParser_GraphicsAttributes_RasterGeometryIsClampedToSixelMaxSide(t *testing.T) {
	// Width 8200 * fallback cell width 8 = 65600, which is over sixelMaxSide
	// (65535) and must be clamped.
	tvWide := NewTerminalView(8200, 2)
	defer tvWide.Close()
	PtyWide := &mockPty{}
	pWide := NewAnsiParser(tvWide, PtyWide)
	pWide.Process([]byte("\x1b[?2;1S"))
	if want := "\x1b[?2;0;65535;32S"; PtyWide.String() != want {
		t.Fatalf("width clamp: got %q, want %q", PtyWide.String(), want)
	}

	// Height 4100 * fallback cell height 16 = 65600, same clamp on the
	// other axis.
	tvTall := NewTerminalView(2, 4100)
	defer tvTall.Close()
	PtyTall := &mockPty{}
	pTall := NewAnsiParser(tvTall, PtyTall)
	pTall.Process([]byte("\x1b[?2;1S"))
	if want := "\x1b[?2;0;16;65535S"; PtyTall.String() != want {
		t.Fatalf("height clamp: got %q, want %q", PtyTall.String(), want)
	}
}

func TestAnsiParser_GraphicsAttributes_UnknownItem(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &mockPty{}
	p := NewAnsiParser(tv, Pty)

	// Items other than 1 (colour registers) and 2 (raster geometry), such
	// as ReGIS geometry, always answer "unsupported" without looking at
	// the action code.
	p.Process([]byte("\x1b[?5;1S"))
	if Pty.String() != "\x1b[?5;1S" {
		t.Fatalf("unknown item: got %q", Pty.String())
	}
}

func TestAnsiParser_GraphicsAttributes_NoPtyDoesNotPanic(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// replyPty() is nil; handleGraphicsAttributes must bail out quietly
	// instead of writing to a nil PTY.
	p.Process([]byte("\x1b[?1;1S"))
	p.Process([]byte("\x1b[?2;1S"))
	p.Process([]byte("\x1b[?9;1S"))
}

// --- SGR style bits that TestAnsiParser_SGR_Advanced does not touch ---

func TestAnsiParser_SGR_StyleTogglesAndResets(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// Dim (2) sets ForegroundDim; 22 clears both Bold and Dim.
	p.Process([]byte("\x1b[0m"))
	p.Process([]byte("\x1b[1;2m"))
	if p.Attr&vtui.ForegroundIntensity == 0 || p.Attr&vtui.ForegroundDim == 0 {
		t.Fatal("1;2m: expected both Bold and Dim set")
	}
	p.Process([]byte("\x1b[22m"))
	if p.Attr&(vtui.ForegroundIntensity|vtui.ForegroundDim) != 0 {
		t.Fatal("22m: expected Bold and Dim cleared")
	}

	// Underline (4) / not-underlined (24).
	p.Process([]byte("\x1b[0m\x1b[4m"))
	if p.Attr&vtui.CommonLvbUnderscore == 0 {
		t.Fatal("4m: expected Underline set")
	}
	p.Process([]byte("\x1b[24m"))
	if p.Attr&vtui.CommonLvbUnderscore != 0 {
		t.Fatal("24m: expected Underline cleared")
	}

	// Reverse (7) / not-reversed (27).
	p.Process([]byte("\x1b[0m\x1b[7m"))
	if p.Attr&vtui.CommonLvbReverse == 0 {
		t.Fatal("7m: expected Reverse set")
	}
	p.Process([]byte("\x1b[27m"))
	if p.Attr&vtui.CommonLvbReverse != 0 {
		t.Fatal("27m: expected Reverse cleared")
	}

	// Strikeout (9) / not-strikeout (29).
	p.Process([]byte("\x1b[0m\x1b[9m"))
	if p.Attr&vtui.CommonLvbStrikeout == 0 {
		t.Fatal("9m: expected Strikeout set")
	}
	p.Process([]byte("\x1b[29m"))
	if p.Attr&vtui.CommonLvbStrikeout != 0 {
		t.Fatal("29m: expected Strikeout cleared")
	}

	// Blink (5) is a recognized, intentionally-ignored no-op: the
	// attribute word must come out unchanged.
	p.Process([]byte("\x1b[0m"))
	before := p.Attr
	p.Process([]byte("\x1b[5m"))
	if p.Attr != before {
		t.Fatalf("5m: expected no attribute change, got %#x (was %#x)", p.Attr, before)
	}
}

func TestAnsiParser_SGR_ExtendedColorBranches(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 38;5;N: 256-colour foreground (the 38;2;... truecolor branch is
	// already covered by TestAnsiParser_SGR_Advanced).
	p.Process([]byte("\x1b[0m\x1b[38;5;196m"))
	if got := vtui.GetIndexFore(p.Attr); got != 196 {
		t.Fatalf("38;5;196: fg index=%d, want 196", got)
	}

	// An out-of-range 256-colour index must be ignored, leaving the
	// attribute untouched.
	p.Process([]byte("\x1b[0m\x1b[38;5;999m"))
	if got := vtui.GetIndexFore(p.Attr); got != vtui.GetIndexFore(DefaultTermAttr) {
		t.Fatalf("38;5;999 (out of range): fg index=%d, want default %d", got, vtui.GetIndexFore(DefaultTermAttr))
	}

	// 48;2;R;G;B: truecolor background (the 48;5;... 256-colour branch is
	// already covered by TestAnsiParser_SGR_Advanced).
	p.Process([]byte("\x1b[0m\x1b[48;2;10;20;30m"))
	if p.Attr&vtui.IsBgRGB == 0 {
		t.Fatal("48;2;...: IsBgRGB flag not set")
	}
	if got, want := vtui.GetRGBBack(p.Attr), uint32(0x0A141E); got != want {
		t.Fatalf("48;2;10;20;30: bg RGB=%06X, want %06X", got, want)
	}
}

func TestAnsiParser_SGR_BrightBackgroundAndDefaultReset(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	// 100-107: bright background (90-97, bright foreground, is already
	// covered by TestAnsiParser_SGR_IntensityPersistence).
	p.Process([]byte("\x1b[0m\x1b[105m"))
	if got := vtui.GetIndexBack(p.Attr); got != 13 { // 5 + 8
		t.Fatalf("105m: bg index=%d, want 13", got)
	}

	// 49: restore default background, leaving the foreground alone.
	p.Process([]byte("\x1b[0m\x1b[32;44m")) // green on blue
	p.Process([]byte("\x1b[49m"))
	if got := vtui.GetIndexBack(p.Attr); got != vtui.GetIndexBack(DefaultTermAttr) {
		t.Fatalf("49m: bg index=%d, want default %d", got, vtui.GetIndexBack(DefaultTermAttr))
	}
	if got := vtui.GetIndexFore(p.Attr); got != 2 { // green, untouched by 49m
		t.Fatalf("49m: fg index=%d, want 2 (untouched)", got)
	}
}

// --- Malformed OSC payloads (handleOSC / OSC 4) ---

func TestAnsiParser_OSC_MalformedPayloadsAreIgnored(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"empty payload", "\x1b]\x07"},
		{"no semicolon", "\x1b]nosemicolon\x07"},
		{"non-numeric command", "\x1b]abc;xyz\x07"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tv := NewTerminalView(80, 24)
			defer tv.Close()
			Pty := &mockPty{}
			p := NewAnsiParser(tv, Pty)
			titleBefore := tv.Title

			p.Process([]byte(tc.seq))

			if tv.Title != titleBefore {
				t.Fatalf("title changed to %q for malformed OSC %q", tv.Title, tc.seq)
			}
			if len(Pty.written) != 0 {
				t.Fatalf("unexpected PTY write %q for malformed OSC %q", Pty.String(), tc.seq)
			}
		})
	}
}

func TestAnsiParser_OSC4_MalformedPayloadsAreIgnored(t *testing.T) {
	cases := []struct {
		name string
		seq  string
	}{
		{"missing colour", "\x1b]4;5\x07"},
		{"index too large", "\x1b]4;300;#FFFFFF\x07"},
		{"index negative", "\x1b]4;-1;#FFFFFF\x07"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tv := NewTerminalView(80, 24)
			defer tv.Close()
			p := NewAnsiParser(tv, nil)
			paletteBefore := tv.Palette

			p.Process([]byte(tc.seq)) // must not panic (out-of-range index)

			if tv.Palette != paletteBefore {
				t.Fatalf("palette changed for malformed OSC 4 %q", tc.seq)
			}
		})
	}
}

// --- ESC dispatch: index, reverse index, next line, RIS, keypad no-ops ---

func TestAnsiParser_HandleEsc_IndexScrollsAtBottomMargin(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.Lines[0][0].Char = 'A'
	tv.Lines[1][0].Char = 'B'
	tv.Lines[2][0].Char = 'C'
	tv.SetCursor(0, 2) // bottom row (ScrollBottom == 2)

	p.Process([]byte("\x1bD")) // ESC D: Index

	if tv.CursorY != 2 {
		t.Fatalf("Index at bottom margin: CursorY=%d, want 2 (clamped)", tv.CursorY)
	}
	if tv.Lines[0][0].Char != 'B' || tv.Lines[1][0].Char != 'C' || tv.Lines[2][0].Char != ' ' {
		t.Fatalf("Index at bottom margin: rows=%s/%s/%s, want B/C/blank",
			charQuote(tv.Lines[0][0].Char), charQuote(tv.Lines[1][0].Char), charQuote(tv.Lines[2][0].Char))
	}
}

func TestAnsiParser_HandleEsc_ReverseIndexScrollsAtTopMargin(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.Lines[0][0].Char = 'A'
	tv.Lines[1][0].Char = 'B'
	tv.Lines[2][0].Char = 'C'
	tv.SetCursor(0, 0) // top row (ScrollTop == 0)

	p.Process([]byte("\x1bM")) // ESC M: Reverse Index

	if tv.CursorY != 0 {
		t.Fatalf("Reverse Index at top margin: CursorY=%d, want 0 (clamped)", tv.CursorY)
	}
	if tv.Lines[0][0].Char != ' ' || tv.Lines[1][0].Char != 'A' || tv.Lines[2][0].Char != 'B' {
		t.Fatalf("Reverse Index at top margin: rows=%s/%s/%s, want blank/A/B",
			charQuote(tv.Lines[0][0].Char), charQuote(tv.Lines[1][0].Char), charQuote(tv.Lines[2][0].Char))
	}
}

func TestAnsiParser_HandleEsc_NextLine(t *testing.T) {
	tv := NewTerminalView(10, 5)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(3, 1)
	p.Process([]byte("\x1bE")) // ESC E: Next Line

	if tv.CursorX != 0 || tv.CursorY != 2 {
		t.Fatalf("NextLine: cursor=(%d,%d), want (0,2)", tv.CursorX, tv.CursorY)
	}
}

func TestAnsiParser_HandleEsc_RISResetsToInitialState(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(2, 1)
	tv.Lines[1][2].Char = 'X'

	p.Process([]byte("\x1bc")) // ESC c: RIS

	if tv.CursorX != 0 || tv.CursorY != tv.Height-1 {
		t.Fatalf("RIS: cursor=(%d,%d), want (0,%d)", tv.CursorX, tv.CursorY, tv.Height-1)
	}
	if tv.ScrollTop != 0 || tv.ScrollBottom != tv.Height-1 {
		t.Fatalf("RIS: scroll region=[%d,%d], want [0,%d]", tv.ScrollTop, tv.ScrollBottom, tv.Height-1)
	}
	if tv.Lines[1][2].Char != ' ' {
		t.Fatalf("RIS: screen not cleared, Lines[1][2]=%s", charQuote(tv.Lines[1][2].Char))
	}
}

func TestAnsiParser_HandleEsc_KeypadModesAreNoOpsAndDoNotWedgeTheParser(t *testing.T) {
	tv := NewTerminalView(10, 5)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(0, 0)
	// ESC = (DECKPAM) and ESC > (DECKPNM): both are absorbed silently.
	// A normal character right after must still land on the screen, which
	// proves the parser returned to StateGround instead of getting stuck.
	p.Process([]byte("\x1b=\x1b>Z"))

	if tv.Lines[0][0].Char != 'Z' {
		t.Fatalf("after keypad no-ops: Lines[0][0]=%s, want 'Z'", charQuote(tv.Lines[0][0].Char))
	}
}

// --- DECSTBM (CSI r) ---

func TestAnsiParser_DECSTBM_SetsMargins(t *testing.T) {
	tv := NewTerminalView(20, 10)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.SetCursor(5, 5)
	p.Process([]byte("\x1b[3;7r"))
	if tv.ScrollTop != 2 || tv.ScrollBottom != 6 {
		t.Fatalf("3;7r: scroll region=[%d,%d], want [2,6]", tv.ScrollTop, tv.ScrollBottom)
	}
	if tv.CursorX != 0 || tv.CursorY != 0 {
		t.Fatalf("3;7r: cursor=(%d,%d), want (0,0) (DECSTBM homes the cursor)", tv.CursorX, tv.CursorY)
	}

	// No parameters resets to the full screen.
	p.Process([]byte("\x1b[r"))
	if tv.ScrollTop != 0 || tv.ScrollBottom != tv.Height-1 {
		t.Fatalf("bare r: scroll region=[%d,%d], want [0,%d]", tv.ScrollTop, tv.ScrollBottom, tv.Height-1)
	}
}

// --- Window ops report (CSI 18 t) ---

func TestAnsiParser_WindowOps_TextAreaSizeReport(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	Pty := &mockPty{}
	p := NewAnsiParser(tv, Pty)

	p.Process([]byte("\x1b[18t"))
	if want := "\x1b[8;24;80t"; Pty.String() != want {
		t.Fatalf("18t: got %q, want %q", Pty.String(), want)
	}
}

// --- Insert/delete lines (CSI L / CSI M) and scroll-down (CSI T) ---

func TestAnsiParser_CSI_InsertLinesAtCursor(t *testing.T) {
	tv := NewTerminalView(5, 4)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.Lines[0][0].Char = 'A'
	tv.Lines[1][0].Char = 'B'
	tv.Lines[2][0].Char = 'C'
	tv.Lines[3][0].Char = 'D'
	tv.SetCursor(0, 1)

	p.Process([]byte("\x1b[1L")) // IL: insert one blank line at the cursor row

	// CharInfo.Char is a uint64 (not a byte), so the rows are compared cell
	// by cell rather than packed into a byte array.
	got := [4]uint64{tv.Lines[0][0].Char, tv.Lines[1][0].Char, tv.Lines[2][0].Char, tv.Lines[3][0].Char}
	want := [4]uint64{'A', ' ', 'B', 'C'} // D falls off the bottom margin
	if got != want {
		t.Fatalf("IL: rows=%v, want %v", got, want)
	}
}

func TestAnsiParser_CSI_DeleteLinesAtCursor(t *testing.T) {
	tv := NewTerminalView(5, 4)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.Lines[0][0].Char = 'A'
	tv.Lines[1][0].Char = 'B'
	tv.Lines[2][0].Char = 'C'
	tv.Lines[3][0].Char = 'D'
	tv.SetCursor(0, 1)

	p.Process([]byte("\x1b[2M")) // DL: delete two lines at the cursor row

	got := [4]uint64{tv.Lines[0][0].Char, tv.Lines[1][0].Char, tv.Lines[2][0].Char, tv.Lines[3][0].Char}
	want := [4]uint64{'A', 'D', ' ', ' '}
	if got != want {
		t.Fatalf("DL: rows=%v, want %v", got, want)
	}
}

func TestAnsiParser_CSI_ScrollDownWithinMargins(t *testing.T) {
	tv := NewTerminalView(5, 3)
	defer tv.Close()
	p := NewAnsiParser(tv, nil)

	tv.Lines[0][0].Char = 'A'
	tv.Lines[1][0].Char = 'B'
	tv.Lines[2][0].Char = 'C'

	p.Process([]byte("\x1b[1T")) // SD: scroll the whole screen down one line

	got := [3]uint64{tv.Lines[0][0].Char, tv.Lines[1][0].Char, tv.Lines[2][0].Char}
	want := [3]uint64{' ', 'A', 'B'} // C falls off the bottom
	if got != want {
		t.Fatalf("SD (1T): rows=%v, want %v", got, want)
	}
}

// --- Intermediate-byte and DCS state transitions ---

func TestAnsiParser_EscIntermediateAndDCSStateTransitionsReturnToGround(t *testing.T) {
	// ESC ( B (select ASCII into G0) exercises the StateEscIntermediate
	// path: an intermediate byte, then a final byte outside 0x20-0x2F.
	// A normal character right after must land on the screen, proving the
	// parser returned to StateGround.
	t.Run("charset selection intermediate byte", func(t *testing.T) {
		tv := NewTerminalView(10, 3)
		defer tv.Close()
		p := NewAnsiParser(tv, nil)
		tv.SetCursor(0, 0)

		p.Process([]byte("\x1b(BZ"))

		if tv.Lines[0][0].Char != 'Z' {
			t.Fatalf("after ESC ( B: Lines[0][0]=%s, want 'Z'", charQuote(tv.Lines[0][0].Char))
		}
	})

	// A DCS introducer aborted by a stray ESC (instead of completing with
	// a final byte) must fall back into StateEsc and, from there, resolve
	// normally on the following String Terminator.
	t.Run("DCS introducer aborted by ESC", func(t *testing.T) {
		tv := NewTerminalView(10, 3)
		defer tv.Close()
		p := NewAnsiParser(tv, nil)
		tv.SetCursor(0, 0)

		p.Process([]byte("\x1bP\x1b\\C")) // ESC P, aborted by ESC, then ST, then 'C'

		if tv.Lines[0][0].Char != 'C' {
			t.Fatalf("after aborted DCS: Lines[0][0]=%s, want 'C'", charQuote(tv.Lines[0][0].Char))
		}
	})

	// A non-sixel DCS string (final byte other than 'q') terminated by
	// BEL instead of ST must be swallowed without touching the screen,
	// and parsing must resume normally afterwards.
	t.Run("non-sixel DCS terminated by BEL", func(t *testing.T) {
		tv := NewTerminalView(10, 3)
		defer tv.Close()
		p := NewAnsiParser(tv, nil)
		tv.SetCursor(0, 0)

		p.Process([]byte("\x1bP0phello\x07B"))

		if tv.Lines[0][0].Char != 'B' {
			t.Fatalf("after BEL-terminated DCS: Lines[0][0]=%s, want 'B'", charQuote(tv.Lines[0][0].Char))
		}
	})
}
