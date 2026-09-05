package main

import (
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// shiftDelEvents returns one Shift+Del event per input backend f4 supports.
// They differ in exactly one bit — whether Delete carries EnhancedKey — and
// that bit used to decide between "ShiftDel" and "ShiftNumDel", of which only
// the first was bound. The key must cut regardless of which backend produced it.
func shiftDelEvents(t *testing.T) map[string]*vtinput.InputEvent {
	t.Helper()

	// far2l terminal extensions: far2l adds ENHANCED_KEY for the navigation
	// cluster (TTYBackend.cpp, IsEnhancedKey) and forwards the record verbatim.
	far2l := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_DELETE,
		ControlKeyState: vtinput.ShiftPressed | vtinput.EnhancedKey,
	}

	// GUI hosts (ebiten, gogpu, x11, wayland) synthesise events with plain
	// modifier state and no EnhancedKey.
	gui := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_DELETE,
		ControlKeyState: vtinput.ShiftPressed,
	}

	// Plain terminal: CSI 3;2~ carries no enhanced flag either.
	csi, _, err := vtinput.ParseKitty([]byte("\x1b[3;2~"))
	if err != nil || csi == nil {
		t.Fatalf("cannot parse CSI 3;2~: %v", err)
	}

	return map[string]*vtinput.InputEvent{"far2l": far2l, "gui": gui, "csi": csi}
}

func TestEditorShiftDelCutsOnEveryBackend(t *testing.T) {
	for name, e := range shiftDelEvents(t) {
		t.Run(name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			vtui.SetClipboard("")

			pt := piecetable.New([]byte("hello world"))
			ev := NewEditorView(pt, nil, "test.txt")
			defer ev.Close()
			ev.SetPosition(0, 0, 80, 10)

			ev.selActive = true
			ev.selAnchorOffset = 0
			ev.CursorLine, ev.CursorPos = 0, 5

			if !ev.ProcessKey(e) {
				t.Fatal("Shift+Del was not handled by the editor")
			}

			data, _ := ev.pt.GetRange(0, ev.pt.Size())
			if got := string(data); got != " world" {
				t.Errorf("buffer = %q, want %q", got, " world")
			}
			if got := vtui.GetClipboard(); got != "hello" {
				t.Errorf("clipboard = %q, want %q: selection deleted without a copy", got, "hello")
			}
		})
	}
}

// Without a block far2l's Shift+Del does nothing at all. It must never behave
// like a plain Del and eat the character under the cursor.
func TestEditorShiftDelWithoutSelectionKeepsText(t *testing.T) {
	for name, e := range shiftDelEvents(t) {
		t.Run(name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

			pt := piecetable.New([]byte("hello world"))
			ev := NewEditorView(pt, nil, "test.txt")
			defer ev.Close()
			ev.SetPosition(0, 0, 80, 10)
			ev.CursorLine, ev.CursorPos = 0, 0

			ev.ProcessKey(e)

			data, _ := ev.pt.GetRange(0, ev.pt.Size())
			if got := string(data); got != "hello world" {
				t.Errorf("buffer = %q, want it untouched", got)
			}
		})
	}
}

// Every name the backends can produce for a Del key must resolve to the same
// action as its counterpart, so a rebound key keeps working too.
func TestHotkeyDelAliasesResolve(t *testing.T) {
	hm := NewHotkeyManager("")

	cases := []struct{ area, key, want string }{
		{"Editor", "ShiftDel", "Editor.Cut"},
		{"Editor", "ShiftNumDel", "Editor.Cut"},
		{"Editor", "CtrlDel", "Editor.DeleteSpacersForward"},
		{"Editor", "CtrlNumDel", "Editor.DeleteSpacersForward"},
	}
	for _, c := range cases {
		if got := hm.GetAction(c.area, c.key); got != c.want {
			t.Errorf("GetAction(%q, %q) = %q, want %q", c.area, c.key, got, c.want)
		}
	}

	// An explicit binding still wins over the alias.
	hm.Bind("Editor", "ShiftNumDel", "Editor.Copy")
	if got := hm.GetAction("Editor", "ShiftNumDel"); got != "Editor.Copy" {
		t.Errorf("explicit binding overridden by alias: got %q", got)
	}
}
