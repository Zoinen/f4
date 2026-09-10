package viewer

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type trackedReadRange struct {
	offset int64
	length int
}

type tailTrackingFile struct {
	size         int64
	newlineEvery int64
	mu           sync.Mutex
	read         []trackedReadRange
}

type partialHeaderFile struct {
	data []byte
}

func (f *partialHeaderFile) Size() int64 { return int64(len(f.data)) }
func (*partialHeaderFile) Close() error  { return nil }
func (f *partialHeaderFile) Read(ctx context.Context, p []byte) (int, error) {
	return f.ReadAt(ctx, p, 0)
}
func (f *partialHeaderFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

type largeBinaryFile struct {
	size    int64
	mu      sync.Mutex
	maxRead int
	reads   []trackedReadRange
}

func TestViewer_UsesDedicatedScrollbarPaletteSlot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}

	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(root), path)
	if err != nil {
		t.Fatalf("NewViewerView: %v", err)
	}
	defer vv.Close()

	if vv.ScrollBar == nil {
		t.Fatal("viewer scrollbar was not initialized")
	}
	if vv.ScrollBar.ColorIdx != theme.ColViewerScrollbar {
		t.Fatalf("viewer scrollbar color index = %d, want %d", vv.ScrollBar.ColorIdx, theme.ColViewerScrollbar)
	}
}

func TestViewerRenderHighlightsCurrentSearchResult(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()

	data := []byte("needle before needle after")
	backend := &ViewerBackend{
		File:      &vfs.MemoryReadAtCloser{Data: data},
		size:      int64(len(data)),
		cacheData: data,
	}
	defer func() { _ = backend.Close() }()

	vv := &ViewerView{
		Backend:          backend,
		WrapMode:         false,
		LastSearch:       "needle",
		LastSearchOffset: 0,
		LastSearchFound:  true,
	}
	vv.SetPosition(0, 0, 79, 3)
	vv.SetVisible(true)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 4)
	vv.DisplayObject(scr)

	for x := 0; x < len("needle"); x++ {
		if got := scr.GetCell(x, 1).Attributes; got != vtui.Palette[theme.ColViewerSelectedText] {
			t.Fatalf("matched cell %d has attributes %016x, want %016x", x, got, vtui.Palette[theme.ColViewerSelectedText])
		}
	}
	second := strings.LastIndex(string(data), "needle")
	for x := second; x < second+len("needle"); x++ {
		if got := scr.GetCell(x, 1).Attributes; got != vtui.Palette[theme.ColViewerText] {
			t.Fatalf("unselected match cell %d has attributes %016x, want base %016x", x, got, vtui.Palette[theme.ColViewerText])
		}
	}
}

func (f *largeBinaryFile) Size() int64 { return f.size }
func (*largeBinaryFile) Close() error  { return nil }
func (f *largeBinaryFile) Read(ctx context.Context, p []byte) (int, error) {
	return f.ReadAt(ctx, p, 0)
}
func (f *largeBinaryFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f.mu.Lock()
	if len(p) > f.maxRead {
		f.maxRead = len(p)
	}
	f.reads = append(f.reads, trackedReadRange{offset: off, length: len(p)})
	f.mu.Unlock()
	if off >= f.size {
		return 0, io.EOF
	}
	n := len(p)
	if remaining := f.size - off; int64(n) > remaining {
		n = int(remaining)
	}
	clear(p[:n])
	if n >= 6 && off == 0 {
		copy(p, []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C})
	}
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

type singleFileVFS struct {
	vfs.VFS
	File vfs.ReadAtCloser
}

func (v *singleFileVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return v.File, nil
}

func TestViewerLargeBinaryOpensLazilyInHexMode(t *testing.T) {
	File := &largeBinaryFile{size: 300 * 1024 * 1024}
	base := vfs.NewOSVFS(t.TempDir())
	vv, err := NewViewerView(context.Background(), &singleFileVFS{VFS: base, File: File}, "large.7z")
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()
	if !vv.HexMode {
		t.Fatal("large binary file did not open in hex mode")
	}
	File.mu.Lock()
	maxRead := File.maxRead
	File.mu.Unlock()
	if maxRead > 16*1024 {
		t.Fatalf("opening binary file read %d bytes at once, want at most the 16 KiB header", maxRead)
	}
}

func (f *tailTrackingFile) Size() int64  { return f.size }
func (f *tailTrackingFile) Close() error { return nil }
func (f *tailTrackingFile) Read(ctx context.Context, p []byte) (int, error) {
	return f.ReadAt(ctx, p, 0)
}
func (f *tailTrackingFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off >= f.size {
		return 0, io.EOF
	}
	n := len(p)
	if remaining := f.size - off; int64(n) > remaining {
		n = int(remaining)
	}
	for i := 0; i < n; i++ {
		p[i] = 'x'
	}
	if f.newlineEvery > 0 {
		for absolute := off; absolute < off+int64(n); absolute++ {
			if (absolute+1)%f.newlineEvery == 0 {
				p[absolute-off] = '\n'
			}
		}
	}
	f.mu.Lock()
	f.read = append(f.read, trackedReadRange{offset: off, length: n})
	f.mu.Unlock()
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (f *tailTrackingFile) ranges() []trackedReadRange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]trackedReadRange(nil), f.read...)
}

type mockCloseFile struct {
	vfs.ReadAtCloser
	closed bool
}

func (m *mockCloseFile) Close() error {
	m.closed = true
	return nil
}
func (m *mockCloseFile) Size() int64 { return 100 }
func (m *mockCloseFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return len(p), nil
}

type mockCloseVFS struct {
	vfs.VFS
	File *mockCloseFile
}

func (m *mockCloseVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	return m.File, nil
}

func TestViewerView_NavigationAndEOF(t *testing.T) {
	vtui.SetDefaultPalette()
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmp, []byte("L1\nL2\nL3\nL4\nL5"), 0600); err != nil { // 5 lines total
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	vv.SetPosition(0, 0, 10, 3) // Height 4 (Y:0..3). 1 line status, 3 lines content.

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(11, 4)
	vtui.FrameManager.Init(scr)

	// 1. Initial Render (Triggers async fetch)
	vv.Show(scr)

	// Wait for background loader to provide data
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("Timeout waiting for initial fetch")
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			vv.Show(scr) // Update internal lineOffsets
		default:
			time.Sleep(10 * time.Millisecond)
		}
		if len(vv.LineOffsets) > 1 {
			break
		}
	}

	if vv.TopOffset != 0 {
		t.Errorf("Initial offset should be 0, got %d", vv.TopOffset)
	}
	if vv.EofVisible {
		t.Error("EOF should not be visible initially")
	}

	// 2. Scroll Down (should move to L2)
	vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	vv.Show(scr)
	if vv.TopOffset <= 0 {
		t.Errorf("Offset should increase after VK_DOWN, got %d", vv.TopOffset)
	}

	// 3. Jump to End (L3, L4, L5 visible)
	vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})

	// VK_END triggers FindLineStart which triggers another fetch
	timeout := time.After(1 * time.Second)
	for !vv.EofVisible {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			vv.Show(scr)
		case <-timeout:
			t.Fatal("Timeout waiting for EOF fetch")
		}
	}

	if !vv.EofVisible {
		t.Error("EOF should be visible after VK_END")
	}

	// 4. Try scrolling past EOF
	oldOffset := vv.TopOffset
	vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if vv.TopOffset != oldOffset {
		t.Errorf("VK_DOWN should be blocked when eofVisible is true. Offset changed from %d to %d", oldOffset, vv.TopOffset)
	}
}

func TestViewerView_EndJumpReadsOnlyTailOfLargeFile(t *testing.T) {
	const (
		fileSize = int64(392077017)
		lineSize = int64(97)
		height   = int64(40)
	)
	want := ((fileSize-1)/lineSize - (height - 1)) * lineSize
	for _, tc := range []struct {
		name string
		wrap bool
	}{{name: "wrapped", wrap: true}, {name: "unwrapped", wrap: false}} {
		t.Run(tc.name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			file := &tailTrackingFile{size: fileSize, newlineEvery: lineSize}
			indexer := &indexingVFS{offsets: []int64{0}, total: 1}
			ctx, cancel := context.WithCancel(context.Background())
			backend := &ViewerBackend{
				File:         file,
				size:         fileSize,
				indexer:      indexer,
				totalLines:   -1,
				totalForSize: -1,
				ctx:          ctx,
				cancelCtx:    cancel,
			}
			vv := &ViewerView{
				Backend: backend, WrapMode: tc.wrap,
				NativeViewportColumns: 120, NativeViewportRows: 40,
			}
			defer vv.Close()

			if !vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END}) {
				t.Fatal("End was not handled")
			}
			deadline := time.After(2 * time.Second)
			for vv.Busy {
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
				case <-deadline:
					t.Fatal("bounded End jump timed out")
				}
			}

			if vv.TopOffset != want {
				t.Fatalf("End top offset=%d, want %d", vv.TopOffset, want)
			}
			if indexer.calls != 0 {
				t.Fatalf("anchored End jump made %d whole-file line-index calls", indexer.calls)
			}
			ranges := file.ranges()
			if len(ranges) != 1 {
				t.Fatalf("anchored End jump made %d source reads, want one: %+v", len(ranges), ranges)
			}
			for _, read := range ranges {
				if read.length > 256*1024 {
					t.Fatalf("End jump issued an oversized source read: %+v", read)
				}
				if read.offset < fileSize-256*1024 {
					t.Fatalf("End jump read outside the final source window: %+v", read)
				}
			}
		})
	}
}

func TestViewerView_MouseScrollbar(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	// Create a file with enough content to scroll
	content := "L1\nL2\nL3\nL4\nL5\nL6\nL7\nL8\nL9\nL10\n" // 10 lines, 33 bytes (3 per line + 1 for last \n)
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "test_mouse.txt")
	if err := os.WriteFile(tmp, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()
	t.Cleanup(func() {
		vv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	})

	// Setup viewport: 11 columns (X=0..10), 5 rows (Y=0..4)
	// Top bar at Y=0. Content area Y=1..4 (4 lines).
	// Scrollbar at X=10, Y=1..4.
	vv.SetPosition(0, 0, 10, 4)

	// Create a dummy ScreenBuf to pass to Show() for initial rendering.
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(11, 5) // width 11 (0..10), height 5 (0..4)
	vtui.FrameManager.Init(scr)

	// IMPORTANT: Call Show initially to populate vv.lineOffsets and set vv.TopOffset.
	// Without this, the navigation logic in ProcessKey has no context.
	vv.Show(scr)

	// Wait for background loader
	// Wait for background loader to populate cache and line offsets
	deadline := time.Now().Add(2 * time.Second)
	for len(vv.LineOffsets) < 2 {
		if time.Now().After(deadline) {
			t.Fatal("Timeout waiting for scrollbar initial fetch and line offsets")
		}
		vv.Show(scr) // Trigger ReadAt (miss) -> Fetch
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	vv.Show(scr)

	// Ensure we start at the top
	vv.TopOffset = 0

	// Check initial state, especially if TopOffset is correctly 0 and eofVisible is false.
	// With 10 lines and 4 content rows, we are definitely not at EOF.
	if vv.TopOffset != 0 {
		t.Errorf("Initial TopOffset expected 0, got %d", vv.TopOffset)
	}
	if vv.EofVisible {
		t.Error("Initial eofVisible expected false, got true")
	}

	// --- Test 1: Mouse wheel down ---
	oldOff := vv.TopOffset
	vv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	vv.Show(scr) // Re-render to update internal state (like vv.lineOffsets)

	if vv.TopOffset == oldOff {
		t.Error("Test 1: Mouse wheel down failed to increase TopOffset")
	}
	if vv.TopOffset != 3 { // Expected to move to start of L2 (offset 3)
		t.Errorf("Test 1: Expected TopOffset 3, got %d", vv.TopOffset)
	}

	// --- Test 2: Click on bottom arrow ---
	oldOff = vv.TopOffset // Should be 3
	vv.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true, // Important for click events
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      10, // Scrollbar X position
		MouseY:      4,  // Bottom arrow Y position (vv.Y2)
	})
	vv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	vv.Show(scr) // Re-render

	if vv.TopOffset == oldOff {
		t.Error("Test 2: Click on bottom arrow failed to increase TopOffset")
	}
	if vv.TopOffset != 6 { // Expected to move to start of L3 (offset 6)
		t.Errorf("Test 2: Expected TopOffset 6, got %d", vv.TopOffset)
	}

	// --- Test 3: Mouse wheel up ---
	oldOff = vv.TopOffset // Should be 6
	vv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	vv.Show(scr)

	if vv.TopOffset == oldOff {
		t.Error("Test 3: Mouse wheel up failed to decrease TopOffset")
	}
	if vv.TopOffset != 3 { // Expected to move to start of L2
		t.Errorf("Test 3: Expected TopOffset 3, got %d", vv.TopOffset)
	}

	// --- Test 4: Click on top arrow ---
	oldOff = vv.TopOffset // Should be 3
	vv.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      10, // Scrollbar X position
		MouseY:      1,  // Top arrow Y position (vv.Y1+1)
	})
	vv.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	vv.Show(scr)

	if vv.TopOffset == oldOff {
		t.Error("Test 4: Click on top arrow failed to decrease TopOffset")
	}
	if vv.TopOffset != 0 { // Expected to move to start of L1
		t.Errorf("Test 4: Expected TopOffset 0, got %d", vv.TopOffset)
	}
}

func TestViewerBar_Content(t *testing.T) {
	vtui.SetDefaultPalette()
	theme.SetDefaultF4Palette()
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "bar_test.txt")
	if err := os.WriteFile(tmp, []byte("Some content"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	vv.SetPosition(0, 0, 40, 10)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(41, 11)

	vv.HexMode = true
	vv.TopBar.Show(scr)

	// Проверяем, что в баре есть путь к файлу и режим "Hex"
	// Проверяем всю доступную ширину буфера (40 колонок)
	foundHex := false
	foundPath := false
	for x := 0; x <= 40; x++ {
		cell := scr.GetCell(x, 0)
		if cell.Char == 'H' {
			foundHex = true
		}
		if cell.Char == 'b' {
			foundPath = true
		} // часть "bar_test.txt"
	}

	if !foundHex {
		t.Error("ViewerBar did not display 'Hex' mode")
	}
	if !foundPath {
		t.Error("ViewerBar did not display file path")
	}
}
func TestViewerView_FileClosure(t *testing.T) {
	mockFile := &mockCloseFile{}
	v := &mockCloseVFS{File: mockFile}

	vv, err := NewViewerView(context.Background(), v, "test.txt")
	if err != nil {
		t.Fatalf("Failed to create viewer: %v", err)
	}

	// 1. Проверка закрытия через Close()
	vv.Close()

	if !vv.IsDone() {
		t.Error("Close() did not set IsDone")
	}
	if !mockFile.closed {
		t.Error("Close() did not close the underlying file")
	}

	// 2. Проверка закрытия через HandleCommand
	mockFile.closed = false
	vv, err = NewViewerView(context.Background(), v, "test.txt")
	if err != nil {
		t.Fatalf("Failed to create second viewer: %v", err)
	}
	vv.HandleCommand(vtui.CmClose, nil)

	if !vv.IsDone() {
		t.Error("CmClose did not set IsDone")
	}
	if !mockFile.closed {
		t.Error("CmClose did not close the underlying file")
	}
}
func TestViewerView_GetTitle(t *testing.T) {
	// Need to use an existing file for NewViewerView, or mock the backend.
	// For a simple title test, creating a temp file is easiest.
	tmpDir := t.TempDir()
	tmp := tmpDir + "/doc.txt"
	if err := os.WriteFile(tmp, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	if vv.GetTitle() != "View: doc.txt" {
		t.Errorf("GetTitle failed: %s", vv.GetTitle())
	}
	if vv.GetWorkspaceTabTitle() != "doc.txt" {
		t.Errorf("GetWorkspaceTabTitle failed: %s", vv.GetWorkspaceTabTitle())
	}
	if vv.GetWorkspaceTabMarker() != "V" {
		t.Errorf("GetWorkspaceTabMarker failed: %s", vv.GetWorkspaceTabMarker())
	}

}

func TestViewerTitle_FullPathSetting(t *testing.T) {
	old := config.App.DisplayFullPathInTitle
	t.Cleanup(func() { config.App.DisplayFullPathInTitle = old })

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nested", "doc.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}

	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(tmpDir), path)
	if err != nil {
		t.Fatalf("Failed to create NewViewerView: %v", err)
	}
	defer vv.Close()

	config.App.DisplayFullPathInTitle = false
	if got, want := vv.TopBar.GetLeft(), " doc.txt"; got != want {
		t.Fatalf("short viewer title = %q, want %q", got, want)
	}

	config.App.DisplayFullPathInTitle = true
	if got, want := vv.TopBar.GetLeft(), " "+path; got != want {
		t.Fatalf("full viewer title = %q, want %q", got, want)
	}
	if got, want := vv.GetWorkspaceTabTitle(), "doc.txt"; got != want {
		t.Fatalf("full-path viewer workspace tab title = %q, want %q", got, want)
	}
}
func TestLayout_ViewerSearchDialog_Validity(t *testing.T) {
	vtui.SetDefaultPalette()
	tmp := filepath.Join(t.TempDir(), "search_layout.txt")
	if err := os.WriteFile(tmp, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())

	// actionViewerSearch uses vtui.InputBox, which is already tested in vtui.
	// But we can check if the progress dialog it creates is valid.
	// We simulate the part of actionViewerSearch that creates the progress dlg.

	title := " Searching... "
	msg := "Looking for: pattern"

	dlg := vtui.NewCenteredDialog(50, 8, title)
	lbl := vtui.NewLabel(0, 0, msg, nil)
	dlg.AddItem(lbl)
	btnCancel := vtui.NewButton(0, 0, "&Cancel")
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 8-4)
	vbox.Add(lbl, vtui.Margins{}, vtui.AlignCenter)
	vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
	vbox.Apply()

	vtui.AssertLayout(t, dlg)
}

func TestViewerView_TabRendering(t *testing.T) {
	vtui.SetDefaultPalette()
	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "tab.txt")
	// "a\tb" -> tab should expand to spaces
	if err := os.WriteFile(tmp, []byte("a\tb"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	vv.SetPosition(0, 0, 10, 2) // Width 11

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(11, 3)
	vtui.FrameManager.Init(scr)

	vv.Show(scr)

	// Wait for background loader
	deadline := time.Now().Add(2 * time.Second)
	for !vv.EofVisible {
		if time.Now().After(deadline) {
			t.Fatal("Timeout waiting for tab view fetch")
		}
		vv.Show(scr)
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	vv.Show(scr)

	// Tab size is config.App.EditorTabSize (default 4).
	// "a" (col 0) -> "\t" starts at col 1, should take 3 spaces to reach col 4.
	// "b" should be at col 4.
	cell := scr.GetCell(4, 1) // Y=1 is content row
	if cell.Char != 'b' {
		t.Errorf("Tab expansion failed. Expected 'b' at column 4, got '%c' (U+%04X)", testutil.Rune(cell.Char), cell.Char)
	}

	// Columns 1, 2, 3 should be empty spaces (' ')
	for x := 1; x <= 3; x++ {
		c := scr.GetCell(x, 1)
		if c.Char != ' ' {
			t.Errorf("Expected space at col %d, got '%c'", x, testutil.Rune(c.Char))
		}
	}
}
func TestViewerView_EndJump_BusyState(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := filepath.Join(t.TempDir(), "large.txt")
	// Создаем файл, гарантированно превышающий размер окна
	if err := os.WriteFile(tmp, []byte(strings.Repeat("line\n", 1000)), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(t.TempDir())
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	vv.SetPosition(0, 0, 80, 24)

	// Нажимаем End
	vv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})

	if !vv.Busy {
		t.Error("Viewer should be in Busy state during End jump calculation")
	}

	// Ждем завершения асинхронной задачи
	timeout := time.After(2 * time.Second)
	for vv.Busy {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-timeout:
			t.Fatal("End jump timed out")
		}
	}

	if vv.TopOffset == 0 {
		t.Error("TopOffset should have moved away from 0 after End jump")
	}
}
func TestViewerView_StateRestoration_Modes(t *testing.T) {
	oldFileState := fileops.GlobalFileState
	t.Cleanup(func() { fileops.GlobalFileState = oldFileState })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmp := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmp, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(t.TempDir())

	fileops.GlobalFileState = &fileops.F4FileStateProvider{Data: make(map[string]*fileops.FileState), Limit: 10}
	fileops.GlobalFileState.SaveViewerState(tmp, 0, false, true) // Wrap OFF, Hex ON

	// Имитируем открытие (логика из actions.go)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	if state := fileops.GlobalFileState.GetState(tmp); state != nil {
		vv.WrapMode = state.ViewerWrap
		vv.HexMode = state.ViewerHex
	}

	if !vv.HexMode {
		t.Error("HexMode was not restored")
	}
	if vv.WrapMode {
		t.Error("WrapMode was not restored (should be false)")
	}
}
func TestViewerView_ScrollbarStability(t *testing.T) {
	fm := vtui.FrameManager
	fm.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	tmpDir := t.TempDir()
	tmp := filepath.Join(tmpDir, "stability_test.txt")
	if err := os.WriteFile(tmp, []byte(strings.Repeat("line\n", 200)), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, tmp)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()

	vv.SetPosition(0, 0, 40, 10)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(41, 11)

	vv.EofVisible = true
	vv.TopOffset = 10

	// Trigger initial show to start background fetch
	vv.Show(scr)

	// Pump tasks to wait for background fetch to complete and scrollbar to initialize
	timeout := time.After(2 * time.Second)
	for vv.ScrollBar.Max == 0 {
		select {
		case task := <-fm.TaskChan:
			task()
			vv.Show(scr)
		case <-timeout:
			t.Fatal("Timeout waiting for scrollbar Max to be populated")
		}
	}

	if vv.ScrollBar.Max != int(vv.Backend.Size()) {
		t.Errorf("Expected scrollbar Max to remain stable at %d even when eofVisible is true, got %d", vv.Backend.Size(), vv.ScrollBar.Max)
	}
}
func TestViewerView_Codepages_Load(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "oem.txt")

	raw := []byte{0x8f, 0xe0, 0xa8, 0xa2, 0xa5, 0xe2}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	oldDefault := config.App.ViewerDefaultCodePage
	config.App.ViewerDefaultCodePage = 866
	defer func() { config.App.ViewerDefaultCodePage = oldDefault }()

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()

	if vv.Codepage != 866 {
		t.Errorf("Expected detected codepage 866, got %d", vv.Codepage)
	}

	data, err := vv.Backend.ReadAt(0, 12)
	if err != nil {
		t.Fatalf("Already-decoded prefix should be readable immediately: %v", err)
	}

	if string(data) != "Привет" {
		t.Errorf("Viewer failed to decode CP866: expected 'Привет', got %q", string(data))
	}
}

func TestViewerView_UTF8BOMIsNotDisplayed(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.txt")
	text := "first line\nsecond line\n"
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte(text)...), 0600); err != nil {
		t.Fatal(err)
	}

	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(dir), path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()

	if vv.Backend.DataOffset != vfs.UTF8BOMSize {
		t.Fatalf("viewer data offset = %d, want %d", vv.Backend.DataOffset, vfs.UTF8BOMSize)
	}
	if got, want := vv.Backend.Size(), int64(len(text)); got != want {
		t.Fatalf("viewer logical size = %d, want %d", got, want)
	}

	var data []byte
	deadline := time.Now().Add(2 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for BOM-stripped viewer data")
		}
		data, err = vv.Backend.ReadAt(0, len(text))
		if err == nil {
			break
		}
		if err != piecetable.ErrLoading {
			t.Fatalf("viewer ReadAt = %v", err)
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(10 * time.Millisecond):
		}
	}
	if string(data) != text {
		t.Fatalf("viewer text = %q, want %q", string(data), text)
	}
	if got, ok := vv.Backend.LineStart(context.Background(), 2); !ok || got != int64(len("first line\n")) {
		t.Fatalf("LineStart(2) = %d, %v; want %d, true", got, ok, len("first line\n"))
	}
}

func TestViewerView_Codepages_AutoDetect(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "auto_view.txt")
	if err := os.WriteFile(path, []byte("plain ascii is valid utf8"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()

	config.App.ViewerAutodetectCodePage = true
	config.App.ViewerDefaultCodePage = 11111 // ANSI
	vv.ReloadWithAutoDetect()

	if vv.Codepage != 65001 {
		t.Errorf("Expected autodetect to recognize valid UTF-8/ASCII as 65001, got %d", vv.Codepage)
	}
}

func TestViewerView_Codepages_RestoresPerFileOverride(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "remembered_cp.txt")
	raw, err := vfs.EncodeBytes([]byte("Привет, сохранённая кодировка\n"), 866)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}

	oldState := fileops.GlobalFileState
	oldAuto, oldDefault := config.App.ViewerAutodetectCodePage, config.App.ViewerDefaultCodePage
	defer func() {
		fileops.GlobalFileState = oldState
		config.App.ViewerAutodetectCodePage = oldAuto
		config.App.ViewerDefaultCodePage = oldDefault
	}()
	fileops.GlobalFileState = &fileops.F4FileStateProvider{Limit: 10, Data: make(map[string]*fileops.FileState)}
	v := vfs.NewOSVFS(tmpDir)
	fileops.GlobalFileState.SaveCodepage(fileops.FileStateKey(v, path), 1251)
	config.App.ViewerAutodetectCodePage = true
	config.App.ViewerDefaultCodePage = 65001

	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()
	if vv.Codepage != 1251 {
		t.Fatalf("remembered Viewer codepage = %d, want 1251", vv.Codepage)
	}
}

func TestViewerView_Codepages_TrimsPartialHeader(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	raw, err := vfs.EncodeBytes([]byte("Привет, это достаточно длинный русский текст для определения кодировки.\n"+
		"Вторая строка содержит числа 123 и знаки препинания.\n"), 866)
	if err != nil {
		t.Fatal(err)
	}
	File := &partialHeaderFile{data: raw}
	base := vfs.NewOSVFS(t.TempDir())
	vv, err := NewViewerView(context.Background(), &singleFileVFS{VFS: base, File: File}, "partial.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()
	if vv.Codepage != 866 && vv.Codepage != 22222 {
		t.Fatalf("partial header detected codepage = %d, want CP866 or the equivalent system OEM alias", vv.Codepage)
	}
	if vv.HexMode {
		t.Fatal("partial header was misclassified as binary")
	}
}

func TestViewerView_Codepages_KeyBarLabel(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "dummy_view.txt")
	if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()

	vv.Codepage = 65001 // UTF-8
	labels := vv.GetKeyLabels()
	if labels.Normal[7] != "ANSI" {
		t.Errorf("Expected F8 KeyBar label to be 'ANSI' for UTF-8 viewer, got %q", labels.Normal[7])
	}

	vv.Codepage = 22222 // OEM
	labels = vv.GetKeyLabels()
	if labels.Normal[7] != "UTF-8" {
		t.Errorf("Expected F8 KeyBar label to be 'UTF-8' for OEM viewer, got %q", labels.Normal[7])
	}
}
func TestViewerView_Codepages_MultipleSwitchNoCrash(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "switch_test.txt")
	if err := os.WriteFile(path, []byte("Test content for codepage switch"), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}

	// Switch codepages twice (F8, F8)
	vv.ReloadWithCodepage(11111)
	vv.ReloadWithCodepage(22222)
	vv.ReloadWithCodepage(65001)

	// Close viewer (F3 exit)
	vv.Close()

	if !vv.IsDone() {
		t.Error("ViewerView failed to close cleanly after codepage switches")
	}
}

func (f *largeBinaryFile) readRanges() []trackedReadRange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]trackedReadRange(nil), f.reads...)
}

func TestViewerFirstWindowReusesDetectionHeader(t *testing.T) {
	file := &largeBinaryFile{size: 300 * 1024 * 1024}
	viewer, err := NewViewerView(context.Background(),
		&singleFileVFS{VFS: vfs.NewOSVFS(t.TempDir()), File: file}, "large.7z")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	// The header has already been read to detect hex/text mode. Its bytes
	// cover the first viewport and overscan without another asynchronous read.
	viewer.SetPosition(0, 0, 119, 50)
	window := viewer.semanticWindow()
	if len(window.Rows) == 0 {
		t.Fatal("first window was empty")
	}
	if !strings.Contains(window.Rows[0].Text, "37 7A") {
		t.Fatalf("first window did not contain the already-read file header: %q", window.Rows[0].Text)
	}
	if reads := file.readRanges(); len(reads) != 1 {
		t.Fatalf("opening fetched the header more than once: %+v", reads)
	}
	if viewer.Backend.isFetching {
		t.Fatal("first viewport needlessly started an asynchronous read")
	}
}

func TestViewerView_PageDownKeepsFinalTextPageStable(t *testing.T) {
	vtui.SetDefaultPalette()

	for _, tc := range []struct {
		name       string
		wrap       bool
		lines      int
		want       int64
		nativeRows int
	}{
		{name: "wrapped-short-final-page", wrap: true, lines: 5, want: 4},
		{name: "unwrapped-short-final-page", wrap: false, lines: 5, want: 4},
		{name: "wrapped-full-final-page", wrap: true, lines: 10, want: 24},
		{name: "unwrapped-full-final-page", wrap: false, lines: 10, want: 24},
		{name: "native-wrapped-short-final-page", wrap: true, lines: 5, want: 4, nativeRows: 4},
		{name: "native-unwrapped-short-final-page", wrap: false, lines: 5, want: 4, nativeRows: 4},
		{name: "native-wrapped-full-final-page", wrap: true, lines: 10, want: 24, nativeRows: 4},
		{name: "native-unwrapped-full-final-page", wrap: false, lines: 10, want: 24, nativeRows: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var content strings.Builder
			for line := 1; line <= tc.lines; line++ {
				fmt.Fprintf(&content, "L%02d\n", line)
			}
			data := []byte(content.String())
			ctx, cancel := context.WithCancel(context.Background())
			backend := &ViewerBackend{
				File:      &vfs.MemoryReadAtCloser{Data: data},
				size:      int64(len(data)),
				cacheData: data,
				ctx:       ctx,
				cancelCtx: cancel,
			}
			vv := &ViewerView{Backend: backend, WrapMode: tc.wrap, NativeViewportRows: tc.nativeRows}
			defer vv.Close()

			terminalRows := 4
			if tc.nativeRows > 0 {
				// Native chrome leaves fewer complete rows than the terminal grid.
				terminalRows += 2
			}
			vv.SetPosition(0, 0, 39, terminalRows)
			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(40, terminalRows+1)

			render := func() {
				vv.Show(scr)
			}
			pageDown := func() {
				vv.ProcessKey(&vtinput.InputEvent{
					Type:           vtinput.KeyEventType,
					KeyDown:        true,
					VirtualKeyCode: vtinput.VK_NEXT,
				})
				render()
			}

			render()
			for press := 0; press < 4; press++ {
				pageDown()
			}

			if vv.TopOffset != tc.want {
				t.Fatalf("final top offset=%d, want %d", vv.TopOffset, tc.want)
			}
			if !vv.EofVisible {
				t.Fatal("final page should be marked as EOF-visible")
			}
			if len(vv.LineOffsets) != 4 {
				t.Fatalf("final page line offsets=%v, want four real rows", vv.LineOffsets)
			}
			if vv.LineOffsets[0] != tc.want || vv.LineOffsets[3] != int64((tc.lines-1)*4) {
				t.Fatalf("final page line offsets=%v, want first=%d last=%d",
					vv.LineOffsets, tc.want, (tc.lines-1)*4)
			}

			oldTop := vv.TopOffset
			pageDown()
			if vv.TopOffset != oldTop {
				t.Fatalf("extra PgDn moved final page from %d to %d", oldTop, vv.TopOffset)
			}
		})
	}
}

func TestViewerView_DecodeMode(t *testing.T) {
	vtui.SetDefaultPalette()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "code.bin")
	// Machine code:
	// 48 89 e5: mov rbp, rsp (3 bytes)
	// 48 83 ec 10: sub rsp, 0x10 (4 bytes)
	// b8 2a 00 00 00: mov eax, 42 (5 bytes)
	// c3: ret (1 byte)
	code := []byte{
		0x48, 0x89, 0xe5,
		0x48, 0x83, 0xec, 0x10,
		0xb8, 0x2a, 0x00, 0x00, 0x00,
		0xc3,
	}
	if err := os.WriteFile(path, code, 0644); err != nil {
		t.Fatalf("Failed to write fixture: %v", err)
	}

	v := vfs.NewOSVFS(tmpDir)
	vv, err := NewViewerView(context.Background(), v, path)
	if err != nil {
		t.Fatalf("Failed to create ViewerView: %v", err)
	}
	defer vv.Close()
	vtui.FrameManager.Push(vv)

	vv.HexMode = false
	vv.DecodeMode = false
	vv.TopOffset = 0

	// 1st F4: Hex Mode
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if !vv.HexMode || vv.DecodeMode {
		t.Fatalf("Expected HexMode after 1st F4, got HexMode=%v DecodeMode=%v", vv.HexMode, vv.DecodeMode)
	}

	// 2nd F4: Decode Mode
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if vv.HexMode || !vv.DecodeMode {
		t.Fatalf("Expected DecodeMode after 2nd F4, got HexMode=%v DecodeMode=%v", vv.HexMode, vv.DecodeMode)
	}

	if mode := vv.effectiveDisasmMode(); mode != 64 {
		t.Errorf("effectiveDisasmMode=%d, want 64", mode)
	}

	// Test semantic window
	vv.NativeViewportColumns = 80
	vv.NativeViewportRows = 10
	vv.SemanticLayoutRevision = 1
	window := vv.semanticWindow()
	if !window.Ready {
		t.Fatalf("semanticWindow not ready in DecodeMode")
	}
	if len(window.Rows) < 4 {
		t.Fatalf("Expected at least 4 instruction rows, got %d", len(window.Rows))
	}
	// Row 0: mov rbp, rsp (offset 0, len 3)
	if window.Rows[0].Offset != 0 || window.Rows[0].EndOffset != 3 {
		t.Errorf("Row 0 offsets: [%d, %d), want [0, 3)", window.Rows[0].Offset, window.Rows[0].EndOffset)
	}
	if !strings.Contains(window.Rows[0].Text, "mov") {
		t.Errorf("Row 0 text does not contain 'mov': %q", window.Rows[0].Text)
	}
	// Row 1: sub rsp, 0x10 (offset 3, len 4)
	if window.Rows[1].Offset != 3 || window.Rows[1].EndOffset != 7 {
		t.Errorf("Row 1 offsets: [%d, %d), want [3, 7)", window.Rows[1].Offset, window.Rows[1].EndOffset)
	}
	// Row 2: mov eax, 0x2a (offset 7, len 5)
	if window.Rows[2].Offset != 7 || window.Rows[2].EndOffset != 12 {
		t.Errorf("Row 2 offsets: [%d, %d), want [7, 12)", window.Rows[2].Offset, window.Rows[2].EndOffset)
	}
	// Row 3: ret (offset 12, len 1)
	if window.Rows[3].Offset != 12 || window.Rows[3].EndOffset != 13 {
		t.Errorf("Row 3 offsets: [%d, %d), want [12, 13)", window.Rows[3].Offset, window.Rows[3].EndOffset)
	}

	// Test navigation in DecodeMode:
	// Down from offset 0 -> offset 3
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if vv.TopOffset != 3 {
		t.Errorf("VK_DOWN TopOffset=%d, want 3", vv.TopOffset)
	}

	// Down from offset 3 -> offset 7
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if vv.TopOffset != 7 {
		t.Errorf("VK_DOWN TopOffset=%d, want 7", vv.TopOffset)
	}

	// Up from offset 7 -> offset 3
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if vv.TopOffset != 3 {
		t.Errorf("VK_UP TopOffset=%d, want 3", vv.TopOffset)
	}

	// Up from offset 3 -> offset 0
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if vv.TopOffset != 0 {
		t.Errorf("VK_UP TopOffset=%d, want 0", vv.TopOffset)
	}

	// 3rd F4: Back to Text Mode
	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if vv.HexMode || vv.DecodeMode {
		t.Fatalf("Expected TextMode after 3rd F4, got HexMode=%v DecodeMode=%v", vv.HexMode, vv.DecodeMode)
	}
}

func TestViewerView_DecodeModeAfterPageDown(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		fmt.Fprintf(&sb, "line %04d: this is some sample content for viewer testing %d\n", i, i*42)
	}
	data := []byte(sb.String())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &ViewerBackend{
		File:      &vfs.MemoryReadAtCloser{Data: data},
		size:      int64(len(data)),
		cacheData: data,
		ctx:       ctx,
		cancelCtx: cancel,
	}
	vv := &ViewerView{Backend: backend}
	defer vv.Close()
	vv.NativeViewportColumns = 80
	vv.NativeViewportRows = 20
	vv.SetPosition(0, 0, 80, 20)
	vtui.FrameManager.Push(vv)
	defer vtui.FrameManager.Pop()

	// Initial render populates lineOffsets
	vv.SemanticNode(nil)

	for i := 0; i < 5; i++ {
		pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
		vv.SemanticNode(nil)
	}
	t.Logf("After PgDn, TopOffset=%d", vv.TopOffset)

	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if !vv.HexMode || vv.DecodeMode {
		t.Fatalf("Expected HexMode after 1st F4, got HexMode=%v DecodeMode=%v", vv.HexMode, vv.DecodeMode)
	}
	t.Logf("After Hex F4, TopOffset=%d", vv.TopOffset)

	pressViewerKey(vv, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4})
	if vv.HexMode || !vv.DecodeMode {
		t.Fatalf("Expected DecodeMode after 2nd F4, got HexMode=%v DecodeMode=%v", vv.HexMode, vv.DecodeMode)
	}
	t.Logf("After Decode F4, TopOffset=%d", vv.TopOffset)

	window := vv.semanticWindow()
	t.Logf("semanticWindow: ready=%v, len(rows)=%d, viewportRow=%d, start=%d, end=%d",
		window.Ready, len(window.Rows), window.ViewportRow, window.Start, window.End)
	if !window.Ready {
		t.Fatalf("semanticWindow is NOT ready in DecodeMode after PgDn! viewportRow=%d", window.ViewportRow)
	}
}

// Mode layout is owned by the viewer; configurable F4 dispatch is tested by app.
func pressViewerKey(f vtui.Frame, e *vtinput.InputEvent) bool {
	if e.VirtualKeyCode == vtinput.VK_F4 {
		f.(*ViewerView).CycleViewMode()
		return true
	}
	return testutil.PressKey(f, e, nil)
}
