package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// Dialogs read SelectPos as the confirmed choice, but browsing an open menu
// (arrows, mouse hover) moves SelectPos live. Cancelling must therefore put
// it back where the menu was opened, or an Esc'd dropdown silently commits
// whatever row the user stopped on — that is how a file manager ends up in
// Georgian (unxed/f4 language dialog, among ~30 other combo-backed settings).
func TestVMenu_CancelRestoresSelection(t *testing.T) {
	FrameManager.Init(NewSilentScreenBuf())

	m := NewVMenu("")
	for _, s := range []string{"en", "de", "ka", "ru"} {
		m.AddItem(MenuItem{Text: s})
	}
	m.SetSelectPos(0)
	m.ClearDone()

	down := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}
	m.ProcessKey(down)
	m.ProcessKey(down)
	if m.SelectPos != 2 {
		t.Fatalf("browsing should move SelectPos, got %d", m.SelectPos)
	}

	esc := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	m.ProcessKey(esc)
	if !m.IsDone() {
		t.Fatal("Esc should close the menu")
	}
	if m.SelectPos != 0 {
		t.Errorf("cancel must restore SelectPos to the value at open, got %d", m.SelectPos)
	}

	// Confirming keeps the browsed row selected.
	m.ClearDone()
	m.ProcessKey(down)
	enter := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}
	m.ProcessKey(enter)
	if !m.IsDone() || m.SelectPos != 1 {
		t.Errorf("Enter must commit the browsed row: done=%v SelectPos=%d", m.IsDone(), m.SelectPos)
	}
}
