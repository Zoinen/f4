package vtui

import (
	"bytes"
	"image"
	"testing"

	"github.com/unxed/vtinput"
)

func TestCocoaKeyCodes_LettersDigitsAndNavigation(t *testing.T) {
	// The key codes name positions on an ANSI Mac keyboard (kVK_ANSI_*).
	letters := map[uint16]uint16{
		0x00: vtinput.VK_A, 0x0B: vtinput.VK_B, 0x08: vtinput.VK_C, 0x02: vtinput.VK_D,
		0x0E: vtinput.VK_E, 0x03: vtinput.VK_F, 0x05: vtinput.VK_G, 0x04: vtinput.VK_H,
		0x22: vtinput.VK_I, 0x26: vtinput.VK_J, 0x28: vtinput.VK_K, 0x25: vtinput.VK_L,
		0x2E: vtinput.VK_M, 0x2D: vtinput.VK_N, 0x1F: vtinput.VK_O, 0x23: vtinput.VK_P,
		0x0C: vtinput.VK_Q, 0x0F: vtinput.VK_R, 0x01: vtinput.VK_S, 0x11: vtinput.VK_T,
		0x20: vtinput.VK_U, 0x09: vtinput.VK_V, 0x0D: vtinput.VK_W, 0x07: vtinput.VK_X,
		0x10: vtinput.VK_Y, 0x06: vtinput.VK_Z,
		0x1D: vtinput.VK_0, 0x12: vtinput.VK_1, 0x13: vtinput.VK_2, 0x14: vtinput.VK_3,
		0x15: vtinput.VK_4, 0x17: vtinput.VK_5, 0x16: vtinput.VK_6, 0x1A: vtinput.VK_7,
		0x1C: vtinput.VK_8, 0x19: vtinput.VK_9,
		0x24: vtinput.VK_RETURN, 0x30: vtinput.VK_TAB, 0x31: vtinput.VK_SPACE,
		0x33: vtinput.VK_BACK, 0x35: vtinput.VK_ESCAPE, 0x75: vtinput.VK_DELETE,
		0x72: vtinput.VK_INSERT, 0x73: vtinput.VK_HOME, 0x77: vtinput.VK_END,
		0x74: vtinput.VK_PRIOR, 0x79: vtinput.VK_NEXT,
		0x7B: vtinput.VK_LEFT, 0x7C: vtinput.VK_RIGHT, 0x7D: vtinput.VK_DOWN, 0x7E: vtinput.VK_UP,
		0x7A: vtinput.VK_F1, 0x78: vtinput.VK_F2, 0x63: vtinput.VK_F3, 0x76: vtinput.VK_F4,
		0x60: vtinput.VK_F5, 0x61: vtinput.VK_F6, 0x62: vtinput.VK_F7, 0x64: vtinput.VK_F8,
		0x65: vtinput.VK_F9, 0x6D: vtinput.VK_F10, 0x67: vtinput.VK_F11, 0x6F: vtinput.VK_F12,
		0x52: vtinput.VK_NUMPAD0, 0x5C: vtinput.VK_NUMPAD9, 0x4C: vtinput.VK_RETURN,
	}
	for code, want := range letters {
		if got := cocoaVKForKeyCode(code); got != want {
			t.Errorf("key code 0x%02X: VK 0x%02X, want 0x%02X", code, got, want)
		}
	}
	if got := cocoaVKForKeyCode(0x3F); got != 0 {
		t.Errorf("Fn (0x3F) has VK 0x%02X, want none", got)
	}
	if got := cocoaVKForKeyCode(0xFFFF); got != 0 {
		t.Errorf("an out of range key code has VK 0x%02X, want none", got)
	}
}

func TestCocoaKeyCodes_NoTwoKeysShareALetterOrDigit(t *testing.T) {
	seen := map[uint16]int{}
	for code, vk := range cocoaKeyCodeToVK {
		if (vk >= vtinput.VK_A && vk <= vtinput.VK_Z) || (vk >= vtinput.VK_0 && vk <= vtinput.VK_9) {
			if prev, ok := seen[vk]; ok {
				t.Errorf("key codes 0x%02X and 0x%02X both map to VK 0x%02X", prev, code, vk)
			}
			seen[vk] = code
		}
	}
	if len(seen) != 36 {
		t.Errorf("%d letters and digits mapped, want 36", len(seen))
	}
}

func TestCocoaControlKeyState_CommandAndControlAreTwoCtrlChannels(t *testing.T) {
	cases := []struct {
		name  string
		flags uint64
		want  vtinput.ControlKeyState
	}{
		{"none", 0, 0},
		{"shift", nsEventModifierFlagShift, vtinput.ShiftPressed},
		{"command", nsEventModifierFlagCommand | nxDeviceLCmdKeyMask, vtinput.LeftCtrlPressed},
		{"control", nsEventModifierFlagControl | nxDeviceLCtlKeyMask, vtinput.RightCtrlPressed},
		{"both", nsEventModifierFlagCommand | nsEventModifierFlagControl, vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed},
		{"left option", nsEventModifierFlagOption | nxDeviceLAltKeyMask, vtinput.LeftAltPressed},
		{"right option", nsEventModifierFlagOption | nxDeviceRAltKeyMask, vtinput.RightAltPressed},
		{"both options", nsEventModifierFlagOption | nxDeviceLAltKeyMask | nxDeviceRAltKeyMask, vtinput.LeftAltPressed},
		{"option, no device bits", nsEventModifierFlagOption, vtinput.LeftAltPressed},
		{"caps lock", nsEventModifierFlagCapsLock, vtinput.CapsLockOn},
	}
	for _, tc := range cases {
		got := cocoaControlKeyState(tc.flags)
		want := tc.want | vtinput.NumLockOn
		if got != want {
			t.Errorf("%s: %v, want %v", tc.name, got, want)
		}
	}
}

func TestCocoaModifierKeyDown(t *testing.T) {
	lcmd := cocoaModifierKeys[0x37]
	rcmd := cocoaModifierKeys[0x36]
	if lcmd.vk != vtinput.VK_LCONTROL || rcmd.vk != vtinput.VK_LCONTROL {
		t.Fatalf("Command keys map to VK 0x%02X and 0x%02X, want VK_LCONTROL", lcmd.vk, rcmd.vk)
	}
	if ctl := cocoaModifierKeys[0x3B]; ctl.vk != vtinput.VK_RCONTROL {
		t.Fatalf("Control maps to VK 0x%02X, want VK_RCONTROL", ctl.vk)
	}

	// Left Command released while right Command is still held: the family
	// flag stays set, the key's own device bit is gone.
	flags := uint64(nsEventModifierFlagCommand | nxDeviceRCmdKeyMask)
	if cocoaModifierKeyDown(lcmd, flags) {
		t.Error("left Command reads as down after its release")
	}
	if !cocoaModifierKeyDown(rcmd, flags) {
		t.Error("right Command reads as up while held")
	}

	// An event synthesized without device bits falls back to the family.
	if !cocoaModifierKeyDown(cocoaModifierKeys[0x38], nsEventModifierFlagShift) {
		t.Error("Shift without device bits reads as up")
	}
	if cocoaModifierKeyDown(cocoaModifierKeys[0x38], 0) {
		t.Error("Shift reads as down with no flags at all")
	}
}

func TestCocoaTextRune(t *testing.T) {
	cases := map[string]rune{
		"":     0,
		"a":    'a',
		"é":    'é',
		"ж":    'ж',
		" ":    ' ',
		"\r":   0,
		"\x1b": 0,
		"\x7f": 0,
		"":    0, // NSF1FunctionKey
		"":    0, // NSUpArrowFunctionKey
		"　":    '　',
		" x":   ' ',
		"ab":  0,
	}
	for in, want := range cases {
		if got := cocoaTextRune(in); got != want {
			t.Errorf("cocoaTextRune(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCocoaCellAt(t *testing.T) {
	cases := []struct {
		x, y, scale  float64
		cellW, cellH int
		wantX, wantY int16
	}{
		{0, 0, 1, 8, 16, 0, 0},
		{7.9, 15.9, 1, 8, 16, 0, 0},
		{8, 16, 1, 8, 16, 1, 1},
		// Retina: points are half the pixels the cells are measured in.
		{8, 16, 2, 16, 32, 1, 1},
		{7.5, 15.5, 2, 16, 32, 0, 0},
		// Just past the top-left edge still reads as the first cell.
		{-3, -3, 1, 8, 16, 0, 0},
		{-9, -17, 1, 8, 16, -1, -1},
		{10, 10, 1, 0, 16, 0, 0},
	}
	for _, tc := range cases {
		gx, gy := cocoaCellAt(tc.x, tc.y, tc.scale, tc.cellW, tc.cellH)
		if gx != tc.wantX || gy != tc.wantY {
			t.Errorf("cocoaCellAt(%v, %v, scale %v, %dx%d) = (%d,%d), want (%d,%d)",
				tc.x, tc.y, tc.scale, tc.cellW, tc.cellH, gx, gy, tc.wantX, tc.wantY)
		}
	}
}

func TestCocoaGridForPixels(t *testing.T) {
	if c, r := cocoaGridForPixels(800, 400, 8, 16); c != 100 || r != 25 {
		t.Errorf("800x400 at 8x16 is %dx%d, want 100x25", c, r)
	}
	if c, r := cocoaGridForPixels(805, 410, 8, 16); c != 100 || r != 25 {
		t.Errorf("805x410 at 8x16 is %dx%d, want 100x25", c, r)
	}
	if c, r := cocoaGridForPixels(3, 3, 8, 16); c != 1 || r != 1 {
		t.Errorf("a window smaller than a cell is %dx%d, want 1x1", c, r)
	}
	if c, r := cocoaGridForPixels(800, 400, 0, 0); c != 1 || r != 1 {
		t.Errorf("no cell size gives %dx%d, want 1x1", c, r)
	}
}

func TestCocoaWheelNotches_WheelMouseIsOneNotchPerEvent(t *testing.T) {
	var acc float64
	for _, tc := range []struct {
		delta     float64
		wantCount int
		wantDir   int
	}{
		{0.1, 1, 1},
		{4.5, 1, 1},
		{-0.1, 1, -1},
		{0, 0, 0},
	} {
		n, dir := cocoaWheelNotches(&acc, tc.delta, false, 48)
		if n != tc.wantCount || dir != tc.wantDir {
			t.Errorf("wheel delta %v: %d notches, dir %d; want %d, %d", tc.delta, n, dir, tc.wantCount, tc.wantDir)
		}
	}
}

func TestCocoaWheelNotches_TrackpadAccumulates(t *testing.T) {
	var acc float64
	total := 0
	for i := 0; i < 10; i++ {
		n, dir := cocoaWheelNotches(&acc, 5, true, 12)
		if n > 0 && dir != 1 {
			t.Fatalf("scrolling up produced direction %d", dir)
		}
		total += n
	}
	// 50 points of travel at 12 points a notch.
	if total != 4 {
		t.Errorf("50 points of trackpad travel gave %d notches, want 4", total)
	}
	// A reversal does not first have to undo what was left over.
	n, dir := cocoaWheelNotches(&acc, -12, true, 12)
	if n != 1 || dir != -1 {
		t.Errorf("reversing by one notch gave %d notches, dir %d; want 1, -1", n, dir)
	}
}

func TestComposeCocoaCanvas_GridTopLeftMarginsBlack(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	copy(src.Pix, []byte{10, 20, 30, 255, 40, 50, 60, 255})

	// A 3x2 canvas with two bytes of row padding, which Core Graphics is
	// free to add.
	const w, h, stride = 3, 2, 14
	dst := bytes.Repeat([]byte{0xAA}, stride*h)
	composeCocoaCanvas(dst, w, h, stride, src)

	want := []byte{
		10, 20, 30, 255, 40, 50, 60, 255, 0, 0, 0, 255, 0xAA, 0xAA,
		0, 0, 0, 255, 0, 0, 0, 255, 0, 0, 0, 255, 0xAA, 0xAA,
	}
	if !bytes.Equal(dst, want) {
		t.Errorf("canvas\n got %v\nwant %v", dst, want)
	}

	// The canvas may also be smaller than the frame, right after a shrink.
	small := make([]byte, 4)
	composeCocoaCanvas(small, 1, 1, 4, src)
	if !bytes.Equal(small, []byte{10, 20, 30, 255}) {
		t.Errorf("1x1 canvas = %v", small)
	}

	composeCocoaCanvas(small, 1, 1, 4, nil)
	if !bytes.Equal(small, []byte{0, 0, 0, 255}) {
		t.Errorf("canvas without a frame = %v, want black", small)
	}
}

func TestCocoaGuiRenderer_ImplementsInterfaces(t *testing.T) {
	var _ SurfaceRenderer = (*CocoaGuiRenderer)(nil)
	var _ GraphicsRenderer = (*CocoaGuiRenderer)(nil)
	var _ softwareBlinkRenderer = (*CocoaGuiRenderer)(nil)
	var _ interface {
		WindowPosition() (int, int, bool)
		SetWindowPosition(int, int)
		ToggleMaximized() bool
	} = (*CocoaGuiRenderer)(nil)
}

func TestCocoaGuiRenderer_RenderComposeAndFlush(t *testing.T) {
	r := NewCocoaGuiRenderer(nil, nil, 4, 8)
	buf, shadow := mkGrid(3, 2, ' ', 0)
	red := SetRGBBoth(0, 0xFFFFFF, 0xE02020)
	buf[4] = CharInfo{Char: ' ', Attributes: red} // cell (1,1)

	r.Render(buf, shadow, 3, 2, true)
	if !r.dirty {
		t.Fatal("a first frame leaves the renderer clean")
	}
	r.Flush()
	if r.dirty {
		t.Fatal("Flush did not take the dirty flag")
	}

	// Compose onto a canvas one cell wider and taller than the grid.
	const w, h = 16, 24
	canvas := make([]byte, w*h*4)
	r.composeCanvas(canvas, w, h, w*4)
	at := func(x, y int) [4]byte {
		o := y*w*4 + x*4
		return [4]byte{canvas[o], canvas[o+1], canvas[o+2], canvas[o+3]}
	}
	if got := at(6, 12); got != [4]byte{0xE0, 0x20, 0x20, 255} {
		t.Errorf("centre of cell (1,1) = %v, want the red background", got)
	}
	bg := ThemePalette[GetIndexBack(0)]
	if got := at(1, 1); got != [4]byte{byte(bg >> 16 & 0xFF), byte(bg >> 8 & 0xFF), byte(bg & 0xFF), 255} {
		t.Errorf("cell (0,0) = %v, want palette background %06X", got, bg)
	}
	if got := at(14, 4); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("right margin = %v, want black", got)
	}
	if got := at(2, 20); got != [4]byte{0, 0, 0, 255} {
		t.Errorf("bottom margin = %v, want black", got)
	}
}

// The Win32 and Cocoa backends draw with one raster, so the same screen
// comes out as the same pixels in both.
func TestCocoaAndWin32RenderersShareTheRaster(t *testing.T) {
	buf, shadow := mkGrid(6, 3, 'x', SetRGBBoth(0, 0x00FF00, 0x000080))
	buf[7] = CharInfo{Char: '─', Attributes: SetIndexBoth(0, 15, 1)}
	buf[8] = CharInfo{Char: 'y', Attributes: SetIndexBoth(0, 14, 4) | CommonLvbUnderscore}

	cocoa := NewCocoaGuiRenderer(nil, nil, 8, 16)
	win32 := NewWin32GuiRenderer(nil, nil, 8, 16)
	cocoa.SetCursor(2, 1, true, CursorShapeBlock)
	win32.SetCursor(2, 1, true, CursorShapeBlock)
	cocoa.Render(buf, shadow, 6, 3, true)
	win32.Render(buf, shadow, 6, 3, true)

	if cocoa.imgBuf == nil || win32.imgBuf == nil {
		t.Fatal("no frame rendered")
	}
	if !bytes.Equal(cocoa.imgBuf.Pix, win32.imgBuf.Pix) {
		t.Error("the Cocoa and Win32 renderers composed different pixels for the same screen")
	}
}

func TestIsCocoaEnhancedNavKey(t *testing.T) {
	for _, vk := range []uint16{vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT, vtinput.VK_INSERT, vtinput.VK_DELETE} {
		if !isCocoaEnhancedNavKey(vk) {
			t.Errorf("VK 0x%02X should carry EnhancedKey", vk)
		}
	}
	// The arrows stay plain, as in the gogpu backend.
	for _, vk := range []uint16{vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_A, vtinput.VK_NUMPAD5} {
		if isCocoaEnhancedNavKey(vk) {
			t.Errorf("VK 0x%02X should not carry EnhancedKey", vk)
		}
	}
}

func TestCocoaCellAt_ClampsFarOffWindow(t *testing.T) {
	x, y := cocoaCellAt(1e9, -1e9, 1, 1, 1)
	if x != 32767 || y != -32768 {
		t.Errorf("cocoaCellAt far off the window = (%d,%d), want the int16 limits", x, y)
	}
}
