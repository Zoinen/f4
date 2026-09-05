package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestForcedMouseSelectionStartsAtShiftClick(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := setupGrabberScreen(t)
	vtui.FrameManager.Push(vtui.NewDesktop())

	press := mouseEvent(1, 0, vtinput.FromLeft1stButtonPressed, false, true)
	press.ControlKeyState = vtinput.ShiftPressed
	if !handleForcedMouseSelectionEvent(press) {
		t.Fatal("Shift+left-click should start screen selection")
	}

	grabber, ok := vtui.FrameManager.GetTopFrame().(*GrabberFrame)
	if !ok {
		t.Fatalf("top frame = %T, want *GrabberFrame", vtui.FrameManager.GetTopFrame())
	}
	if !grabber.hasSnap || grabber.snapW != scr.Width() || grabber.snapH != scr.Height() {
		t.Fatalf("grabber snapshot = %v %dx%d, want initialized %dx%d",
			grabber.hasSnap, grabber.snapW, grabber.snapH, scr.Width(), scr.Height())
	}
	if grabber.anchorX != 1 || grabber.anchorY != 0 || grabber.curX != 1 || grabber.curY != 0 {
		t.Fatalf("selection anchor/current = (%d,%d)/(%d,%d), want (1,0)/(1,0)",
			grabber.anchorX, grabber.anchorY, grabber.curX, grabber.curY)
	}
	if !grabber.mouseSelecting {
		t.Fatal("initial Shift+left-click should keep mouse selection active")
	}

	grabber.ProcessMouse(mouseEvent(5, 2, vtinput.FromLeft1stButtonPressed, true, true))
	grabber.ProcessMouse(mouseEvent(5, 2, 0, false, false))
	if grabber.mouseSelecting {
		t.Fatal("mouse release should end selection")
	}
	if got := grabber.copyText(); got == "" {
		t.Fatal("Shift+mouse drag should select non-empty screen text")
	}
}

func TestForcedMouseSelectionLeavesOtherMouseGesturesAlone(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	setupGrabberScreen(t)
	vtui.FrameManager.Push(vtui.NewDesktop())

	tests := []struct {
		name  string
		mods  vtinput.ControlKeyState
		moved bool
		btn   uint32
		down  bool
	}{
		{name: "plain click", btn: vtinput.FromLeft1stButtonPressed, down: true},
		{name: "ctrl shift", mods: vtinput.LeftCtrlPressed | vtinput.ShiftPressed, btn: vtinput.FromLeft1stButtonPressed, down: true},
		{name: "alt shift", mods: vtinput.LeftAltPressed | vtinput.ShiftPressed, btn: vtinput.FromLeft1stButtonPressed, down: true},
		{name: "middle button", mods: vtinput.ShiftPressed, btn: vtinput.FromLeft2ndButtonPressed, down: true},
		{name: "motion", mods: vtinput.ShiftPressed, moved: true, btn: vtinput.FromLeft1stButtonPressed, down: true},
		{name: "release", mods: vtinput.ShiftPressed, btn: 0, down: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := mouseEvent(1, 0, tt.btn, tt.moved, tt.down)
			e.ControlKeyState = tt.mods
			if handleForcedMouseSelectionEvent(e) {
				t.Fatal("event should remain available to normal mouse dispatch")
			}
			if _, ok := vtui.FrameManager.GetTopFrame().(*GrabberFrame); ok {
				t.Fatal("unexpected grabber opened")
			}
		})
	}
}

func TestForcedMouseSelectionDoesNotNestGrabbers(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	setupGrabberScreen(t)
	vtui.FrameManager.Push(vtui.NewDesktop())

	first := mouseEvent(1, 0, vtinput.FromLeft1stButtonPressed, false, true)
	first.ControlKeyState = vtinput.ShiftPressed
	if !handleForcedMouseSelectionEvent(first) {
		t.Fatal("first Shift+left-click should start selection")
	}
	grabber := vtui.FrameManager.GetTopFrame()
	second := mouseEvent(2, 0, vtinput.FromLeft1stButtonPressed, false, true)
	second.ControlKeyState = vtinput.ShiftPressed
	if handleForcedMouseSelectionEvent(second) {
		t.Fatal("a Shift+left-click must not open a nested grabber")
	}
	if vtui.FrameManager.GetTopFrame() != grabber {
		t.Fatal("existing grabber was replaced")
	}
}
