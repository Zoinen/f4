package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
		pressKey(pf, &vtinput.InputEvent{
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
		pressKey(pf, &vtinput.InputEvent{
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
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
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
	pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	q := pf.altPanels[0].(*QuickViewPanel)
	pf.Show(scr)

	// Tab — active moves to left (alt slot). From now on wheel should
	// hit the alt, regardless of mouse pointer.
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
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

// TestQuickView_DirScan_PopulatesRecursive builds a small tree and
// checks that the async scan settles on the right recursive counts.
// The scan runs in a goroutine, so we wait on scanDoneCh with a
// generous test timeout instead of polling.
func TestQuickView_DirScan_PopulatesRecursive(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	// Layout:
	//   dir/
	//     a.txt    (100 bytes)
	//     sub/
	//       b.txt  (50 bytes)
	// Expected recursive: Folders=1 (sub), Files=2, Bytes=150.
	dir := filepath.Join(tmp, "dir")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 100), 0644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), make([]byte, 50), 0644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "dir", IsDir: true}},
	}
	fsp.cursorIdx = 1
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 19)
	q.Show(scr) // triggers refreshCache → startDirScan

	// Wait for the scan goroutine to close its done channel.
	q.scanMu.Lock()
	done := q.scanDoneCh
	q.scanMu.Unlock()
	if done == nil {
		t.Fatal("startDirScan didn't create scanDoneCh")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not finish within 5s")
	}

	q.scanMu.Lock()
	stats := q.scanStats
	scanDone := q.scanDone
	scanErr := q.scanErr
	q.scanMu.Unlock()

	if !scanDone {
		t.Fatal("scanDone should be true after channel close")
	}
	if scanErr != nil {
		t.Fatalf("unexpected scan error: %v", scanErr)
	}
	// CalculateStats counts the base dir itself, so Dirs is 2 (dir+sub).
	// renderDir subtracts 1 for display, but here we assert raw stats.
	if stats.Dirs != 2 {
		t.Errorf("Dirs = %d, want 2", stats.Dirs)
	}
	if stats.Files != 2 {
		t.Errorf("Files = %d, want 2", stats.Files)
	}
	if stats.Bytes != 150 {
		t.Errorf("Bytes = %d, want 150", stats.Bytes)
	}
	// PhysicalBytes is populated per-item by the VFS (stat.Blocks on
	// Unix / GetCompressedFileSize on Windows) and accumulated by the
	// scanner. On Unix tempdirs the block count is always > 0 for a
	// dense file, so the sum must be at least the logical byte count.
	if stats.PhysicalBytes < stats.Bytes {
		t.Errorf("PhysicalBytes (%d) < Bytes (%d) — dense files should not shrink under scan",
			stats.PhysicalBytes, stats.Bytes)
	}
}

// TestQuickView_DotDot_ScansCurrentDir locks in far2/far2l behaviour:
// with the cursor on "..", the panel shows the running scan of the
// CURRENT dir (basename in the title), not a static "Parent directory"
// note.
func TestQuickView_DotDot_ScansCurrentDir(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "one.bin"), make([]byte, 200), 0644); err != nil {
		t.Fatal(err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "one.bin", Size: 200}},
	}
	fsp.cursorIdx = 0 // sit on ".."
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 19)
	q.Show(scr) // triggers scan of tmp itself

	q.scanMu.Lock()
	done := q.scanDoneCh
	q.scanMu.Unlock()
	if done == nil {
		t.Fatal("dot-dot should have triggered a scan (no scanDoneCh)")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not finish within 5s")
	}

	q.scanMu.Lock()
	stats := q.scanStats
	q.scanMu.Unlock()

	if stats.Files != 1 || stats.Bytes != 200 {
		t.Errorf("Files=%d Bytes=%d, want Files=1 Bytes=200", stats.Files, stats.Bytes)
	}
}

// TestQuickView_DirScan_CancelsOnSelectionChange checks that starting
// a second scan cancels the first one — the old goroutine drops its
// callbacks (scanGen mismatch) and doesn't clobber the new scanStats.
func TestQuickView_DirScan_CancelsOnSelectionChange(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "A")
	dirB := filepath.Join(tmp, "B")
	if err := os.MkdirAll(dirA, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "onlyA.bin"), make([]byte, 42), 0644); err != nil {
		t.Fatal(err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "A", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "B", IsDir: true}},
	}
	fsp.cursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 19)
	q.Show(scr) // starts scan on A

	q.scanMu.Lock()
	firstGen := q.scanGen
	q.scanMu.Unlock()

	// Move to B and re-render — this cancels the A scan and starts a
	// B scan (empty).
	fsp.cursorIdx = 1
	q.Show(scr)

	q.scanMu.Lock()
	newGen := q.scanGen
	q.scanMu.Unlock()
	if newGen == firstGen {
		t.Fatalf("scanGen should bump on new dir; still %d", newGen)
	}

	q.scanMu.Lock()
	done := q.scanDoneCh
	q.scanMu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("B scan did not finish within 5s")
	}

	q.scanMu.Lock()
	stats := q.scanStats
	q.scanMu.Unlock()

	// The final state must reflect B, not A. B is empty apart from
	// itself (Dirs=1, Files=0, Bytes=0). If the stale A callback ever
	// wrote through we'd see Bytes=42 or Files=1.
	if stats.Bytes != 0 || stats.Files != 0 {
		t.Errorf("stale A scan clobbered B state: %+v", stats)
	}
	if stats.Dirs != 1 {
		t.Errorf("B scan Dirs = %d, want 1 (self only)", stats.Dirs)
	}
}

// TestPanelsFrame_BToggle_WithQuickView ensures pressing plain `B`
// while a quick-view alt is up flips AppConfig.InfoPanelBytes. Before
// this PR the B toggle only fired for `info` alts.
func TestPanelsFrame_BToggle_WithQuickView(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	// Install a QuickView (Ctrl+Q). Active panel is right (activeIdx=1),
	// so alt lands on left (index 0).
	pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_Q, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if _, ok := pf.altPanels[0].(*QuickViewPanel); !ok {
		t.Fatalf("expected QuickView on left, got %T", pf.altPanels[0])
	}

	before := AppConfig.InfoPanelBytes
	pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_B,
	})
	if AppConfig.InfoPanelBytes == before {
		t.Error("B with QuickView visible should flip InfoPanelBytes")
	}
	// Flip back so the test is idempotent across a full suite.
	AppConfig.InfoPanelBytes = before
}

// TestQuickView_ImageFilePreview generates a valid 1x1 QOI image file,
// loads it in QuickView, and verifies that the image pipeline successfully
// decodes and registers the image surface.
func TestQuickView_ImageFilePreview(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	filePath := filepath.Join(tmp, "image.qoi")

	// Valid 1x1 QOI file bytes:
	// "qoif" + width(1) + height(1) + channels(4) + colorspace(0) + tagRGBA(0xff) + R(255), G(0), B(0), A(255)
	qoiBytes := []byte{
		'q', 'o', 'i', 'f',
		0, 0, 0, 1,
		0, 0, 0, 1,
		4, 0,
		0xff, 0xff, 0x00, 0x00, 0xff,
	}

	if err := os.WriteFile(filePath, qoiBytes, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "image.qoi", Size: int64(len(qoiBytes))}},
	}
	fsp.cursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)
	q.Show(scr) // Triggers refreshCache and ImagePipe.Load

	if !q.cacheImage {
		t.Error("Expected qoi file to be flagged as image")
	}

	// Drain tasks to process async image load on UI thread
	timeout := time.After(2 * time.Second)
	for q.imageSurf == nil && q.cacheReadErr == nil {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("Timeout waiting for image to decode")
		}
	}

	if q.cacheReadErr != nil {
		t.Fatalf("Image decode failed: %v", q.cacheReadErr)
	}

	if q.imageSurf == nil {
		t.Fatal("Expected imageSurf to be populated")
	}

	if q.imageSurf.Width != 1 || q.imageSurf.Height != 1 {
		t.Errorf("Unexpected image dimensions: %dx%d", q.imageSurf.Width, q.imageSurf.Height)
	}
}

// TestQuickView_ImageGraphicsNotSupported verifies that a fallback message
// is rendered when the output terminal or screen buffer does not support
// image graphics.
func TestQuickView_ImageGraphicsNotSupported(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	fsp.entries = []*fileEntry{
		{VFSItem: vfs.VFSItem{Name: "dummy.png", Size: 100}},
	}
	fsp.cursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)

	// Pre-seed cacheKey to skip async file loading during Test
	path := fsp.vfs.Join(fsp.vfs.GetPath(), "dummy.png")
	q.cacheKey = path
	q.cacheImage = true
	q.imageSurf = vtui.NewImageSurface(1, 1)

	q.Show(scr)

	foundNotSupported := false
	for y := q.Y1; y <= q.Y2; y++ {
		var line []rune
		for x := q.X1; x <= q.X2; x++ {
			ci := scr.GetCell(x, y)
			if ci.Char != 0 {
				line = append(line, rune(ci.Char))
			}
		}
		if strings.Contains(string(line), "not supported") {
			foundNotSupported = true
			break
		}
	}

	if !foundNotSupported {
		t.Error("Expected 'Image graphics not supported' or similar message in the output")
	}
}
