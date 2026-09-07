package main

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// waitForClipboard polls the clipboard until it matches want or the
// deadline expires. copyAndExit fires SetClipboard on a goroutine
// to keep the UI responsive, so tests must be tolerant of a small delay.
func waitForClipboard(t *testing.T, want string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := vtui.GetClipboard(); got == want {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	return vtui.GetClipboard()
}

const (
	testGrabberW = 40
	testGrabberH = 6
)

// setupGrabberScreen initialises the FrameManager with a silent 40x6
// screen and paints a couple of predictable rows so extraction tests
// can assert against known text.
func setupGrabberScreen(t *testing.T) *vtui.ScreenBuf {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(testGrabberW, testGrabberH)
	vtui.FrameManager.Init(scr)
	SetDefaultF4Palette()
	attr := vtui.SetRGBBoth(0, 0xFFFFFF, 0x000000)
	scr.FillRect(0, 0, testGrabberW-1, testGrabberH-1, ' ', attr)
	scr.Write(0, 0, vtui.StringToCharInfo("hello world", attr))
	scr.Write(0, 1, vtui.StringToCharInfo("second line trailing spaces      ", attr))
	scr.Write(0, 2, vtui.StringToCharInfo("third", attr))
	return scr
}

func keyDown(vk uint16, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: mods,
	}
}

func TestGrabber_SnapshotOnFirstShow(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	if !g.hasSnap {
		t.Fatal("expected snapshot to be taken on first Show")
	}
	if g.snapW != testGrabberW || g.snapH != testGrabberH {
		t.Fatalf("snapshot dims %dx%d, want %dx%d", g.snapW, g.snapH, testGrabberW, testGrabberH)
	}
	if testRune(g.snap[0][0].Char) != 'h' {
		t.Fatalf("snap[0][0]=%v, want 'h'", g.snap[0][0].Char)
	}
	// Cursor starts collapsed at (0,0) — anchor==cur.
	if g.curX != 0 || g.curY != 0 || g.anchorX != 0 || g.anchorY != 0 {
		t.Fatalf("initial cursor/anchor = (%d,%d)/(%d,%d), want origin",
			g.curX, g.curY, g.anchorX, g.anchorY)
	}
}

func TestGrabber_PlainArrowCollapsesSelection(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	// Extend right by 3 with Shift.
	for i := 0; i < 3; i++ {
		g.ProcessKey(keyDown(vtinput.VK_RIGHT, vtinput.ShiftPressed))
	}
	if g.curX != 3 || g.anchorX != 0 {
		t.Fatalf("after Shift+Right×3: cur=%d anchor=%d, want cur=3 anchor=0", g.curX, g.anchorX)
	}

	// Plain Right must move AND collapse anchor to cursor.
	g.ProcessKey(keyDown(vtinput.VK_RIGHT, 0))
	if g.curX != 4 || g.anchorX != 4 {
		t.Fatalf("after plain Right: cur=%d anchor=%d, want both 4", g.curX, g.anchorX)
	}
}

func TestGrabber_SelectAllAndReset(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	g.ProcessKey(keyDown(vtinput.VK_A, vtinput.LeftCtrlPressed))
	if g.anchorX != 0 || g.anchorY != 0 || g.curX != testGrabberW-1 || g.curY != testGrabberH-1 {
		t.Fatalf("Ctrl+A: anchor=(%d,%d) cur=(%d,%d), want (0,0)/(%d,%d)",
			g.anchorX, g.anchorY, g.curX, g.curY, testGrabberW-1, testGrabberH-1)
	}

	g.ProcessKey(keyDown(vtinput.VK_U, vtinput.LeftCtrlPressed))
	if g.anchorX != g.curX || g.anchorY != g.curY {
		t.Fatalf("Ctrl+U: anchor=(%d,%d) cur=(%d,%d), expected collapsed",
			g.anchorX, g.anchorY, g.curX, g.curY)
	}
}

func TestGrabber_CopyTextTrimsTrailingSpaces(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	// Select the full first row: (0,0) → (W-1, 0).
	g.ProcessKey(keyDown(vtinput.VK_END, vtinput.ShiftPressed))
	got := g.copyText()
	if got != "hello world" {
		t.Fatalf("row-0 copy = %q, want %q", got, "hello world")
	}

	// Now select the first three rows via Ctrl+A then Shift+Home to
	// bring cursor back... simpler: manually set state.
	g.anchorX, g.anchorY = 0, 0
	g.curX, g.curY = testGrabberW-1, 2
	got = g.copyText()
	wantLines := []string{"hello world", "second line trailing spaces", "third"}
	if got != strings.Join(wantLines, "\n") {
		t.Fatalf("multiline copy = %q, want %q", got, strings.Join(wantLines, "\n"))
	}
}

func TestGrabber_EnterCopiesAndExits(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	// Select "hello world" on row 0.
	g.anchorX, g.anchorY = 0, 0
	g.curX, g.curY = 10, 0
	// Clear whatever the runtime clipboard held from prior tests.
	vtui.SetClipboard("")

	g.ProcessKey(keyDown(vtinput.VK_RETURN, 0))
	if !g.IsDone() {
		t.Fatal("Enter should mark grabber Done")
	}
	if got := waitForClipboard(t, "hello world"); got != "hello world" {
		t.Fatalf("clipboard = %q, want %q", got, "hello world")
	}
}

func TestGrabber_CtrlInsCopiesAndExits(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	g.anchorX, g.anchorY = 0, 0
	g.curX, g.curY = 4, 0
	vtui.SetClipboard("")

	g.ProcessKey(keyDown(vtinput.VK_INSERT, vtinput.LeftCtrlPressed))
	if !g.IsDone() {
		t.Fatal("Ctrl+Ins should mark grabber Done")
	}
	if got := waitForClipboard(t, "hello"); got != "hello" {
		t.Fatalf("clipboard = %q, want %q", got, "hello")
	}
}

func TestGrabber_EscCancelsWithoutClipboard(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	vtui.SetClipboard("sentinel")
	g.anchorX, g.anchorY = 0, 0
	g.curX, g.curY = 4, 0

	g.ProcessKey(keyDown(vtinput.VK_ESCAPE, 0))
	if !g.IsDone() {
		t.Fatal("Esc should mark grabber Done")
	}
	if got := vtui.GetClipboard(); got != "sentinel" {
		t.Fatalf("clipboard = %q, want unchanged %q", got, "sentinel")
	}
}

func TestGrabber_AltInsToggleCloses(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	g.ProcessKey(keyDown(vtinput.VK_INSERT, vtinput.LeftAltPressed))
	if !g.IsDone() {
		t.Fatal("Alt+Ins on open grabber should close it")
	}
}

func mouseEvent(x, y int16, button uint32, moved, down bool) *vtinput.InputEvent {
	e := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      x,
		MouseY:      y,
		ButtonState: button,
		KeyDown:     down,
	}
	if moved {
		e.MouseEventFlags |= vtinput.MouseMoved
	}
	return e
}

func TestGrabber_MouseDragSelectsArea(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	// Press at (1,0) — anchors the selection.
	g.ProcessMouse(mouseEvent(1, 0, vtinput.FromLeft1stButtonPressed, false, true))
	if g.curX != 1 || g.curY != 0 || g.anchorX != 1 || g.anchorY != 0 {
		t.Fatalf("after press: cur=(%d,%d) anchor=(%d,%d), want (1,0)/(1,0)",
			g.curX, g.curY, g.anchorX, g.anchorY)
	}

	// Drag to (5,2) — extends the rectangle.
	g.ProcessMouse(mouseEvent(5, 2, vtinput.FromLeft1stButtonPressed, true, true))
	if g.curX != 5 || g.curY != 2 || g.anchorX != 1 || g.anchorY != 0 {
		t.Fatalf("after drag: cur=(%d,%d) anchor=(%d,%d), want (5,2)/(1,0)",
			g.curX, g.curY, g.anchorX, g.anchorY)
	}

	// Release stops the selection, keeping it highlighted.
	g.ProcessMouse(mouseEvent(5, 2, 0, false, false))
	if g.mouseSelecting {
		t.Fatal("release should clear mouseSelecting")
	}
	if got := g.copyText(); got == "" {
		t.Fatal("mouse drag should produce a non-empty selection")
	}

	// Subsequent pointer movement (button up) must NOT change the
	// frozen rectangle.
	g.ProcessMouse(mouseEvent(0, 5, 0, true, true))
	if g.curX != 5 || g.curY != 2 || g.anchorX != 1 || g.anchorY != 0 {
		t.Fatalf("after release hover: cur=(%d,%d) anchor=(%d,%d), want frozen (5,2)/(1,0)",
			g.curX, g.curY, g.anchorX, g.anchorY)
	}

	// A new press starts a fresh selection again.
	g.ProcessMouse(mouseEvent(2, 1, vtinput.FromLeft1stButtonPressed, false, true))
	if g.anchorX != 2 || g.anchorY != 1 || g.curX != 2 || g.curY != 1 {
		t.Fatalf("after re-press: cur=(%d,%d) anchor=(%d,%d), want (2,1)/(2,1)",
			g.curX, g.curY, g.anchorX, g.anchorY)
	}
}

func TestGrabber_JumpKeys(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	g.ProcessKey(keyDown(vtinput.VK_END, 0))
	if g.curX != testGrabberW-1 || g.curY != 0 {
		t.Fatalf("End: cur=(%d,%d), want (%d,0)", g.curX, g.curY, testGrabberW-1)
	}
	g.ProcessKey(keyDown(vtinput.VK_NEXT, 0))
	if g.curY != testGrabberH-1 {
		t.Fatalf("PgDn: curY=%d, want %d", g.curY, testGrabberH-1)
	}
	g.ProcessKey(keyDown(vtinput.VK_HOME, vtinput.LeftCtrlPressed))
	if g.curX != 0 || g.curY != 0 {
		t.Fatalf("Ctrl+Home: cur=(%d,%d), want (0,0)", g.curX, g.curY)
	}
	g.ProcessKey(keyDown(vtinput.VK_END, vtinput.LeftCtrlPressed))
	if g.curX != testGrabberW-1 || g.curY != testGrabberH-1 {
		t.Fatalf("Ctrl+End: cur=(%d,%d), want (%d,%d)", g.curX, g.curY, testGrabberW-1, testGrabberH-1)
	}
}

func TestGrabber_ShiftTrackedViaVKShift(t *testing.T) {
	// Table-driven: X11 hosts deliver Shift as VK_LSHIFT/VK_RSHIFT,
	// Windows console tends to send the generic VK_SHIFT.
	for _, shiftVK := range []uint16{vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT} {
		scr := setupGrabberScreen(t)
		g := NewGrabberFrame()
		g.Show(scr)

		g.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode: shiftVK,
		})
		g.ProcessKey(keyDown(vtinput.VK_RIGHT, 0))
		g.ProcessKey(keyDown(vtinput.VK_RIGHT, 0))
		if g.curX != 2 || g.anchorX != 0 {
			t.Fatalf("shift VK=0x%x extend: cur=%d anchor=%d, want cur=2 anchor=0",
				shiftVK, g.curX, g.anchorX)
		}

		g.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: false,
			VirtualKeyCode: shiftVK,
		})
		g.ProcessKey(keyDown(vtinput.VK_RIGHT, 0))
		if g.curX != 3 || g.anchorX != 3 {
			t.Fatalf("shift VK=0x%x post-release arrow: cur=%d anchor=%d, want both 3",
				shiftVK, g.curX, g.anchorX)
		}
	}
}

func TestGrabber_ShowHighlightsSelectionRect(t *testing.T) {
	scr := setupGrabberScreen(t)
	g := NewGrabberFrame()
	g.Show(scr)

	baseAttr := scr.GetCell(2, 0).Attributes
	baseFG := vtui.GetRGBFore(baseAttr)
	baseBG := vtui.GetRGBBack(baseAttr)
	if baseFG == baseBG {
		t.Fatal("test setup: base fg and bg must differ so a swap is observable")
	}

	g.anchorX, g.anchorY = 1, 0
	g.curX, g.curY = 3, 0
	g.Show(scr)

	// Inside the rect: fg/bg swapped.
	for x := 1; x <= 3; x++ {
		cell := scr.GetCell(x, 0).Attributes
		if vtui.GetRGBFore(cell) != baseBG || vtui.GetRGBBack(cell) != baseFG {
			t.Errorf("cell (%d,0) not color-swapped: fg=%06x bg=%06x, want fg=%06x bg=%06x",
				x, vtui.GetRGBFore(cell), vtui.GetRGBBack(cell), baseBG, baseFG)
		}
	}
	// Outside the rect: colors unchanged.
	for _, x := range []int{0, 4} {
		cell := scr.GetCell(x, 0).Attributes
		if vtui.GetRGBFore(cell) != baseFG || vtui.GetRGBBack(cell) != baseBG {
			t.Errorf("cell (%d,0) was modified but should be outside selection", x)
		}
	}
}

func TestGrabber_InvertAttrColorsRGB(t *testing.T) {
	// White on black → black on white, other flags preserved.
	src := vtui.SetRGBBoth(vtui.ForegroundIntensity, 0xFFFFFF, 0x000000)
	got := invertAttrColors(src)
	if vtui.GetRGBFore(got) != 0x000000 {
		t.Errorf("fg after swap = %06x, want 000000", vtui.GetRGBFore(got))
	}
	if vtui.GetRGBBack(got) != 0xFFFFFF {
		t.Errorf("bg after swap = %06x, want FFFFFF", vtui.GetRGBBack(got))
	}
	if got&vtui.ForegroundIntensity == 0 {
		t.Error("style flags must survive the color swap")
	}
	if got&vtui.IsFgRGB == 0 || got&vtui.IsBgRGB == 0 {
		t.Error("RGB flags must be set on both slots")
	}
}
