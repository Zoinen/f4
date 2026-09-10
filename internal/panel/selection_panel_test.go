package panel

import (
	"github.com/unxed/f4/internal/keymap"
	"testing"

	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"time"
)

// internal/terminal keeps its own copy.
type fakePTY struct {
	writes []byte
	busy   bool // reported by IsBusy; a busy terminal.PTY receives raw keys
}

func (p *fakePTY) Read(b []byte) (int, error)            { return 0, nil }
func (p *fakePTY) Write(b []byte) (int, error)           { p.writes = append(p.writes, b...); return len(b), nil }
func (p *fakePTY) Close() error                          { return nil }
func (p *fakePTY) SetSize(cols, rows int)                {}
func (p *fakePTY) Wait() error                           { return nil }
func (p *fakePTY) Run(name string, args ...string) error { return nil }
func (p *fakePTY) IsBusy() bool                          { return p.busy }

// The terminal package keeps its own; a mock is not worth sharing.

// seedRow writes a plain ASCII string into tv.Lines[row] starting at
// column 0. Preserves existing right-side padding.
func seedRow(tv *terminal.TerminalView, row int, text string) {
	attr := terminal.DefaultTermAttr
	for i, r := range text {
		if i >= tv.Width {
			return
		}
		tv.Lines[row][i] = vtui.CharInfo{Char: testutil.Uint64Rune(r), Attributes: attr}
	}
}

func panelsFrameWithMouseSelect(t *testing.T) (*PanelsFrame, *fakePTY) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	pf := NewPanelsFrame()
	pf.ResizeConsole(80, 25)
	waitForLoad(t, pf.Panels[0].(*FileSystemPanel))
	waitForLoad(t, pf.Panels[1].(*FileSystemPanel))
	pf.ShowPanels = false
	pf.TermView.SetPosition(0, 0, 79, 22)
	pf.TermView.ClipboardWriter = func(string) {}
	// zoin-bot: these tests exercise the terminal/PTy mouse path. A visible
	// f4 command line has its own paste path and is covered separately below.
	pf.CmdLine.SetVisible(false)
	pty := &fakePTY{}
	pf.Pty = pty
	return pf, pty
}

func TestPanelsFrame_TerminalMouseSelect_Drag(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
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

func TestPanelsFrame_TerminalMouseSelect_KeyDownClearsHighlight(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
	tv.StartSelection(2, 0, false)
	tv.ExtendSelection(6, 0)

	pf.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    'x',
	})
	if tv.HasSelection() {
		t.Fatal("a key-down should clear the terminal mouse highlight")
	}
}

func TestPanelsFrame_TerminalMouseSelect_EscapeDismissesWithoutPTY(t *testing.T) {
	pf, Pty := panelsFrameWithMouseSelect(t)
	Pty.busy = true // raw keys go straight to the terminal.PTY, as with a running command
	tv := pf.TermView
	tv.StartSelection(2, 0, false)
	tv.ExtendSelection(6, 0)

	esc := func(down bool) *vtinput.InputEvent {
		return &vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        down,
			VirtualKeyCode: vtinput.VK_ESCAPE,
			Char:           0x1b,
		}
	}

	if !pf.ProcessKey(esc(true)) {
		t.Fatal("Esc on a highlighted selection should be consumed")
	}
	if tv.HasSelection() {
		t.Fatal("Esc should clear the terminal mouse highlight")
	}
	if !pf.termSelEscHeld {
		t.Fatal("the dismissing Esc press should arm key-up swallowing")
	}
	if len(Pty.writes) != 0 {
		t.Fatalf("Esc press must not reach the term.PTY, got %q", Pty.writes)
	}

	if !pf.ProcessKey(esc(false)) {
		t.Fatal("the release of the dismissing Esc should be consumed")
	}
	if pf.termSelEscHeld {
		t.Fatal("key-up swallowing should disarm after one release")
	}
	if len(Pty.writes) != 0 {
		t.Fatalf("Esc release must not reach the term.PTY, got %q", Pty.writes)
	}

	// With no highlight left, Esc goes to the shell as before.
	pf.ProcessKey(esc(true))
	if len(Pty.writes) == 0 {
		t.Fatal("Esc without a selection should still be forwarded to the term.PTY")
	}
}

func TestPanelsFrame_TerminalMouseSelect_EscapeWinsOverEscToggle(t *testing.T) {
	pf, pty := panelsFrameWithMouseSelect(t)
	pty.busy = false // idle terminal: EscToggle would otherwise be disabled
	oldHotkeys, oldMacros := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	macro.MacroMgr = macro.NewMacroManager("")
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr, macro.MacroMgr = oldHotkeys, oldMacros
	})

	// Publish the frame geometry before establishing the terminal selection.
	vtui.FrameManager.Push(pf)

	tv := pf.TermView
	tv.StartSelection(2, 0, false)
	tv.ExtendSelection(6, 0)

	esc := func(down bool) *vtinput.InputEvent {
		return &vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        down,
			VirtualKeyCode: vtinput.VK_ESCAPE,
			Char:           0x1b,
		}
	}

	if !pressKey(pf, esc(true)) {
		t.Fatal("Esc on a selected idle terminal should be consumed")
	}
	if pf.ShowPanels {
		t.Fatal("Esc on a terminal selection must not toggle panels")
	}
	if tv.HasSelection() {
		t.Fatal("Esc should clear the terminal selection through the hotkey path")
	}
	if len(pty.writes) != 0 {
		t.Fatalf("Esc press must not reach the term.PTY, got %q", pty.writes)
	}

	if !pressKey(pf, esc(false)) {
		t.Fatal("release of the dismissing Esc should be consumed")
	}
	if pf.ShowPanels || len(pty.writes) != 0 {
		t.Fatalf("Esc release changed terminal state: showPanels=%v writes=%q", pf.ShowPanels, pty.writes)
	}
}

func TestPanelsFrame_TerminalMouseSelect_ModifiedEscapeNotSwallowed(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
	tv.StartSelection(2, 0, false)
	tv.ExtendSelection(6, 0)

	pf.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_ESCAPE,
		Char:            0x1b,
		ControlKeyState: vtinput.LeftAltPressed,
	})
	if tv.HasSelection() {
		t.Fatal("Alt+Esc should clear the highlight like any other key")
	}
	if pf.termSelEscHeld {
		t.Fatal("Alt+Esc must not arm key-up swallowing")
	}
}

func TestPanelsFrame_TerminalMouseSelect_AltShiftBlockCopies(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
	seedRow(tv, 0, "abcdefghij")
	seedRow(tv, 1, "0123456789")
	seedRow(tv, 2, "ABCDEFGHIJ")

	copied := make(chan string, 1)
	tv.ClipboardWriter = func(text string) { copied <- text }
	mods := vtinput.LeftAltPressed | vtinput.ShiftPressed
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      2, MouseY: 0, ControlKeyState: mods,
	})
	if !tv.SelBlock {
		t.Fatal("Alt+Shift+LMB should start a rectangular terminal selection")
	}

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
		MouseX:          4, MouseY: 2, ControlKeyState: mods,
	})
	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		MouseX: 4, MouseY: 2,
	})

	select {
	case got := <-copied:
		if want := "cde\n234\nCDE"; got != want {
			t.Errorf("terminal Alt+Shift block copy = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal rectangular selection was not copied on release")
	}
	if !tv.HasSelection() {
		t.Fatal("terminal selection should remain highlighted after release")
	}
}

func TestPanelsFrame_TerminalMouseSelect_ShiftDoubleClickCopiesWord(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
	seedRow(tv, 0, "foo bar baz")

	// Model the second press of a Shift+double-click. The frame's click
	// counter has already seen the first press, while the terminal backend
	// reports the modifier and double-click flag on this press.
	pf.termSelClickN = 1
	pf.termSelClickAt = time.Now()
	pf.termSelClickX, pf.termSelClickY = 5, 0
	copied := make(chan string, 1)
	tv.ClipboardWriter = func(text string) { copied <- text }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		ControlKeyState: vtinput.ShiftPressed,
		MouseEventFlags: vtinput.DoubleClick,
		MouseX:          5, MouseY: 0,
	})
	if !pf.termSelDragging {
		t.Fatal("double-click selection must remain active until button release")
	}

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: 0, ControlKeyState: vtinput.ShiftPressed,
		MouseX: 5, MouseY: 0,
	})

	select {
	case got := <-copied:
		if got != "bar" {
			t.Fatalf("copied word = %q, want %q", got, "bar")
		}
	case <-time.After(time.Second):
		t.Fatal("Shift+double-click did not copy the selected word")
	}
}

func TestPanelsFrame_TerminalMouseSelect_ShiftTripleClickCopiesLine(t *testing.T) {
	pf, _ := panelsFrameWithMouseSelect(t)
	tv := pf.TermView
	seedRow(tv, 0, "foo bar baz")

	// Model the third press of a Shift+triple-click after the first two
	// presses have established the click sequence.
	pf.termSelClickN = 2
	pf.termSelClickAt = time.Now()
	pf.termSelClickX, pf.termSelClickY = 5, 0
	copied := make(chan string, 1)
	tv.ClipboardWriter = func(text string) { copied <- text }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		ControlKeyState: vtinput.ShiftPressed,
		MouseEventFlags: vtui.TripleClick,
		MouseX:          5, MouseY: 0,
	})
	if !pf.termSelDragging {
		t.Fatal("triple-click selection must remain active until button release")
	}

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: 0, ControlKeyState: vtinput.ShiftPressed,
		MouseX: 5, MouseY: 0,
	})

	select {
	case got := <-copied:
		if got != "foo bar baz" {
			t.Fatalf("copied line = %q, want %q", got, "foo bar baz")
		}
	case <-time.After(time.Second):
		t.Fatal("Shift+triple-click did not copy the selected line")
	}
}

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
			tv := pf.TermView
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
			tv := pf.TermView
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
	pf, Pty := panelsFrameWithMouseSelect(t)
	pf.TermView.ClipboardReader = func() string { return "pasted" }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX:      5, MouseY: 5,
	})
	if string(Pty.writes) != "pasted" {
		t.Errorf("RMB paste sent %q, want %q", string(Pty.writes), "pasted")
	}
}

func TestPanelsFrame_TerminalMouseSelect_RightClickPasteBracketed(t *testing.T) {
	pf, Pty := panelsFrameWithMouseSelect(t)
	pf.TermView.BracketedPasteMode = true
	pf.TermView.ClipboardReader = func() string { return "pasted" }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX:      5, MouseY: 5,
	})
	want := "\x1b[200~pasted\x1b[201~"
	if string(Pty.writes) != want {
		t.Errorf("bracketed paste sent %q, want %q", string(Pty.writes), want)
	}
}

func TestPanelsFrame_TerminalMouseSelect_RightClickPastesIntoVisibleCommandLine(t *testing.T) {
	pf, Pty := panelsFrameWithMouseSelect(t)
	pf.CmdLine.SetVisible(true)
	pf.ShellMode = terminal.ShellModeOwn
	pf.TermView.ClipboardReader = func() string { return "echo pasted" }

	pf.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX:      5, MouseY: 5,
	})

	if got := pf.CmdLine.Edit.GetText(); got != "echo pasted" {
		t.Fatalf("visible command line after RMB paste = %q, want %q", got, "echo pasted")
	}
	if len(Pty.writes) != 0 {
		t.Fatalf("visible command line RMB paste wrote directly to term.PTY: %q", string(Pty.writes))
	}
}
