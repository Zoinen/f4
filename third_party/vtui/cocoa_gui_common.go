package vtui

import (
	"image"
	"math"
	"unicode"
	"unicode/utf8"

	"github.com/unxed/vtinput"
)

// The pure half of the Cocoa backend: tables and arithmetic that turn what
// AppKit reports into vtinput events and cells, and the step that lays a
// frame onto the canvas Core Animation is given. Nothing here calls into the
// Objective-C runtime, so it builds, and is tested, on every platform;
// cocoa_gui_darwin.go is the part that talks to AppKit.

// NSEventModifierFlags, the device-independent half.
const (
	nsEventModifierFlagCapsLock   = 1 << 16
	nsEventModifierFlagShift      = 1 << 17
	nsEventModifierFlagControl    = 1 << 18
	nsEventModifierFlagOption     = 1 << 19
	nsEventModifierFlagCommand    = 1 << 20
	nsEventModifierFlagNumericPad = 1 << 21
	nsEventModifierFlagFunction   = 1 << 23
)

// The device-dependent bits of NSEventModifierFlags (NX_DEVICE*KEYMASK in
// IOKit's IOLLEvent.h). They are what tells the left key of a pair from the
// right one; the flags above only say that one of them is down.
const (
	nxDeviceLCtlKeyMask   = 0x00000001
	nxDeviceLShiftKeyMask = 0x00000002
	nxDeviceRShiftKeyMask = 0x00000004
	nxDeviceLCmdKeyMask   = 0x00000008
	nxDeviceRCmdKeyMask   = 0x00000010
	nxDeviceLAltKeyMask   = 0x00000020
	nxDeviceRAltKeyMask   = 0x00000040
	nxDeviceRCtlKeyMask   = 0x00002000

	nxDeviceKeyMasks = nxDeviceLCtlKeyMask | nxDeviceLShiftKeyMask | nxDeviceRShiftKeyMask |
		nxDeviceLCmdKeyMask | nxDeviceRCmdKeyMask | nxDeviceLAltKeyMask | nxDeviceRAltKeyMask |
		nxDeviceRCtlKeyMask
)

// cocoaKeyCodeToVK maps the hardware key codes of a Mac keyboard (kVK_* in
// Carbon's Events.h; they name positions, not characters, exactly like
// Windows scan codes) to Windows virtual keys. Zero is "no virtual key": the
// key still types whatever the layout gives it, as an event without one.
//
// The modifier keys are not here; see cocoaModifierKeys.
var cocoaKeyCodeToVK = [128]uint16{
	0x00: vtinput.VK_A,
	0x01: vtinput.VK_S,
	0x02: vtinput.VK_D,
	0x03: vtinput.VK_F,
	0x04: vtinput.VK_H,
	0x05: vtinput.VK_G,
	0x06: vtinput.VK_Z,
	0x07: vtinput.VK_X,
	0x08: vtinput.VK_C,
	0x09: vtinput.VK_V,
	0x0A: vtinput.VK_OEM_102, // ISO section key, left of 1 or of Z
	0x0B: vtinput.VK_B,
	0x0C: vtinput.VK_Q,
	0x0D: vtinput.VK_W,
	0x0E: vtinput.VK_E,
	0x0F: vtinput.VK_R,
	0x10: vtinput.VK_Y,
	0x11: vtinput.VK_T,
	0x12: vtinput.VK_1,
	0x13: vtinput.VK_2,
	0x14: vtinput.VK_3,
	0x15: vtinput.VK_4,
	0x16: vtinput.VK_6,
	0x17: vtinput.VK_5,
	0x18: vtinput.VK_OEM_PLUS,
	0x19: vtinput.VK_9,
	0x1A: vtinput.VK_7,
	0x1B: vtinput.VK_OEM_MINUS,
	0x1C: vtinput.VK_8,
	0x1D: vtinput.VK_0,
	0x1E: vtinput.VK_OEM_6,
	0x1F: vtinput.VK_O,
	0x20: vtinput.VK_U,
	0x21: vtinput.VK_OEM_4,
	0x22: vtinput.VK_I,
	0x23: vtinput.VK_P,
	0x24: vtinput.VK_RETURN,
	0x25: vtinput.VK_L,
	0x26: vtinput.VK_J,
	0x27: vtinput.VK_OEM_7,
	0x28: vtinput.VK_K,
	0x29: vtinput.VK_OEM_1,
	0x2A: vtinput.VK_OEM_5,
	0x2B: vtinput.VK_OEM_COMMA,
	0x2C: vtinput.VK_OEM_2,
	0x2D: vtinput.VK_N,
	0x2E: vtinput.VK_M,
	0x2F: vtinput.VK_OEM_PERIOD,
	0x30: vtinput.VK_TAB,
	0x31: vtinput.VK_SPACE,
	0x32: vtinput.VK_OEM_3,
	0x33: vtinput.VK_BACK,   // "Delete" on the Mac keyboard: it deletes backwards
	0x34: vtinput.VK_RETURN, // the Enter key of old PowerBooks
	0x35: vtinput.VK_ESCAPE,
	0x40: vtinput.VK_F17,
	0x41: vtinput.VK_DECIMAL,
	0x43: vtinput.VK_MULTIPLY,
	0x45: vtinput.VK_ADD,
	// Clear sits where a PC keypad has NumLock, and every other Mac port of
	// a PC program (SDL, GLFW) calls it that.
	0x47: vtinput.VK_NUMLOCK,
	0x4B: vtinput.VK_DIVIDE,
	0x4C: vtinput.VK_RETURN, // keypad Enter
	0x4E: vtinput.VK_SUBTRACT,
	0x4F: vtinput.VK_F18,
	0x50: vtinput.VK_F19,
	0x52: vtinput.VK_NUMPAD0,
	0x53: vtinput.VK_NUMPAD1,
	0x54: vtinput.VK_NUMPAD2,
	0x55: vtinput.VK_NUMPAD3,
	0x56: vtinput.VK_NUMPAD4,
	0x57: vtinput.VK_NUMPAD5,
	0x58: vtinput.VK_NUMPAD6,
	0x59: vtinput.VK_NUMPAD7,
	0x5A: vtinput.VK_F20,
	0x5B: vtinput.VK_NUMPAD8,
	0x5C: vtinput.VK_NUMPAD9,
	0x5F: vtinput.VK_SEPARATOR, // JIS keypad comma
	0x60: vtinput.VK_F5,
	0x61: vtinput.VK_F6,
	0x62: vtinput.VK_F7,
	0x63: vtinput.VK_F3,
	0x64: vtinput.VK_F8,
	0x65: vtinput.VK_F9,
	0x67: vtinput.VK_F11,
	0x69: vtinput.VK_F13, // Print Screen on a PC keyboard
	0x6A: vtinput.VK_F16,
	0x6B: vtinput.VK_F14, // Scroll Lock on a PC keyboard
	0x6D: vtinput.VK_F10,
	0x6E: vtinput.VK_APPS, // the context menu key of a PC keyboard
	0x6F: vtinput.VK_F12,
	0x71: vtinput.VK_F15,    // Pause on a PC keyboard
	0x72: vtinput.VK_INSERT, // Help; Insert on a PC keyboard
	0x73: vtinput.VK_HOME,
	0x74: vtinput.VK_PRIOR,
	0x75: vtinput.VK_DELETE, // forward delete
	0x76: vtinput.VK_F4,
	0x77: vtinput.VK_END,
	0x78: vtinput.VK_F2,
	0x79: vtinput.VK_NEXT,
	0x7A: vtinput.VK_F1,
	0x7B: vtinput.VK_LEFT,
	0x7C: vtinput.VK_RIGHT,
	0x7D: vtinput.VK_DOWN,
	0x7E: vtinput.VK_UP,
}

// cocoaVKForKeyCode returns the virtual key for a key code, or zero.
func cocoaVKForKeyCode(keyCode uint16) uint16 {
	if int(keyCode) < len(cocoaKeyCodeToVK) {
		return cocoaKeyCodeToVK[keyCode]
	}
	return 0
}

// cocoaModifierKey describes one modifier key as AppKit reports it in a
// flagsChanged: event: the virtual key it becomes, the device-dependent bit
// that is set while this very key is down, and the device-independent flag
// of its family.
type cocoaModifierKey struct {
	vk     uint16
	device uint64
	family uint64
}

// cocoaModifierKeys maps the key codes of the modifier keys.
//
// Command and Control fold into the two Ctrl channels the way the gogpu
// backend folds them on macOS: both Command keys are VK_LCONTROL carrying
// LeftCtrlPressed, both physical Control keys VK_RCONTROL carrying
// RightCtrlPressed. Command is the shortcut modifier on a Mac -- Cmd+C must
// reach the application as a Ctrl chord or no clipboard shortcut works --
// and keeping the physical Control key on the other channel means an
// application can still tell the two apart. The two channels never share a
// virtual key, so a release on one cannot be read as a release on the other.
var cocoaModifierKeys = map[uint16]cocoaModifierKey{
	0x37: {vtinput.VK_LCONTROL, nxDeviceLCmdKeyMask, nsEventModifierFlagCommand},
	0x36: {vtinput.VK_LCONTROL, nxDeviceRCmdKeyMask, nsEventModifierFlagCommand},
	0x3B: {vtinput.VK_RCONTROL, nxDeviceLCtlKeyMask, nsEventModifierFlagControl},
	0x3E: {vtinput.VK_RCONTROL, nxDeviceRCtlKeyMask, nsEventModifierFlagControl},
	0x38: {vtinput.VK_LSHIFT, nxDeviceLShiftKeyMask, nsEventModifierFlagShift},
	0x3C: {vtinput.VK_RSHIFT, nxDeviceRShiftKeyMask, nsEventModifierFlagShift},
	0x3A: {vtinput.VK_LMENU, nxDeviceLAltKeyMask, nsEventModifierFlagOption},
	0x3D: {vtinput.VK_RMENU, nxDeviceRAltKeyMask, nsEventModifierFlagOption},
	0x39: {vtinput.VK_CAPITAL, 0, nsEventModifierFlagCapsLock},
}

// cocoaModifierKeyDown reports whether the modifier key of a flagsChanged:
// event went down or came up. flags are the event's modifier flags, which
// AppKit reports as they are after the change.
//
// Hardware events carry the device-dependent bit of every modifier held, so
// the key's own bit answers the question even while its twin on the other
// side is held. An event that carries none of those bits -- one synthesized
// by another program -- is judged by its family flag instead.
func cocoaModifierKeyDown(key cocoaModifierKey, flags uint64) bool {
	if key.device != 0 && flags&nxDeviceKeyMasks != 0 {
		return flags&key.device != 0
	}
	return flags&key.family != 0
}

// cocoaControlKeyState turns NSEvent modifier flags into vtinput's.
//
// See cocoaModifierKeys for why Command is the left Ctrl channel and Control
// the right one. Option is Alt, right Option RightAlt when the flags say it
// is the only Option held. NumLock is reported on, always: a Mac keypad has
// no NumLock and always types digits, which is what a PC keypad does with
// NumLock on, and the applications that tell the keypad's two meanings apart
// read it from here.
func cocoaControlKeyState(flags uint64) vtinput.ControlKeyState {
	mods := vtinput.ControlKeyState(vtinput.NumLockOn)
	if flags&nsEventModifierFlagShift != 0 {
		mods |= vtinput.ShiftPressed
	}
	if flags&nsEventModifierFlagCommand != 0 {
		mods |= vtinput.LeftCtrlPressed
	}
	if flags&nsEventModifierFlagControl != 0 {
		mods |= vtinput.RightCtrlPressed
	}
	if flags&nsEventModifierFlagOption != 0 {
		if flags&nxDeviceRAltKeyMask != 0 && flags&nxDeviceLAltKeyMask == 0 {
			mods |= vtinput.RightAltPressed
		} else {
			mods |= vtinput.LeftAltPressed
		}
	}
	if flags&nsEventModifierFlagCapsLock != 0 {
		mods |= vtinput.CapsLockOn
	}
	return mods
}

// isCocoaEnhancedNavKey reports the keys that carry EnhancedKey, the
// navigation cluster between the main block and the keypad. The arrows stay
// plain, as they do in the gogpu backend: the picture viewer pans with them,
// and flagging them would send them down the directory walk instead.
func isCocoaEnhancedNavKey(vk uint16) bool {
	switch vk {
	case vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT,
		vtinput.VK_INSERT, vtinput.VK_DELETE:
		return true
	}
	return false
}

// cocoaTextRune returns the character a key's text stands for, or zero when
// it stands for none: control characters, and the private use range from
// U+F700 to U+F8FF where AppKit puts the function and navigation keys
// (NSUpArrowFunctionKey and the rest).
func cocoaTextRune(s string) rune {
	r, size := utf8.DecodeRuneInString(s)
	if size == 0 || !cocoaIsTypedRune(r) {
		return 0
	}
	return r
}

// cocoaIsTypedRune reports whether r is text rather than a key AppKit spells
// as a character.
func cocoaIsTypedRune(r rune) bool {
	if r >= 0xf700 && r <= 0xf8ff {
		return false
	}
	return r >= ' ' && !unicode.IsControl(r)
}

// cocoaCellAt converts a point in the view, in points with the origin at the
// top-left corner, to a cell. pointScale is the backing scale factor (2 on a
// Retina display), cellW and cellH the cell size in pixels. The division
// truncates toward zero, as the Win32 backend's does, so a pointer dragged
// just past the top or left edge still reads as the first row or column.
func cocoaCellAt(x, y, pointScale float64, cellW, cellH int) (int16, int16) {
	if cellW <= 0 || cellH <= 0 {
		return 0, 0
	}
	if pointScale <= 0 {
		pointScale = 1
	}
	return cocoaInt16(int(x*pointScale) / cellW), cocoaInt16(int(y*pointScale) / cellH)
}

// cocoaInt16 narrows a cell coordinate to vtinput's int16, clamping one
// that is out of range: a pointer dragged far off the window.
func cocoaInt16(v int) int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}

// cocoaGridForPixels is how many whole cells fit into a view of pixW x pixH
// pixels, never less than one of each.
func cocoaGridForPixels(pixW, pixH, cellW, cellH int) (cols, rows int) {
	cols, rows = 1, 1
	if cellW > 0 && pixW/cellW > 1 {
		cols = pixW / cellW
	}
	if cellH > 0 && pixH/cellH > 1 {
		rows = pixH / cellH
	}
	return cols, rows
}

// cocoaWheelNotches turns one scrollWheel: delta into wheel notches.
//
// A wheel mouse reports line deltas, often fractional and already
// accelerated by the system; each such event is one notch, in the direction
// of its sign, which is what the Win32 backend makes of one WM_MOUSEWHEEL.
// A trackpad reports precise deltas in points; those are accumulated in acc
// and become a notch per notchPoints, so slow scrolling still scrolls
// instead of being lost below the first notch. A reversal starts the count
// afresh. The result is the number of notches and their direction, 1 for
// scrolling towards the top (vtinput's forward) and -1 towards the bottom.
func cocoaWheelNotches(acc *float64, delta float64, precise bool, notchPoints float64) (count, dir int) {
	if !precise {
		*acc = 0
		switch {
		case delta > 0:
			return 1, 1
		case delta < 0:
			return 1, -1
		}
		return 0, 0
	}
	if notchPoints <= 0 {
		notchPoints = 1
	}
	if (*acc > 0 && delta < 0) || (*acc < 0 && delta > 0) {
		*acc = 0
	}
	*acc += delta
	for *acc >= notchPoints {
		*acc -= notchPoints
		count++
		dir = 1
	}
	for *acc <= -notchPoints {
		*acc += notchPoints
		count++
		dir = -1
	}
	return count, dir
}

// composeCocoaCanvas lays the composed frame src onto a w x h canvas whose
// rows start stride bytes apart, in the RGBX byte order the layer's bitmap
// is created with. The grid goes to the top-left corner and whatever it
// does not cover -- the partial column and row of a window that is not a
// whole number of cells, or the rest of a window that just grew and has not
// been rendered at its new size yet -- is painted black, as the Win32
// backend paints its frame margins (f4 #283). A nil src gives a black
// canvas.
func composeCocoaCanvas(dst []byte, w, h, stride int, src *image.RGBA) {
	if w <= 0 || h <= 0 || stride < w*4 || len(dst) < (h-1)*stride+w*4 {
		return
	}
	srcW, srcH := 0, 0
	if src != nil {
		srcW, srcH = src.Rect.Dx(), src.Rect.Dy()
	}
	rowBytes := w * 4
	for y := 0; y < h; y++ {
		row := dst[y*stride : y*stride+rowBytes]
		n := 0
		if y < srcH {
			n = min(w, srcW) * 4
			off := y * src.Stride
			copy(row[:n], src.Pix[off:off+n])
		}
		for i := n; i < rowBytes; i += 4 {
			row[i], row[i+1], row[i+2], row[i+3] = 0, 0, 0, 255
		}
	}
}
