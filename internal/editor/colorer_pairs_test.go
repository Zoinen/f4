package editor

import (
	"testing"

	colorer "github.com/unxed/colorer4go"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

// pairLines builds a pairsAt over lines written as text: '(' is a pair start
// and ')' a pair end, at their rune offsets. A line that is nil is unparsed.
func pairLines(lines ...*string) func(int) ([]colorer.Pair, bool) {
	return func(idx int) ([]colorer.Pair, bool) {
		if idx < 0 || idx >= len(lines) || lines[idx] == nil {
			return nil, false
		}
		var pairs []colorer.Pair
		for i, r := range []rune(*lines[idx]) {
			if r == '(' || r == ')' {
				pairs = append(pairs, colorer.Pair{Start: i, End: i + 1, Opens: r == '('})
			}
		}
		return pairs, true
	}
}

func str(s string) *string { return &s }

func TestMatchColorerPair_ForwardAcrossLines(t *testing.T) {
	at := pairLines(str("f(a, (b)"), str("  c"), str(")")) // the first '(' closes on line 2
	m, ok := matchColorerPair(0, 1, 0, 2, at)
	if !ok || !m.top || !m.found {
		t.Fatalf("got %+v, %v; want a found match below", m, ok)
	}
	if m.end.line != 2 || m.end.pair.Start != 0 {
		t.Errorf("end = line %d col %d, want line 2 col 0", m.end.line, m.end.pair.Start)
	}
}

func TestMatchColorerPair_BackwardFromAnEnd(t *testing.T) {
	at := pairLines(str("x (y"), str("(z) )"))
	m, ok := matchColorerPair(1, 4, 0, 1, at)
	if !ok || m.top || !m.found || m.end.line != 0 || m.end.pair.Start != 2 {
		t.Fatalf("got %+v, %v; want the '(' on line 0 col 2", m, ok)
	}
}

// getPairMatch includes the end offset, so the cursor right after a bracket
// finds it; with two adjacent tokens the later one wins.
func TestMatchColorerPair_CursorAfterTheToken(t *testing.T) {
	at := pairLines(str("()"))
	m, ok := matchColorerPair(0, 1, 0, 0, at)
	if !ok || m.top || m.start.pair.Start != 1 {
		t.Fatalf("got %+v, %v; want the ')' at col 1, the last token covering col 1", m, ok)
	}
	if !m.found || m.end.pair.Start != 0 {
		t.Errorf("end = %+v, want the '(' at col 0", m.end)
	}
}

func TestMatchColorerPair_StopsAtTheWindowAndUnparsedLines(t *testing.T) {
	at := pairLines(str("("), str(""), str(")"))
	if m, ok := matchColorerPair(0, 0, 0, 1, at); !ok || m.found {
		t.Errorf("window ending on line 1: got %+v, %v; want the start without a match", m, ok)
	}
	unparsed := pairLines(str("("), nil, str(")"))
	if m, ok := matchColorerPair(0, 0, 0, 2, unparsed); !ok || m.found {
		t.Errorf("unparsed line 1: got %+v, %v; want the start without a match", m, ok)
	}
	if _, ok := matchColorerPair(0, 3, 0, 2, at); ok {
		t.Error("a cursor on no pair token found one")
	}
}

func TestColorerPairOverlay_PaintsCopies(t *testing.T) {
	attrs := []uint64{1, 1, 1}
	overlay := colorerPairOverlay{0: {{Start: 1, End: 2, Opens: true, Back: 0x123456, IsBackSet: true}}}
	out := overlay.apply(0, attrs)
	if attrs[1] != 1 {
		t.Fatal("the cached attributes were modified")
	}
	if out[1] == attrs[1] || out[0] != attrs[0] || out[2] != attrs[2] {
		t.Errorf("out = %v; want only rune 1 repainted", out)
	}
	if got := overlay.apply(1, attrs); &got[0] != &attrs[0] {
		t.Error("a line without tokens was copied")
	}
}

// A walk that meets an unparsed line stops there without moving, and carries
// on from the same place once the line is there.
func TestColorerPairWalk_ResumesAtTheMissingLine(t *testing.T) {
	lines := []*string{str("("), nil, str("())")}
	at := pairLines(lines...)
	pairs, _ := at(0)
	w, ok := startColorerPairWalk(0, 0, pairs)
	if !ok {
		t.Fatal("no pair token under the cursor")
	}
	if stop, need := w.advance(0, 2, at); stop != colorerWalkNeedsLine || need != 1 {
		t.Fatalf("advance = %v, %d; want to need line 1", stop, need)
	}
	if stop, need := w.advance(0, 2, at); stop != colorerWalkNeedsLine || need != 1 {
		t.Fatalf("second advance = %v, %d; want to still need line 1", stop, need)
	}
	lines[1] = str("")
	if stop, _ := w.advance(0, 2, pairLines(lines...)); stop != colorerWalkFound {
		t.Fatalf("advance after parsing = %v, want found", stop)
	}
	if w.match.end.line != 2 || w.match.end.pair.Start != 2 {
		t.Errorf("end = line %d col %d, want line 2 col 2", w.match.end.line, w.match.end.pair.Start)
	}
}

func TestRuneByteIndexes(t *testing.T) {
	text := "aЖ\U0001F600b"
	for r, b := range []int{0, 1, 3, 7, 8} {
		if got := byteIndexAtRune(text, r); got != b {
			t.Errorf("byteIndexAtRune(%d) = %d, want %d", r, got, b)
		}
		if got := runeIndexAtByte(text, b); got != r {
			t.Errorf("runeIndexAtByte(%d) = %d, want %d", b, got, r)
		}
	}
	if byteIndexAtRune(text, 99) != len(text) || runeIndexAtByte(text, 99) != 4 {
		t.Error("offsets past the end are not clamped")
	}
}

// FarColorer's positions, on a two-line text: "f(Ж," and "x)".
func TestApplyColorerPair_Positions(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	text := "f(\u0416,\nx)"
	match := colorerPairMatch{
		start: colorerPairToken{0, colorer.Pair{Start: 1, End: 2, Opens: true}},
		end:   colorerPairToken{1, colorer.Pair{Start: 1, End: 2}},
		found: true,
		top:   true,
	}
	offsetOf := func(ev *EditorView) int { return ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos }

	ev := NewEditorView(piecetable.New([]byte(text)), nil, "test.c")
	defer ev.Close()
	ev.applyColorerPair(ColorerMatchPair, match)
	if ev.CursorLine != 1 || ev.CursorPos != 1 || ev.SelActive {
		t.Errorf("match pair: cursor %d:%d sel %v, want 1:1 without a selection", ev.CursorLine, ev.CursorPos, ev.SelActive)
	}

	ev.applyColorerPair(ColorerSelectPair, match)
	// From after "(" (byte 2) to before ")" on line 1 (byte offset 6+1).
	if !ev.SelActive || ev.SelAnchorOffset != 2 || offsetOf(ev) != 7 {
		t.Errorf("select pair: anchor %d cursor %d, want 2 and 7", ev.SelAnchorOffset, offsetOf(ev))
	}

	ev.applyColorerPair(ColorerSelectBlock, match)
	if !ev.SelActive || ev.SelAnchorOffset != 1 || offsetOf(ev) != 8 {
		t.Errorf("select block: anchor %d cursor %d, want 1 and 8", ev.SelAnchorOffset, offsetOf(ev))
	}

	// The same pair seen from its end: the positions do not change.
	back := colorerPairMatch{start: match.end, end: match.start, found: true}
	ev.applyColorerPair(ColorerSelectBlock, back)
	if ev.SelAnchorOffset != 1 || offsetOf(ev) != 8 {
		t.Errorf("select block from the end: anchor %d cursor %d, want 1 and 8", ev.SelAnchorOffset, offsetOf(ev))
	}
	ev.applyColorerPair(ColorerMatchPair, back)
	if ev.CursorLine != 0 || ev.CursorPos != 1 {
		t.Errorf("match pair from the end: cursor %d:%d, want 0:1", ev.CursorLine, ev.CursorPos)
	}
}
