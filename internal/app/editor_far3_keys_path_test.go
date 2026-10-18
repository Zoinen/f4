package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The FAR3 editor keys of f4#1729 have to work when they are pressed, not only
// resolve in the hotkey table: the key goes through the same filter as a real
// key press (macroFilter), then to the editor.
func TestEditorFAR3KeysWorkThroughTheKeyPath(t *testing.T) {
	oldHotkeys := keymap.GlobalHotkeysMgr
	oldMacro := macro.MacroMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	keymap.GlobalHotkeysMgr.InitDefaults()
	macro.MacroMgr = &macro.MacroManager{}
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = oldHotkeys
		macro.MacroMgr = oldMacro
	})

	press := func(ev *editor.EditorView, vk uint16, char rune) {
		e := &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: char,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}
		if !macroFilter(macro.MacroMgr, e) {
			ev.ProcessKey(e)
		}
	}
	open := func(text string) *editor.EditorView {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
		ev := editor.NewEditorView(piecetable.New([]byte(text)), nil, "")
		t.Cleanup(func() { ev.Close() })
		ev.SetPosition(0, 0, 80, 24)
		vtui.FrameManager.Push(ev)
		return ev
	}
	text := func(ev *editor.EditorView) string {
		data, _ := ev.Pt.Bytes()
		return string(data)
	}

	// Ctrl+K deletes to the end of the line.
	ev := open("hello world\nnext\n")
	ev.CursorPos = 5
	press(ev, vtinput.VK_K, 0x0b)
	if got := text(ev); got != "hello\nnext\n" {
		t.Errorf("Ctrl+K left %q, want %q", got, "hello\nnext\n")
	}

	// Ctrl+Backspace deletes the word before the cursor.
	ev = open("hello world\n")
	ev.CursorPos = 11
	press(ev, vtinput.VK_BACK, 0x7f)
	// The binding is spelled CtrlBS, the name the key table gives Backspace;
	// "CtrlBack" never matched a key press. How much space goes with the
	// word is DeleteWordBackward's business, not this test's.
	if got := text(ev); !strings.HasPrefix(got, "hello") || strings.Contains(got, "world") {
		t.Errorf("Ctrl+Backspace left %q, want the word before the cursor gone", got)
	}

	// Ctrl+Delete is an action so it appears in generated help. Its handler
	// preserves the editor's line-join behavior at the end of a line.
	if got, want := keymap.GlobalHotkeysMgr.GetAction("Editor", "CtrlDel"), "Editor.DeleteWordForward"; got != want {
		t.Fatalf("Ctrl+Delete resolves to %q, want %q", got, want)
	}
	ev = open("say hello world\n")
	ev.CursorPos = len("say ")
	press(ev, vtinput.VK_DELETE, 0)
	if got := text(ev); got != "say world\n" {
		t.Errorf("Ctrl+Delete left %q, want %q", got, "say world\n")
	}

	// At the end of a line Ctrl+Delete is plain Delete and joins the lines.
	ev = open("ab\ncd\n")
	ev.CursorPos = 2
	press(ev, vtinput.VK_DELETE, 0)
	if got := text(ev); got != "abcd\n" {
		t.Errorf("Ctrl+Delete at end of line left %q, want %q", got, "abcd\n")
	}

	// A selection takes precedence over word deletion.
	ev = open("say hello world\n")
	ev.SelActive = true
	ev.SelAnchorOffset = len("say ")
	ev.CursorPos = len("say hello")
	press(ev, vtinput.VK_DELETE, 0)
	if got := text(ev); got != "say  world\n" {
		t.Errorf("Ctrl+Delete with selection left %q, want %q", got, "say  world\n")
	}
}

// Ctrl+Backspace at the start of a line and Ctrl+Delete at the end of one are
// plain Backspace/Delete in far2l and join the lines. The word-delete actions
// do nothing there, so the key has to get past the hotkey dispatcher to the
// editor's own handling (f4#1729).
func TestEditorFAR3WordDeleteJoinsLinesAtLineEdges(t *testing.T) {
	oldHotkeys := keymap.GlobalHotkeysMgr
	oldMacro := macro.MacroMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	keymap.GlobalHotkeysMgr.InitDefaults()
	macro.MacroMgr = &macro.MacroManager{}
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = oldHotkeys
		macro.MacroMgr = oldMacro
	})

	press := func(ev *editor.EditorView, vk uint16, char rune) {
		e := &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: char,
			ControlKeyState: vtinput.LeftCtrlPressed,
		}
		if !macroFilter(macro.MacroMgr, e) {
			ev.ProcessKey(e)
		}
	}
	open := func(text string) *editor.EditorView {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
		ev := editor.NewEditorView(piecetable.New([]byte(text)), nil, "")
		t.Cleanup(func() { ev.Close() })
		ev.SetPosition(0, 0, 80, 24)
		vtui.FrameManager.Push(ev)
		return ev
	}
	text := func(ev *editor.EditorView) string {
		data, _ := ev.Pt.Bytes()
		return string(data)
	}

	ev := open("ab\ncd\n")
	ev.CursorLine = 1
	ev.CursorPos = 0
	press(ev, vtinput.VK_BACK, 0x7f)
	if got := text(ev); got != "abcd\n" {
		t.Errorf("Ctrl+Backspace at the start of a line left %q, want %q", got, "abcd\n")
	}

	ev = open("ab\ncd\n")
	ev.CursorLine = 0
	ev.CursorPos = 2
	press(ev, vtinput.VK_DELETE, 0)
	if got := text(ev); got != "abcd\n" {
		t.Errorf("Ctrl+Delete at the end of a line left %q, want %q", got, "abcd\n")
	}
}
