package wincondrag

import "github.com/unxed/vtui"

// Rect is a window rectangle in screen pixels, right and bottom exclusive.
type Rect struct{ Left, Top, Right, Bottom int32 }

// Contains reports whether the screen point lies inside the rectangle.
func (r Rect) Contains(x, y int32) bool {
	return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom
}

// Empty reports whether the rectangle covers no pixel, which is what the
// pseudoconsole's helper window answers under Windows Terminal.
func (r Rect) Empty() bool { return r.Right <= r.Left || r.Bottom <= r.Top }

// Host is a window the tool window can be laid over.
type Host struct {
	Handle  uintptr
	Rect    Rect
	Visible bool
	Topmost bool
}

// PickHost chooses the window the tool window goes over: the first
// candidate that is visible and has the pointer inside it. The console
// window comes first; under Windows Terminal it is a 0x0 helper, and the
// foreground window -- the terminal itself -- is the one that counts. The
// pointer has to be inside: the synthetic press lands wherever the pointer
// is, and only a tool window under it receives that press.
func PickHost(x, y int32, candidates ...Host) (Host, bool) {
	for _, c := range candidates {
		if c.Handle == 0 || !c.Visible || c.Rect.Empty() || !c.Rect.Contains(x, y) {
			continue
		}
		return c, true
	}
	return Host{}, false
}

// OLE drop effects.
const (
	dropEffectCopy = 1
	dropEffectMove = 2
	dropEffectLink = 4
)

// ActionToEffects is what a source that allows the given actions offers OLE.
func ActionToEffects(allowed vtui.DropAction) uint32 {
	var eff uint32
	if allowed&vtui.DropCopy != 0 {
		eff |= dropEffectCopy
	}
	if allowed&vtui.DropMove != 0 {
		eff |= dropEffectMove
	}
	if allowed&vtui.DropLink != 0 {
		eff |= dropEffectLink
	}
	return eff
}

// EffectToAction is what the target reported doing. Move wins over copy:
// a target that says both did move, which is the one the source has to know.
func EffectToAction(eff uint32) vtui.DropAction {
	switch {
	case eff&dropEffectMove != 0:
		return vtui.DropMove
	case eff&dropEffectCopy != 0:
		return vtui.DropCopy
	case eff&dropEffectLink != 0:
		return vtui.DropLink
	}
	return vtui.DropNone
}

// mouse_event flags and virtual keys of the two buttons.
const (
	mouseEventMove      = 0x0001
	mouseEventLeftDown  = 0x0002
	mouseEventLeftUp    = 0x0004
	mouseEventRightDown = 0x0008
	mouseEventRightUp   = 0x0010

	vkLButton = 0x01
	vkRButton = 0x02
)

// Buttons names the physical button that carries a logical left drag.
// mouse_event and GetAsyncKeyState both speak physical buttons, and with
// the buttons swapped in the mouse settings the logical left one -- the one
// the console reported and the one WM_LBUTTONDOWN means -- is the physical
// right one.
type Buttons struct {
	VirtualKey uintptr
	Down, Up   uintptr
}

// PrimaryButtons returns the physical button for a logical left drag.
func PrimaryButtons(swapped bool) Buttons {
	if swapped {
		return Buttons{VirtualKey: vkRButton, Down: mouseEventRightDown, Up: mouseEventRightUp}
	}
	return Buttons{VirtualKey: vkLButton, Down: mouseEventLeftDown, Up: mouseEventLeftUp}
}
