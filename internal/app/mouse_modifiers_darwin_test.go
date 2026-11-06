//go:build darwin

package app

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestDarwinGogpuMousePressInheritsHeldPhysicalControl(t *testing.T) {
	tracker := darwinMouseModifierTracker{}
	control := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RCONTROL,
		ControlKeyState: vtinput.RightCtrlPressed,
	}
	tracker.normalize(control, "gogpu")

	click := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	}
	tracker.normalize(click, "gogpu")
	if click.ControlKeyState&vtinput.RightCtrlPressed == 0 {
		t.Fatal("GoGPU mouse press did not inherit held physical Control")
	}

	control.KeyDown = false
	control.ControlKeyState = 0
	tracker.normalize(control, "gogpu")
	nextClick := &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true}
	tracker.normalize(nextClick, "gogpu")
	if nextClick.ControlKeyState != 0 {
		t.Fatalf("released modifier leaked into next click: %#x", nextClick.ControlKeyState)
	}
}
