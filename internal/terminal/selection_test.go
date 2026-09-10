package terminal

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// seedRow writes a plain ASCII string into tv.Lines[row] starting at
// column 0. Preserves existing right-side padding.
func seedRow(tv *TerminalView, row int, text string) {
	attr := DefaultTermAttr
	for i, r := range text {
		if i >= tv.Width {
			return
		}
		tv.Lines[row][i] = vtui.CharInfo{Char: testutil.Uint64Rune(r), Attributes: attr}
	}
}

func newSelectableTV(w, h int) *TerminalView {
	tv := NewTerminalView(w, h)
	tv.SetPosition(0, 0, w-1, h-1)
	return tv
}

func TestTerminalSelection_ExtractStreamSingleRow(t *testing.T) {
	tv := newSelectableTV(40, 6)
	defer tv.Close()
	seedRow(tv, 0, "hello world")

	tv.StartSelection(0, 0, false)
	tv.ExtendSelection(4, 0)

	if got := tv.ExtractSelection(); got != "hello" {
		t.Errorf("single-row stream: got %q, want %q", got, "hello")
	}
}

func TestTerminalSelection_ExtractStreamMultiRow(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 0, "line one")
	seedRow(tv, 1, "line two")
	seedRow(tv, 2, "line three")

	// Start mid-row 0, end mid-row 2 — stream picks up first row's
	// tail, all of row 1, and row 2's head.
	tv.StartSelection(5, 0, false)
	tv.ExtendSelection(3, 2)

	got := tv.ExtractSelection()
	want := "one\nline two\nline"
	if got != want {
		t.Errorf("multi-row stream:\n got %q\nwant %q", got, want)
	}
}

func TestTerminalSelection_ExtractBlock(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 0, "abcdefghij")
	seedRow(tv, 1, "0123456789")
	seedRow(tv, 2, "ABCDEFGHIJ")

	tv.StartSelection(2, 0, true) // block
	tv.ExtendSelection(4, 2)

	got := tv.ExtractSelection()
	want := "cde\n234\nCDE"
	if got != want {
		t.Errorf("block: got %q, want %q", got, want)
	}
}

func TestTerminalSelection_SelectWordAt(t *testing.T) {
	tv := newSelectableTV(40, 6)
	defer tv.Close()
	seedRow(tv, 0, "foo bar baz")

	tv.SelectWordAt(5, 0) // click on the 'a' in "bar"
	if got := tv.ExtractSelection(); got != "bar" {
		t.Errorf("word at 'bar': got %q, want %q", got, "bar")
	}

	tv.SelectWordAt(3, 0) // click on space — must not activate
	if got := tv.ExtractSelection(); got != "bar" {
		t.Errorf("clicking whitespace should leave prior selection intact, got %q", got)
	}
}

func TestTerminalSelection_SelectLineAt(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 1, "hello    ") // padded

	tv.SelectLineAt(1)
	if got := tv.ExtractSelection(); got != "hello" {
		t.Errorf("line select trims trailing spaces: got %q, want %q", got, "hello")
	}
}

func TestTerminalSelection_ClearAndEmpty(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 0, "hello")

	if tv.HasSelection() {
		t.Fatal("fresh TerminalView shouldn't have a selection")
	}
	tv.StartSelection(0, 0, false)
	if !tv.HasSelection() {
		t.Fatal("HasSelection should report true after StartSelection")
	}
	if !tv.SelectionIsEmpty() {
		t.Fatal("SelectionIsEmpty should be true when start==end (bare click)")
	}
	tv.ExtendSelection(4, 0)
	if tv.SelectionIsEmpty() {
		t.Fatal("SelectionIsEmpty should be false after extending")
	}
	tv.ClearSelection()
	if tv.HasSelection() {
		t.Fatal("ClearSelection should drop the selection")
	}
}

func TestTerminalSelection_ClearedWhenScreenLifecycleChanges(t *testing.T) {
	tests := []struct {
		name   string
		change func(*TerminalView)
	}{
		{
			name: "full erase",
			change: func(tv *TerminalView) {
				tv.EraseDisplay(2, DefaultTermAttr)
			},
		},
		{
			name: "reset",
			change: func(tv *TerminalView) {
				tv.ResetBuffer(20, 6)
			},
		},
		{
			name: "resize",
			change: func(tv *TerminalView) {
				tv.Resize(21, 7)
			},
		},
		{
			name: "move",
			change: func(tv *TerminalView) {
				tv.SetPosition(1, 0, 20, 5)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tv := newSelectableTV(20, 6)
			defer tv.Close()
			tv.StartSelection(1, 1, false)
			tv.ExtendSelection(4, 1)

			test.change(tv)
			if tv.HasSelection() {
				t.Fatalf("selection survived %s", test.name)
			}
		})
	}

	t.Run("alternate screen", func(t *testing.T) {
		tv := newSelectableTV(20, 6)
		defer tv.Close()
		tv.StartSelection(1, 1, false)
		tv.ExtendSelection(4, 1)
		tv.SetAltScreen(true)
		if tv.HasSelection() {
			t.Fatal("selection survived entering the alternate screen")
		}

		tv.StartSelection(1, 1, false)
		tv.ExtendSelection(4, 1)
		tv.SetAltScreen(false)
		if tv.HasSelection() {
			t.Fatal("selection survived returning to the primary screen")
		}
	})
}

func TestTerminalSelection_HighlightInvertsCells(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 0, "hello world")

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 6)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	tv.SetVisible(true)
	tv.StartSelection(2, 0, false)
	tv.ExtendSelection(4, 0)
	tv.Show(scr)

	baseFG := vtui.GetRGBFore(DefaultTermAttr)
	baseBG := vtui.GetRGBBack(DefaultTermAttr)
	for x := 2; x <= 4; x++ {
		attr := scr.GetCell(x, 0).Attributes
		if vtui.GetRGBFore(attr) != baseBG || vtui.GetRGBBack(attr) != baseFG {
			t.Errorf("cell (%d,0) not inverted: fg=%06x bg=%06x, want fg=%06x bg=%06x",
				x, vtui.GetRGBFore(attr), vtui.GetRGBBack(attr), baseBG, baseFG)
		}
	}
	if attr := scr.GetCell(1, 0).Attributes; vtui.GetRGBFore(attr) == baseBG {
		t.Error("cell (1,0) outside selection should not be inverted")
	}
	if attr := scr.GetCell(5, 0).Attributes; vtui.GetRGBFore(attr) == baseBG {
		t.Error("cell (5,0) outside selection should not be inverted")
	}
}

func TestTerminalSelection_InTerminalArea(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	tv.SetPosition(10, 5, 29, 10) // move it off origin

	cases := []struct {
		x, y int
		want bool
	}{
		{10, 5, true},
		{29, 10, true},
		{9, 5, false},
		{30, 10, false},
		{20, 4, false},
		{20, 11, false},
	}
	for _, c := range cases {
		if got := tv.InTerminalArea(c.x, c.y); got != c.want {
			t.Errorf("InTerminalArea(%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

// panelsFrameWithMouseSelect returns a PanelsFrame configured for
// hidden-panels terminal-selection tests: 80x25 grid, no mouse
// tracking, no altScreen, a live termView, and a fake PTY.

// TestPanelsFrame_TerminalMouseSelect_Drag exercises the full click →
// drag → release pipeline through PanelsFrame.ProcessMouse. Asserts
// on selection state; the release-time clipboard write happens on a
// goroutine and depends on external tools (xclip / wl-copy / far2l
// IPC), so we don't assert on GetClipboard here — that path is
// covered separately in the vtui clipboard tests.
// TestPanelsFrame_TerminalMouseSelect_EscapeDismissesWithoutPTY covers
// #881: Esc on a highlighted selection only drops the highlight. Neither
// the press nor its release reaches the shell, so nothing behaves like a
// line reset. Once the highlight is gone Esc is an ordinary key again.
// TestPanelsFrame_TerminalMouseSelect_EscapeWinsOverEscToggle exercises the
// complete input path. EscToggle is a global idle-terminal binding, so the
// selection must disable that condition long enough for PanelsFrame to clear
// the highlight instead of making the panels visible (#881).
// TestPanelsFrame_TerminalMouseSelect_ModifiedEscapeNotSwallowed keeps
// Alt+Esc / Ctrl+Esc / Shift+Esc on the regular any-key path: they clear
// the highlight but are not treated as the dismiss gesture.
// TestPanelsFrame_TerminalMouseSelect_ReleaseHostShapes locks in
// support for the four distinct release-event shapes across our
// backends. See the giant comment inside handleTerminalMouseSelection
// for the taxonomy.
// TestPanelsFrame_TerminalMouseSelect_DragHostShapes locks in support
// for the two motion-event shapes we've seen: Wayland-style with the
// button code still in ButtonState, and X11-style where motion carries
// only the MouseMoved flag.
