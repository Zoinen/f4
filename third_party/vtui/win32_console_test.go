package vtui

import "testing"

func TestWin32ColorMapping_Ansi16(t *testing.T) {
	cases := []struct {
		ansiIdx uint8
		want    uint16
	}{
		{0, 0},
		{1, Win32FgRed},
		{2, Win32FgGreen},
		{3, Win32FgRed | Win32FgGreen},
		{4, Win32FgBlue},
		{5, Win32FgRed | Win32FgBlue},
		{6, Win32FgGreen | Win32FgBlue},
		{7, Win32FgRed | Win32FgGreen | Win32FgBlue},
		{8, Win32FgIntensity},
		{9, Win32FgIntensity | Win32FgRed},
		{10, Win32FgIntensity | Win32FgGreen},
		{11, Win32FgIntensity | Win32FgRed | Win32FgGreen},
		{12, Win32FgIntensity | Win32FgBlue},
		{13, Win32FgIntensity | Win32FgRed | Win32FgBlue},
		{14, Win32FgIntensity | Win32FgGreen | Win32FgBlue},
		{15, Win32FgIntensity | Win32FgRed | Win32FgGreen | Win32FgBlue},
	}

	for _, tc := range cases {
		got := AnsiIndexToWin32Color(tc.ansiIdx)
		if got != tc.want {
			t.Errorf("AnsiIndexToWin32Color(%d) = %#04x, want %#04x", tc.ansiIdx, got, tc.want)
		}
	}
}

func TestWin32AttrMapping_AttributesAndStyles(t *testing.T) {
	// Index color (1: Red FG, 4: Blue BG)
	attr := SetIndexBoth(0, 1, 4) | ForegroundIntensity | CommonLvbUnderscore
	win32Attr := attrToWin32Attr(attr, nil)

	wantFG := Win32FgRed | Win32FgIntensity
	wantBG := Win32BgBlue
	wantStyles := Win32CommonLvbUnderscore

	expected := wantFG | wantBG | wantStyles
	if win32Attr != expected {
		t.Errorf("attrToWin32Attr() = %#04x, want %#04x (FG:%#04x BG:%#04x Styles:%#04x)",
			win32Attr, expected, wantFG, wantBG, wantStyles)
	}
}

func TestWin32AttrMapping_RGBQuantization(t *testing.T) {
	// Pure Green RGB FG (0x00FF00), Pure Red RGB BG (0xFF0000)
	attr := SetRGBBoth(0, 0x00FF00, 0xFF0000)
	win32Attr := attrToWin32Attr(attr, &XTerm256Palette)

	// ANSI Green is 2 (or 10), Red is 1 (or 9)
	fg := win32Attr & 0x000F
	bg := (win32Attr & 0x00F0) >> 4

	if fg&Win32FgGreen == 0 {
		t.Errorf("expected Green FG bit in %#04x", fg)
	}
	if bg&Win32FgRed == 0 {
		t.Errorf("expected Red BG bit in %#04x", bg)
	}
}

func TestCharInfoToWin32(t *testing.T) {
	ci := CharInfo{
		Char:       'A',
		Attributes: SetIndexBoth(0, 7, 0),
	}
	wCi := charInfoToWin32(ci, nil)

	if wCi.UnicodeChar != 'A' {
		t.Errorf("UnicodeChar = %c, want 'A'", rune(wCi.UnicodeChar))
	}
	if wCi.Attributes != (Win32FgRed | Win32FgGreen | Win32FgBlue) {
		t.Errorf("Attributes = %#04x, want light gray (%#04x)", wCi.Attributes, Win32FgRed|Win32FgGreen|Win32FgBlue)
	}

	// WideCharFiller should become a space
	ciFiller := CharInfo{Char: WideCharFiller, Attributes: 0}
	wCiFiller := charInfoToWin32(ciFiller, nil)
	if wCiFiller.UnicodeChar != ' ' {
		t.Errorf("WideCharFiller = %c, want ' '", rune(wCiFiller.UnicodeChar))
	}

	// Zero char should become a space
	ciZero := CharInfo{Char: 0, Attributes: 0}
	wCiZero := charInfoToWin32(ciZero, nil)
	if wCiZero.UnicodeChar != ' ' {
		t.Errorf("Zero char = %c, want ' '", rune(wCiZero.UnicodeChar))
	}
}

func TestWin32ConsoleRenderer_ImplementsSurfaceRenderer(t *testing.T) {
	var _ SurfaceRenderer = (*Win32ConsoleRenderer)(nil)
}

func TestDefaultConsoleBackend(t *testing.T) {
	backend := DefaultConsoleBackend()
	if backend != "winapi" && backend != "ansi" {
		t.Errorf("Expected 'winapi' or 'ansi' backend, got %q", backend)
	}
}

func TestGetTerminalSize_FallbackSafety(t *testing.T) {
	oldGet := GetTerminalSize
	defer func() { GetTerminalSize = oldGet }()

	w, h, err := GetTerminalSize()
	if err != nil {
		t.Fatalf("GetTerminalSize returned error: %v", err)
	}
	if w <= 0 || h <= 0 {
		t.Errorf("GetTerminalSize returned invalid dimensions: %dx%d", w, h)
	}
}

func TestHasConsoleBuffer_Safety(t *testing.T) {
	// Calling hasConsoleBufferOS should never panic on any platform
	_ = hasConsoleBufferOS()
}
func TestWin32CoordAndSmallRectTypes(t *testing.T) {
	var c win32Coord
	c.X = 80
	c.Y = 25
	if c.X != 80 || c.Y != 25 {
		t.Errorf("win32Coord values mismatch: got (%d, %d)", c.X, c.Y)
	}

	var sr win32SmallRect
	sr.Left = 0
	sr.Top = 0
	sr.Right = 79
	sr.Bottom = 24
	if sr.Right != 79 || sr.Bottom != 24 {
		t.Errorf("win32SmallRect values mismatch: got right=%d, bottom=%d", sr.Right, sr.Bottom)
	}
}
func TestScreenBuf_ApplyShadow_16ColorQuantization(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	// 1. Shadow over classic blue desktop (0x0000A0)
	desktopAttr := Palette[ColDesktopBackground]
	scr.FillRect(0, 0, 9, 4, ' ', desktopAttr)

	// Apply shadow on cell (2, 2)
	scr.ApplyShadow(2, 2, 2, 2)

	shadowCell := scr.GetCell(2, 2)
	win32Desktop := attrToWin32Attr(desktopAttr, &XTerm256Palette)
	win32Shadow := attrToWin32Attr(shadowCell.Attributes, &XTerm256Palette)

	// The shadow must produce a different Win32 attribute (Black background vs Blue background)
	if win32Desktop == win32Shadow {
		t.Errorf("Shadow on blue desktop must be distinguishable in Win32 16-color mode: desktop=%#04x shadow=%#04x",
			win32Desktop, win32Shadow)
	}
	if (win32Shadow & 0x00F0) != 0 {
		t.Errorf("Expected black background (0x00) for shadow on blue desktop in 16-color mode, got %#04x", win32Shadow&0x00F0)
	}

	// 2. Shadow over light gray dialog (0xC0C0C0)
	dialogAttr := Palette[ColDialogText]
	scr.FillRect(0, 0, 9, 4, ' ', dialogAttr)
	scr.ApplyShadow(2, 2, 2, 2)

	dlgShadowCell := scr.GetCell(2, 2)
	win32DlgShadow := attrToWin32Attr(dlgShadowCell.Attributes, &XTerm256Palette)

	// Expected Dark Gray background (0x80)
	if (win32DlgShadow & 0x00F0) != (Win32BgIntensity) {
		t.Errorf("Expected DarkGray background (%#04x) for shadow on dialog in 16-color mode, got %#04x",
			Win32BgIntensity, win32DlgShadow&0x00F0)
	}
}

func TestWin32ConsoleRenderer_ScreenBufIntegration(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)

	renderer := NewWin32ConsoleRenderer(scr)
	scr.Renderer = renderer

	scr.Write(0, 0, StringToCharInfo("WinAPI Test", Palette[ColDialogText]))
	scr.SetCursorPos(5, 0)
	scr.SetCursorVisible(true)
	scr.SetCursorShape(CursorShapeUnderline)

	// Flush should safely execute without panicking on any platform
	scr.Flush()
}

func TestFrameManager_GetBackendName_Win32(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	scr.Renderer = NewWin32ConsoleRenderer(scr)
	if name := fm.GetBackendName(); name != "Console (WinAPI)" {
		t.Errorf("GetBackendName() = %q, want 'Console (WinAPI)'", name)
	}

	scr.Renderer = &AnsiRenderer{parent: scr}
	if name := fm.GetBackendName(); name != "Console (ANSI)" {
		t.Errorf("GetBackendName() = %q, want 'Console (ANSI)'", name)
	}
}
func TestWin32AttrMapping_WineWorkaround(t *testing.T) {
	// Light Gray (7) on Blue (4)
	attr := SetIndexBoth(0, 7, 4)
	win32Attr := attrToWin32Attr(attr, nil)

	if isWineOS() {
		// Under Wine, FG 7 on non-black BG must be promoted to FG 15 (Bright White)
		want := Win32FgRed | Win32FgGreen | Win32FgBlue | Win32FgIntensity | Win32BgBlue
		if win32Attr != want {
			t.Errorf("attrToWin32Attr() under Wine = %#04x, want %#04x (promoted to bright white)", win32Attr, want)
		}
	} else {
		// On native Windows, FG 7 is preserved
		want := Win32FgRed | Win32FgGreen | Win32FgBlue | Win32BgBlue
		if win32Attr != want {
			t.Errorf("attrToWin32Attr() on native Windows = %#04x, want %#04x", win32Attr, want)
		}
	}

	// Light Gray (7) on Black (0) is never promoted
	blackBgAttr := SetIndexBoth(0, 7, 0)
	win32Black := attrToWin32Attr(blackBgAttr, nil)
	wantBlack := Win32FgRed | Win32FgGreen | Win32FgBlue
	if win32Black != wantBlack {
		t.Errorf("attrToWin32Attr() with black BG = %#04x, want %#04x", win32Black, wantBlack)
	}
}
