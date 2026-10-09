package dialog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newTestHotkeyAssignFrame(hm *keymap.HotkeyManager, onComplete func()) *HotkeyAssignFrame {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	return NewHotkeyAssignFrame(hm, "Test.Action", "Shell", onComplete)
}

// TestNewHotkeyAssignFrame_CurrentBindingLabel covers the "Current: ..."
// label built in the constructor: "(none)" when the action has no binding
// yet, and the formatted key once one exists.
func TestNewHotkeyAssignFrame_CurrentBindingLabel(t *testing.T) {
	t.Run("no existing binding shows none", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		f := newTestHotkeyAssignFrame(hm, nil)

		want := fmt.Sprintf(i18n.Msg("Hotkeys.AssignCurrent"), i18n.Msg("Hotkeys.AssignNone"))
		if texts := dialogTexts(t, f); !containsText(texts, want) {
			t.Errorf("dialog lacks the %q label; texts:\n%s", want, strings.Join(texts, "\n"))
		}
	})

	t.Run("existing binding shows the formatted key", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		hm.Bind("Shell", "CtrlA", "Test.Action")
		f := newTestHotkeyAssignFrame(hm, nil)

		want := fmt.Sprintf(i18n.Msg("Hotkeys.AssignCurrent"), keymap.FormatKeyForUI("CtrlA"))
		if texts := dialogTexts(t, f); !containsText(texts, want) {
			t.Errorf("dialog lacks the %q label; texts:\n%s", want, strings.Join(texts, "\n"))
		}
	})
}

func TestHotkeyAssignFrame_EscapeClosesWithoutBinding(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	f := newTestHotkeyAssignFrame(hm, nil)

	handled := f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})

	if !handled {
		t.Error("ProcessKey(Escape) = false, want true")
	}
	if !f.IsDone() {
		t.Error("Escape did not close the frame")
	}
	if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != "" {
		t.Errorf("Escape must not create a binding, got %q", key)
	}
}

func TestHotkeyAssignFrame_ModifierOnlyKeysAreIgnored(t *testing.T) {
	codes := []struct {
		name string
		code uint16
	}{
		{"VK_CONTROL", vtinput.VK_CONTROL},
		{"VK_LCONTROL", vtinput.VK_LCONTROL},
		{"VK_RCONTROL", vtinput.VK_RCONTROL},
		{"VK_MENU", vtinput.VK_MENU},
		{"VK_LMENU", vtinput.VK_LMENU},
		{"VK_RMENU", vtinput.VK_RMENU},
		{"VK_CAPITAL", vtinput.VK_CAPITAL},
		{"VK_NUMLOCK", vtinput.VK_NUMLOCK},
		{"VK_SCROLL", vtinput.VK_SCROLL},
	}
	for _, tc := range codes {
		t.Run(tc.name, func(t *testing.T) {
			hm := keymap.NewHotkeyManager("")
			f := newTestHotkeyAssignFrame(hm, nil)

			handled := f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: tc.code})

			if !handled {
				t.Errorf("ProcessKey(%s) = false, want true (swallowed, not passed through)", tc.name)
			}
			if f.IsDone() {
				t.Errorf("%s must not close the assignment dialog", tc.name)
			}
			if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != "" {
				t.Errorf("%s must not create a binding, got %q", tc.name, key)
			}
		})
	}
}

func TestHotkeyAssignFrame_KeyUpIsIgnored(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	f := newTestHotkeyAssignFrame(hm, nil)

	handled := f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_A})

	if handled {
		t.Error("ProcessKey(key-up) = true, want false")
	}
	if f.IsDone() {
		t.Error("a key-up event must not close the frame")
	}
}

func TestHotkeyAssignFrame_PlainKeyBindsAndCloses(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	var completed bool
	f := newTestHotkeyAssignFrame(hm, func() { completed = true })

	handled := f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A, Char: 'a'})

	if !handled {
		t.Error("ProcessKey(A) = false, want true")
	}
	if !f.IsDone() {
		t.Error("a plain key press must close the frame")
	}
	if !completed {
		t.Error("onComplete was not called")
	}
	area, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action")
	if area != "Shell" || key == "" {
		t.Errorf("ConfiguredHotkeyBinding = (%q, %q), want area Shell and a non-empty key", area, key)
	}
}

func TestHotkeyAssignFrame_NilOnCompleteDoesNotPanic(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	f := newTestHotkeyAssignFrame(hm, nil)

	if !f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A}) {
		t.Error("ProcessKey(A) = false, want true")
	}
}

func TestHotkeyAssignFrame_NilHotkeyManagerDoesNotPanic(t *testing.T) {
	f := newTestHotkeyAssignFrame(nil, nil)

	if !f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A}) {
		t.Error("ProcessKey(A) = false, want true")
	}
	if !f.IsDone() {
		t.Error("a plain key press must close the frame even without a HotkeyManager")
	}
}

// TestHotkeyAssignFrame_ShiftHeldCarriesOverToNextEvent exercises the
// documented quirk (see the comment on ProcessKey): some GUI hosts report
// the Shift key-down separately and then omit ShiftPressed from the
// following key event, so the frame must remember it was held.
func TestHotkeyAssignFrame_ShiftHeldCarriesOverToNextEvent(t *testing.T) {
	plainA := func() string {
		return keymap.EventToHotkeyString(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A})
	}
	shiftedA := func() string {
		return keymap.EventToHotkeyString(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A,
			ControlKeyState: vtinput.ShiftPressed,
		})
	}
	if plainA() == shiftedA() {
		t.Fatal("test setup is broken: plain A and Shift+A must have different key strings")
	}

	t.Run("shift key-down then a plain-looking A is bound as Shift+A", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		f := newTestHotkeyAssignFrame(hm, nil)

		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LSHIFT})
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A})

		if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != shiftedA() {
			t.Errorf("bound key = %q, want %q (Shift+A)", key, shiftedA())
		}
	})

	t.Run("without a prior shift key-down, A is bound unshifted", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		f := newTestHotkeyAssignFrame(hm, nil)

		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A})

		if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != plainA() {
			t.Errorf("bound key = %q, want %q (plain A)", key, plainA())
		}
	})

	t.Run("releasing shift before the letter clears the held state", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		f := newTestHotkeyAssignFrame(hm, nil)

		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LSHIFT})
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_LSHIFT})
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A})

		if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != plainA() {
			t.Errorf("bound key = %q, want %q (shift was released before A)", key, plainA())
		}
	})

	t.Run("losing focus clears the held state", func(t *testing.T) {
		hm := keymap.NewHotkeyManager("")
		f := newTestHotkeyAssignFrame(hm, nil)

		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LSHIFT})
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_A})

		if _, key := keymap.ConfiguredHotkeyBinding(hm, "Test.Action"); key != plainA() {
			t.Errorf("bound key = %q, want %q (focus loss must clear the held Shift)", key, plainA())
		}
	})
}
