package main

import (
	"testing"

	"github.com/unxed/vtinput"
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
		tv.Lines[row][i] = vtui.CharInfo{Char: uint64(r), Attributes: attr}
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

func TestTerminalSelection_HighlightInvertsCells(t *testing.T) {
	tv := newSelectableTV(20, 6)
	defer tv.Close()
	seedRow(tv, 0, "hello world")

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 6)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()

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
func panelsFrameWithMouseSelect(t *testing.T) (*PanelsFrame, *fakePTY) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()
	pf := NewPanelsFrame()
	pf.ResizeConsole(80, 25)
	pf.showPanels = false
	pf.termView.SetPosition(0, 0, 79, 22)
	pty := &fakePTY{}
	pf.pty = pty
	return pf, pty
}

type fakePTY struct{ writes []byte }

func (p *fakePTY) Read(b []byte) (int, error)            { return 0, nil }
func (p *fakePTY) Write(b []byte) (int, error)           { p.writes = append(p.writes, b...); return len(b), nil }
func (p *fakePTY) Close() error                          { return nil }
func (p *fakePTY) SetSize(cols, rows int)                {}
func (p *fakePTY) Wait() error                           { return nil }
func (p *fakePTY) Run(name string, args ...string) error { return nil }
func (p *fakePTY) IsBusy() bool                          { return false }

// TestPanelsFrame_TerminalMouseSelect_Drag exercises the full click →
// drag → release pipeline through PanelsFrame.ProcessMouse. Asserts
// on selection state; the release-time clipboard write happens on a
// goroutine and depends on external tools (xclip / wl-copy / far2l
// IPC), so we don't assert on GetClipboard here — that path is
// covered separately in the vtui clipboard tests.
func TestPanelsFrame_TerminalMouseSelect_Drag(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.termView
	seedRow(tv, 0, "hello world")

	// LMB down at (2, 0)
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      2, MouseY: 0,
	})
	if !tv.HasSelection() {
		t.Fatal("expected HasSelection=true after LMB down")
	}
	if !pf.termSelDragging {
		t.Fatal("expected termSelDragging=true after LMB down inside terminal area")
	}
	// Drag to (6, 0)
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
		MouseX:          6, MouseY: 0,
	})
	if got := tv.ExtractSelection(); got != "llo w" {
		t.Errorf("after drag: got %q, want %q", got, "llo w")
	}

	// Release button — drag flag drops, selection stays (xterm-style).
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: 0,
		MouseX:      6, MouseY: 0,
	})
	if pf.termSelDragging {
		t.Fatal("release should clear termSelDragging")
	}
	if !tv.HasSelection() {
		t.Fatal("release should keep the highlight (xterm-style)")
	}
	if got := tv.ExtractSelection(); got != "llo w" {
		t.Errorf("post-release extract: got %q, want %q", got, "llo w")
	}
}

// TestPanelsFrame_TerminalMouseSelect_ReleaseHostShapes locks in
// support for the four distinct release-event shapes across our
// backends. See the giant comment inside handleTerminalMouseSelection
// for the taxonomy.
func TestPanelsFrame_TerminalMouseSelect_ReleaseHostShapes(t *testing.T) {
	cases := []struct {
		name    string
		release *vtinput.InputEvent
	}{
		{
			"Wayland/Windows-console: ButtonState=0, KeyDown=false",
			&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: false,
				ButtonState: 0,
				MouseX:      6, MouseY: 0,
			},
		},
		{
			"Windows-console: ButtonState=0, KeyDown=true (release inferred by ButtonState==0)",
			&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: true,
				ButtonState: 0,
				MouseX:      6, MouseY: 0,
			},
		},
		{
			"X11/purex11/tty-SGR: ButtonState=LMB left in place, KeyDown=false",
			&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: false,
				ButtonState: vtinput.FromLeft1stButtonPressed,
				MouseX:      6, MouseY: 0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := panelsFrameWithMouseSelect(t)
			tv := pf.termView
			seedRow(tv, 0, "hello world")

			pf.ProcessMouse(&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: true,
				ButtonState: vtinput.FromLeft1stButtonPressed,
				MouseX:      2, MouseY: 0,
			})
			pf.ProcessMouse(&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: true,
				ButtonState:     vtinput.FromLeft1stButtonPressed,
				MouseEventFlags: vtinput.MouseMoved,
				MouseX:          6, MouseY: 0,
			})
			pf.ProcessMouse(tc.release)
			if pf.termSelDragging {
				t.Fatalf("release didn't clear termSelDragging: %+v", tc.release)
			}
			if got := tv.ExtractSelection(); got != "llo w" {
				t.Fatalf("selection lost through release: got %q, want %q", got, "llo w")
			}
		})
	}
}

// TestPanelsFrame_TerminalMouseSelect_DragHostShapes locks in support
// for the two motion-event shapes we've seen: Wayland-style with the
// button code still in ButtonState, and X11-style where motion carries
// only the MouseMoved flag.
func TestPanelsFrame_TerminalMouseSelect_DragHostShapes(t *testing.T) {
	cases := []struct {
		name string
		drag *vtinput.InputEvent
	}{
		{
			"Wayland: ButtonState=LMB, KeyDown=false, MouseMoved",
			&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: false,
				ButtonState:     vtinput.FromLeft1stButtonPressed,
				MouseEventFlags: vtinput.MouseMoved,
				MouseX:          6, MouseY: 0,
			},
		},
		{
			"X11: ButtonState=0, KeyDown=false, MouseMoved",
			&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: false,
				ButtonState:     0,
				MouseEventFlags: vtinput.MouseMoved,
				MouseX:          6, MouseY: 0,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pf, _ := panelsFrameWithMouseSelect(t)
			tv := pf.termView
			seedRow(tv, 0, "hello world")

			pf.ProcessMouse(&vtinput.InputEvent{
				Type: vtinput.MouseEventType, KeyDown: true,
				ButtonState: vtinput.FromLeft1stButtonPressed,
				MouseX:      2, MouseY: 0,
			})
			pf.ProcessMouse(tc.drag)
			if got := tv.ExtractSelection(); got != "llo w" {
				t.Fatalf("drag didn't extend: got %q, want %q", got, "llo w")
			}
		})
	}
}

func TestPanelsFrame_TerminalMouseSelect_RightClickPastes(t *testing.T) {
	pf, pty := panelsFrameWithMouseSelect(t)
	pf.termView.clipboardReader = func() string { return "pasted" }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX:      5, MouseY: 5,
	})
	if string(pty.writes) != "pasted" {
		t.Errorf("RMB paste sent %q, want %q", string(pty.writes), "pasted")
	}
}

func TestPanelsFrame_TerminalMouseSelect_RightClickPasteBracketed(t *testing.T) {
	pf, pty := panelsFrameWithMouseSelect(t)
	pf.termView.BracketedPasteMode = true
	pf.termView.clipboardReader = func() string { return "pasted" }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX:      5, MouseY: 5,
	})
	want := "\x1b[200~pasted\x1b[201~"
	if string(pty.writes) != want {
		t.Errorf("bracketed paste sent %q, want %q", string(pty.writes), want)
	}
}
