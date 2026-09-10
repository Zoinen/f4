package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestQuickView_TextFilePreview drives QuickView over a real text
// file and checks that its cached content starts with the file's
// first line — no panics, no binary heuristic tripping.
func TestQuickView_TextFilePreview(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	FilePath := filepath.Join(tmp, "hello.txt")
	if err := os.WriteFile(FilePath, []byte("first line\nsecond line\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "hello.txt", Size: 23}},
	}
	fsp.CursorIdx = 1
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
		if _, err := b.WriteString(strings.Repeat("abcdefghij", 20)); err != nil { // 200 cols per line
			t.Fatal(err)
		}
		if err := b.WriteByte('\n'); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(tmp, "long.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "long.txt", Size: int64(b.Len())}},
	}
	fsp.CursorIdx = 0
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
	before := q.ScrollY
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if q.ScrollY != before+1 {
		t.Errorf("Down: scrollY=%d, want %d", q.ScrollY, before+1)
	}
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if q.ScrollY <= before+1 {
		t.Errorf("PgDn should scroll further; scrollY=%d", q.ScrollY)
	}
	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if q.ScrollY != 0 {
		t.Errorf("Home: scrollY=%d, want 0", q.ScrollY)
	}

	// Wrap flip via F2. Toggle it, then a second render must produce
	// a different displayLines count than the wrapped version — with
	// wrap OFF, one source line = one display line; with wrap ON,
	// each 200-col line becomes multiple 38-cell chunks.
	if !q.Wrap {
		t.Fatalf("precondition: expected wrap=true by default")
	}
	q.Show(scr)
	wrappedCount := len(q.displayLines)

	q.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F2})
	if q.Wrap {
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

// TestQuickView_MouseWheelScrolls confirms the wheel drives scrollY.
func TestQuickView_MouseWheelScrolls(t *testing.T) {
	tmp := t.TempDir()
	var b strings.Builder
	for i := 0; i < 100; i++ {
		if _, err := b.WriteString("line\n"); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(tmp, "many.txt")
	if err := os.WriteFile(path, []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "many.txt", Size: int64(b.Len())}}}
	fsp.CursorIdx = 0
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
	if q.ScrollY <= 0 {
		t.Errorf("wheel down should advance scrollY; got %d", q.ScrollY)
	}
	before := q.ScrollY
	q.ProcessMouse(&vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: +1, // scroll up
	})
	if q.ScrollY >= before {
		t.Errorf("wheel up should retreat scrollY; got %d, was %d", q.ScrollY, before)
	}
}

// TestQuickView_BinaryDetection ensures a NUL byte flips viewer.LooksBinary.
func TestQuickView_BinaryDetection(t *testing.T) {
	if !viewer.LooksBinary([]byte{'A', 0, 'B'}) {
		t.Error("NUL byte should mark buffer as binary")
	}
	if viewer.LooksBinary([]byte("plain ascii text\n")) {
		t.Error("plain ascii must not be flagged as binary")
	}
	if viewer.LooksBinary(nil) {
		t.Error("empty buffer is not binary")
	}
}

func TestQuickView_CodepageAndHexToggle(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	raw, err := vfs.EncodeBytes([]byte("Привет\n"), 866)
	if err != nil {
		t.Fatal(err)
	}
	q := &QuickViewPanel{cacheRaw: raw, cacheCodepage: 866, cacheLines: nil}
	if !q.applyPreviewCodepage(866, false) {
		t.Fatal("applyPreviewCodepage failed")
	}
	if got := strings.Join(q.cacheLines, "\n"); got != "Привет" {
		t.Fatalf("decoded quick view = %q", got)
	}
	if !q.toggleHexMode() || !q.hexMode {
		t.Fatal("F4 should switch Quick View to hex mode")
	}
	if len(q.cacheLines) == 0 || !strings.Contains(q.cacheLines[0], "8F") {
		t.Fatalf("hex preview = %v", q.cacheLines)
	}
	if !q.toggleHexMode() || q.hexMode {
		t.Fatal("F4 should switch Quick View back to text mode")
	}
	if got := strings.Join(q.cacheLines, "\n"); got != "Привет" {
		t.Fatalf("text after hex toggle = %q", got)
	}
}

func TestQuickView_ShiftF7ContinuesSearchForward(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	q := &QuickViewPanel{
		Focused:          true,
		Wrap:             true,
		cacheLines:       []string{"needle first", "needle current", "needle next"},
		lastSearch:       "needle",
		lastSearchSource: 1,
	}
	q.SetPosition(0, 0, 39, 19)

	if !q.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode:  vtinput.VK_F7,
		ControlKeyState: vtinput.ShiftPressed,
	}) {
		t.Fatal("Shift+F7 should continue an existing Quick View search")
	}
	if q.lastSearchSource != 2 {
		t.Fatalf("Shift+F7 selected source line %d, want 2", q.lastSearchSource)
	}
}

func TestQuickView_RestoresRememberedCodepage(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	path := filepath.Join(root, "remembered.txt")
	v := vfs.NewOSVFS(root)
	raw, err := vfs.EncodeBytes([]byte("Привет\n"), 866)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	oldState := fileops.GlobalFileState
	fileops.GlobalFileState = &fileops.F4FileStateProvider{Limit: 10, Data: make(map[string]*fileops.FileState)}
	t.Cleanup(func() { fileops.GlobalFileState = oldState })
	fileops.GlobalFileState.SaveQuickViewCodepage(fileops.FileStateKey(v, path), 866)

	q := &QuickViewPanel{
		src:       &FileSystemPanel{Vfs: v},
		cachePath: path,
		CacheKey:  makeQuickViewSelectionKey(v, path, vfs.VFSItem{Name: "remembered.txt", Size: int64(len(raw))}),
	}
	q.applyFilePreview(quickViewFileResult{raw: raw, codepage: 65001, Lines: quickViewTextLines(raw)})

	if q.cacheCodepage != 866 {
		t.Fatalf("restored Quick View codepage = %d, want 866", q.cacheCodepage)
	}
	if got := strings.Join(q.cacheLines, "\n"); got != "Привет" {
		t.Fatalf("restored Quick View text = %q, want %q", got, "Привет")
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
	if err := os.MkdirAll(sub, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), make([]byte, 100), 0600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), make([]byte, 50), 0600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "dir", IsDir: true}},
	}
	fsp.CursorIdx = 1
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
	if err := os.WriteFile(filepath.Join(tmp, "one.bin"), make([]byte, 200), 0600); err != nil {
		t.Fatal(err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "one.bin", Size: 200}},
	}
	fsp.CursorIdx = 0 // sit on ".."
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
	if err := os.MkdirAll(dirA, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirB, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirA, "onlyA.bin"), make([]byte, 42), 0600); err != nil {
		t.Fatal(err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "A", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "B", IsDir: true}},
	}
	fsp.CursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 19)
	q.Show(scr) // starts scan on A

	q.scanMu.Lock()
	firstGen := q.scanGen
	q.scanMu.Unlock()

	// Move to B and re-render — this cancels the A scan and starts a
	// B scan (empty).
	fsp.CursorIdx = 1
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

// TestQuickView_ImageFilePreview generates a valid 1x1 QOI image file,
// loads it in QuickView, and verifies that the image pipeline successfully
// decodes and registers the image surface.
func TestQuickView_ImageFilePreview(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	FilePath := filepath.Join(tmp, "image.qoi")

	// Valid 1x1 QOI file bytes:
	// "qoif" + width(1) + height(1) + channels(4) + colorspace(0) + tagRGBA(0xff) + R(255), G(0), B(0), A(255)
	qoiBytes := []byte{
		'q', 'o', 'i', 'f',
		0, 0, 0, 1,
		0, 0, 0, 1,
		4, 0,
		0xff, 0xff, 0x00, 0x00, 0xff,
	}

	if err := os.WriteFile(FilePath, qoiBytes, 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "image.qoi", Size: int64(len(qoiBytes))}},
	}
	fsp.CursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)
	q.Show(scr) // Triggers refreshCache and media.ImagePipe.Load

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
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "dummy.png", Size: 100}},
	}
	fsp.CursorIdx = 0
	fsp.Refresh()

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 24)

	// Pre-seed cacheKey to skip async file loading during Test
	path := fsp.Vfs.Join(fsp.Vfs.GetPath(), "dummy.png")
	q.CacheKey = makeQuickViewSelectionKey(fsp.Vfs, path, fsp.Entries[0].VFSItem)
	q.cacheValid = true
	q.cacheImage = true
	q.imageSurf = vtui.NewImageSurface(1, 1)

	q.Show(scr)

	foundNotSupported := false
	for y := q.Y1; y <= q.Y2; y++ {
		var line []rune
		for x := q.X1; x <= q.X2; x++ {
			ci := scr.GetCell(x, y)
			if ci.Char != 0 {
				line = append(line, testutil.Rune(ci.Char))
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
