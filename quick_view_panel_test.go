package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestPanelsFrame_CtrlQ_TogglesQuickView mirrors the Ctrl+L test:
// first Ctrl+Q installs a QuickViewPanel on the passive side, second
// press removes it, Tab flips the focused marker without closing,
// and Ctrl+Q on the focused alt closes IT (not spawns a second one).
func TestPanelsFrame_CtrlQ_TogglesQuickView(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	send := func(vk uint16, mods vtinput.ControlKeyState) {
		pf.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
	}

	send(vtinput.VK_Q, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] == nil {
		t.Fatal("Ctrl+Q should install alt on passive (left) side")
	}
	if _, ok := pf.altPanels[0].(*QuickViewPanel); !ok {
		t.Errorf("expected *QuickViewPanel, got %T", pf.altPanels[0])
	}

	// Second press toggles off.
	send(vtinput.VK_Q, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("second Ctrl+Q should remove alt panel")
	}

	// Reinstall, Tab, Ctrl+Q — closes the focused alt, doesn't spawn another.
	send(vtinput.VK_Q, vtinput.LeftCtrlPressed)
	send(vtinput.VK_TAB, 0)
	if pf.altPanels[0] == nil {
		t.Error("Tab must NOT close the alt panel")
	}
	send(vtinput.VK_Q, vtinput.LeftCtrlPressed)
	if pf.altPanels[0] != nil {
		t.Error("Ctrl+Q on focused quick-view should close it")
	}
	if pf.altPanels[1] != nil {
		t.Error("Ctrl+Q on focused alt must not spawn a second alt")
	}
}

// TestPanelsFrame_CtrlLQ_CoexistOnDifferentSides verifies the two
// hotkeys can point at different alt-panel kinds simultaneously —
// info on one side, quick view on the other — without collisions.
func TestPanelsFrame_CtrlLQ_CoexistOnDifferentSides(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	send := func(vk uint16) {
		pf.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true,
			VirtualKeyCode:  vk,
			ControlKeyState: vtinput.LeftCtrlPressed,
		})
	}

	// active = right → Ctrl+L opens info on left.
	send(vtinput.VK_L)
	if _, ok := pf.altPanels[0].(*InfoPanel); !ok {
		t.Fatalf("expected InfoPanel on left, got %T", pf.altPanels[0])
	}
	// Tab to left (so info is now on active side).
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if pf.activeIdx != 0 {
		t.Fatalf("Tab: expected activeIdx=0, got %d", pf.activeIdx)
	}
	// Ctrl+Q now — active side (0) has info, not quick-view, so
	// toggleAltPanel opens quick-view on the passive side (1).
	send(vtinput.VK_Q)
	if _, ok := pf.altPanels[1].(*QuickViewPanel); !ok {
		t.Errorf("expected QuickViewPanel on right, got %T", pf.altPanels[1])
	}
	if _, ok := pf.altPanels[0].(*InfoPanel); !ok {
		t.Errorf("Ctrl+Q must not disturb existing InfoPanel on left, got %T", pf.altPanels[0])
	}
}

// TestQuickView_TextFilePreview drives QuickView over a real text
// file and checks that its cached content starts with the file's
// first line — no panics, no binary heuristic tripping.
func TestQuickView_TextFilePreview(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(filePath, []byte("first line\nsecond line\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "hello.txt", Size: 23}},
	}
	fsp.cursorIdx = 1
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 19)
	q.Show(scr) // triggers refreshCache

	if q.cacheBinary {
		t.Error("plain text file should not be flagged as binary")
	}
	if len(q.cacheLines) < 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(q.cacheLines), q.cacheLines)
	}
	if !strings.Contains(q.cacheLines[0], "first line") {
		t.Errorf("first cached line %q should contain 'first line'", q.cacheLines[0])
	}
}

// TestQuickView_ScrollAndWrap drives QuickView.ProcessKey directly
// with plenty of content so scroll and F2 wrap-toggle can actually
// change something. Verifies the panel eats plain arrows / PgDn /
// Home / End / F2 when focused and lets them through when not.
func TestQuickView_ScrollAndWrap(t *testing.T) {
	tmp := t.TempDir()
	// Build a file with 50 non-trivial-length lines so both vertical
	// and horizontal scroll have something to move against.
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString(strings.Repeat("abcdefghij", 20)) // 200 cols per line
		b.WriteByte('\n')
	}
	path := filepath.Join(tmp, "long.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "long.txt", Size: int64(b.Len())}},
	}
	fsp.cursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)
	// One render primes the cache and computes displayLines.
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()
	q.Show(scr)

	// Not focused: ProcessKey should decline every key.
	if q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Error("unfocused panel must not consume arrow keys")
	}

	q.SetFocus(true)
	before := q.scrollY
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if q.scrollY != before+1 {
		t.Errorf("Down: scrollY=%d, want %d", q.scrollY, before+1)
	}
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if q.scrollY <= before+1 {
		t.Errorf("PgDn should scroll further; scrollY=%d", q.scrollY)
	}
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if q.scrollY != 0 {
		t.Errorf("Home: scrollY=%d, want 0", q.scrollY)
	}

	// Wrap flip via F2. Toggle it, then a second render must produce
	// a different displayLines count than the wrapped version — with
	// wrap OFF, one source line = one display line; with wrap ON,
	// each 200-col line becomes multiple 38-cell chunks.
	if !q.wrap {
		t.Fatalf("precondition: expected wrap=true by default")
	}
	q.Show(scr)
	wrappedCount := len(q.displayLines)

	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})
	if q.wrap {
		t.Error("F2 should have flipped wrap off")
	}
	q.Show(scr)
	if len(q.displayLines) >= wrappedCount {
		t.Errorf("wrap-off should produce fewer display lines than wrap-on: off=%d wrap=%d",
			len(q.displayLines), wrappedCount)
	}

	// Horizontal scroll only affects wrap-off. Right → scrollX up.
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if q.scrollX != 1 {
		t.Errorf("Right: scrollX=%d, want 1", q.scrollX)
	}
	// Left below zero should clamp.
	q.scrollX = 0
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if q.scrollX != 0 {
		t.Errorf("Left at 0 must clamp; scrollX=%d", q.scrollX)
	}
}

// TestPanelsFrame_QuickViewWheel_ActivePanelScrolls locks in
// far/far2l behaviour: whichever panel is active gets scrolled by the
// wheel, regardless of where the mouse points. If the active slot is
// covered by a quick-view alt panel, the alt scrolls — not the file
// panel underneath.
func TestPanelsFrame_QuickViewWheel_ActivePanelScrolls(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	tmp := t.TempDir()
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line\n")
	}
	os.WriteFile(filepath.Join(tmp, "big.txt"), []byte(b.String()), 0644)
	fsp := pf.panels[1].(*FileSystemPanel)
	fsp.vfs = vfs.NewOSVFS(tmp)
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "big.txt", Size: int64(b.Len())}}}
	fsp.cursorIdx = 0
	fsp.Refresh()

	// Ctrl+Q — alt lands on left (opposite of active=right).
	pf.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	q := pf.altPanels[0].(*QuickViewPanel)
	pf.Show(scr)

	// Tab — active moves to left (alt slot). From now on wheel should
	// hit the alt, regardless of mouse pointer.
	pf.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	pf.Show(scr)

	before := q.scrollY
	// Point the mouse at the RIGHT half (over the file panel, not
	// over the alt) — active-side rule should still send wheel to alt.
	pf.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		MouseEventFlags: vtinput.MouseWheeled,
		WheelDirection:  -1,
		MouseX:          60,
		MouseY:          10,
	})
	if q.scrollY == before {
		t.Errorf("wheel over passive side should still scroll active alt; scrollY=%d", q.scrollY)
	}
}

// TestQuickView_MouseWheelScrolls confirms the wheel drives scrollY.
func TestQuickView_MouseWheelScrolls(t *testing.T) {
	tmp := t.TempDir()
	var b strings.Builder
	for i := 0; i < 100; i++ {
		b.WriteString("line\n")
	}
	path := filepath.Join(tmp, "many.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "many.txt", Size: int64(b.Len())}}}
	fsp.cursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()
	q.Show(scr)

	q.SetFocus(true)
	// Simulate the Linux SGR event shape (WheelDirection set, but
	// MouseWheeled flag not set) — this is what tripped the earlier
	// missed-scroll on Linux while Windows worked fine.
	q.ProcessMouse(&vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: -1, // scroll down
	})
	if q.scrollY <= 0 {
		t.Errorf("wheel down should advance scrollY; got %d", q.scrollY)
	}
	before := q.scrollY
	q.ProcessMouse(&vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: +1, // scroll up
	})
	if q.scrollY >= before {
		t.Errorf("wheel up should retreat scrollY; got %d, was %d", q.scrollY, before)
	}
}

// TestQuickView_BinaryDetection ensures a NUL byte flips looksBinary.
func TestQuickView_BinaryDetection(t *testing.T) {
	if !looksBinary([]byte{'A', 0, 'B'}) {
		t.Error("NUL byte should mark buffer as binary")
	}
	if looksBinary([]byte("plain ascii text\n")) {
		t.Error("plain ascii must not be flagged as binary")
	}
	if looksBinary(nil) {
		t.Error("empty buffer is not binary")
	}
}
