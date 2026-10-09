package vfs

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestDispatchPanelKeyMatchesModifiersByIntent(t *testing.T) {
	ran := 0
	keys := []PanelKey{{VK: vtinput.VK_F8, Mods: vtinput.LeftCtrlPressed, Run: func() { ran++ }}}

	rightCtrl := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8,
		ControlKeyState: vtinput.RightCtrlPressed | vtinput.NumLockOn}
	if !DispatchPanelKey(keys, rightCtrl) || ran != 1 {
		t.Fatalf("Right Ctrl+F8 with NumLock: ran=%d, want a match on a LeftCtrl declaration", ran)
	}
	plain := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8}
	if DispatchPanelKey(keys, plain) {
		t.Fatal("plain F8 matched a Ctrl+F8 declaration")
	}
	up := *rightCtrl
	up.KeyDown = false
	if DispatchPanelKey(keys, &up) || DispatchPanelKey(keys, nil) {
		t.Fatal("a key-up or nil event matched")
	}
}

func TestDispatchPanelKeyConsumesADisabledKeyWithoutRunningIt(t *testing.T) {
	ran := 0
	keys := []PanelKey{{VK: vtinput.VK_F3, Run: func() { ran++ }, Enabled: func() bool { return false }}}
	if !DispatchPanelKey(keys, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F3}) {
		t.Fatal("a disabled declared key was not consumed")
	}
	if ran != 0 {
		t.Fatal("a disabled declared key ran")
	}
}
