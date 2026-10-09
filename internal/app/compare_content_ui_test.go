package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestReadTextFileForCompareContentVariants exercises every branch of
// readTextFileForCompare (f4#613): trailing/absent newline handling, the
// empty-file shortcut, the binary-file and oversized-file refusals, and the
// plain os.Stat error paths (missing file, directory).
func TestReadTextFileForCompareContentVariants(t *testing.T) {
	dir := t.TempDir()

	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("trailing newline is dropped", func(t *testing.T) {
		path := write("trailing.txt", []byte("first\nsecond\nthird\n"))
		lines, err := readTextFileForCompare(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"first", "second", "third"}; !reflect.DeepEqual(lines, want) {
			t.Fatalf("lines = %#v, want %#v", lines, want)
		}
	})

	t.Run("missing trailing newline keeps the last line", func(t *testing.T) {
		path := write("notrailing.txt", []byte("only line"))
		lines, err := readTextFileForCompare(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := []string{"only line"}; !reflect.DeepEqual(lines, want) {
			t.Fatalf("lines = %#v, want %#v", lines, want)
		}
	})

	t.Run("empty file yields no lines and no error", func(t *testing.T) {
		path := write("empty.txt", nil)
		lines, err := readTextFileForCompare(path)
		if err != nil || lines != nil {
			t.Fatalf("empty file = %#v, %v, want nil, nil", lines, err)
		}
	})

	t.Run("a NUL byte is refused as binary", func(t *testing.T) {
		path := write("binary.bin", []byte("abc\x00def"))
		lines, err := readTextFileForCompare(path)
		if lines != nil || err == nil || !strings.Contains(err.Error(), "binary") {
			t.Fatalf("binary file = %#v, %v, want a binary-file error", lines, err)
		}
	})

	t.Run("a directory is refused", func(t *testing.T) {
		sub := filepath.Join(dir, "subdir")
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatal(err)
		}
		lines, err := readTextFileForCompare(sub)
		if lines != nil || err == nil || !strings.Contains(err.Error(), "directory") {
			t.Fatalf("directory target = %#v, %v, want a directory error", lines, err)
		}
	})

	t.Run("a missing file surfaces the stat error", func(t *testing.T) {
		lines, err := readTextFileForCompare(filepath.Join(dir, "does-not-exist.txt"))
		if lines != nil || err == nil || !os.IsNotExist(err) {
			t.Fatalf("missing file = %#v, %v, want an os.IsNotExist error", lines, err)
		}
	})

	t.Run("a file over the size cap is refused without being read", func(t *testing.T) {
		path := filepath.Join(dir, "huge.bin")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		// Truncate creates a sparse file of the wanted size without writing
		// (and without allocating) the actual bytes, so this stays cheap even
		// though the size itself is 8 MiB+1.
		if err := f.Truncate(compareContentMaxFileSize + 1); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		lines, err := readTextFileForCompare(path)
		if lines != nil || err == nil || !strings.Contains(err.Error(), "larger than") {
			t.Fatalf("oversized file = %#v, %v, want a size-limit error", lines, err)
		}
	})
}

// TestSingleLocalFileTargetGuards drives singleLocalFileTarget through its
// guard clauses in order: a nil panel, a non-local VFS, a directory under the
// cursor, more than one item selected, and finally the single-regular-file
// shape the function exists to recognize.
func TestSingleLocalFileTargetGuards(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if path, title, ok := singleLocalFileTarget(nil); ok || path != "" || title != "" {
		t.Fatalf("nil panel = (%q, %q, %v), want (\"\", \"\", false)", path, title, ok)
	}

	remote := panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewNullVFS(0))
	paneltest.WaitForLoad(t, remote)
	if _, _, ok := singleLocalFileTarget(remote); ok {
		t.Fatal("a non-local VFS reported a comparable target")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	local := panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(dir))
	paneltest.WaitForLoad(t, local)

	local.SelectName("sub")
	if _, _, ok := singleLocalFileTarget(local); ok {
		t.Fatal("a directory under the cursor reported a comparable target")
	}

	var fileIdx, subIdx = -1, -1
	for i, e := range local.Entries {
		switch e.Name {
		case "file.txt":
			fileIdx = i
		case "sub":
			subIdx = i
		}
	}
	if fileIdx < 0 || subIdx < 0 {
		t.Fatalf("expected panel entries for file.txt and sub, got %#v", local.Entries)
	}
	local.SetItemSelected(fileIdx, true)
	local.SetItemSelected(subIdx, true)
	if _, _, ok := singleLocalFileTarget(local); ok {
		t.Fatal("two marked entries reported a comparable target")
	}
	local.SetItemSelected(fileIdx, false)
	local.SetItemSelected(subIdx, false)

	local.SelectName("file.txt")
	path, title, ok := singleLocalFileTarget(local)
	if !ok {
		t.Fatal("a single regular file was not reported as a comparable target")
	}
	if want := local.Vfs.Join(dir, "file.txt"); path != want || title != "file.txt" {
		t.Fatalf("target = (%q, %q), want (%q, %q)", path, title, want, "file.txt")
	}
}

// setupComparePanels wires two file panels into a panels frame pushed on the
// frame manager, the shape panelCanCompareFilesByContent and
// actionCompareFilesByContent both read through panel.FindPanelsFrameAnyScreen
// / pf.GetActivePanel/GetInactivePanel.
func setupComparePanels(t *testing.T, activeDir, inactiveDir string) *panel.PanelsFrame {
	t.Helper()
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)
	active := panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(activeDir))
	inactive := panel.NewFileSystemPanel(40, 0, 40, 20, vfs.NewOSVFS(inactiveDir))
	paneltest.WaitForLoad(t, active)
	paneltest.WaitForLoad(t, inactive)
	// PanelsFrame.Active()/Passive() are keyed off ActiveIdx (NewPanelsFrame
	// defaults it to 1), so Panels[1] is what GetActivePanel returns.
	pf.Panels[1] = active
	pf.Panels[0] = inactive
	pf.ResizeConsole(120, 40)
	vtui.FrameManager.Push(pf)
	return pf
}

// TestPanelCanCompareFilesByContentGuards mirrors
// TestCompareFoldersGuards' shape for the sibling "compare by content"
// feature: no panels frame at all, a mismatched selection, and finally the
// single-file-in-both-panels shape that lets the menu entry show up.
func TestPanelCanCompareFilesByContentGuards(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	if panelCanCompareFilesByContent() {
		t.Fatal("an empty frame manager reported comparable panels")
	}
}

func TestPanelCanCompareFilesByContentWithPanels(t *testing.T) {
	activeDir, inactiveDir := t.TempDir(), t.TempDir()
	for _, dir := range []string{activeDir, inactiveDir} {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Created before the panel below loads its directory, so "sub" is already
	// part of its entries by the time SelectName looks for it.
	if err := os.Mkdir(filepath.Join(inactiveDir, "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	pf := setupComparePanels(t, activeDir, inactiveDir)

	pf.GetActivePanel().SelectName("a.txt")
	pf.GetInactivePanel().SelectName("a.txt")
	if !panelCanCompareFilesByContent() {
		t.Fatal("two single-file panels were not reported as comparable")
	}

	pf.GetInactivePanel().SelectName("sub")
	if panelCanCompareFilesByContent() {
		t.Fatal("a directory in the inactive panel was still reported as comparable")
	}
}

// TestActionCompareFilesByContentGuardsUpFront checks that
// actionCompareFilesByContent returns before starting the background read
// (no task is ever posted) when either panel's selection is not a single
// local file.
func TestActionCompareFilesByContentGuardsUpFront(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	otherDir := t.TempDir()
	pf := setupComparePanels(t, dir, otherDir)

	pf.GetActivePanel().SelectName("a.txt")
	// The inactive panel's directory is empty: it has no regular file to fall
	// the cursor on, so singleLocalFileTarget refuses it either way (no
	// selection, or a ".." entry that is not a regular file).

	actionCompareFilesByContent(pf)

	select {
	case <-vtui.FrameManager.TaskChan:
		t.Fatal("compare-by-content posted a result despite an unusable inactive-panel selection")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestActionCompareFilesByContentEndToEnd runs the full path: two single-file
// panel selections, the async read in actionCompareFilesByContent, and the
// resulting diff view pushed by showCompareFilesByContentResult.
func TestActionCompareFilesByContentEndToEnd(t *testing.T) {
	leftDir, rightDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(leftDir, "a.txt"), []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rightDir, "a.txt"), []byte("one\nTWO\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pf := setupComparePanels(t, leftDir, rightDir)

	pf.GetActivePanel().SelectName("a.txt")
	pf.GetInactivePanel().SelectName("a.txt")

	actionCompareFilesByContent(pf)

	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(5 * time.Second):
		t.Fatal("compare-by-content did not post its result in time")
	}

	dv, ok := vtui.FrameManager.GetTopFrame().(*diffview.DiffView)
	if !ok {
		t.Fatalf("top frame after compare = %T, want a diff view", vtui.FrameManager.GetTopFrame())
	}
	if dv.LeftTitle != "a.txt" || dv.RightTitle != "a.txt" {
		t.Fatalf("diff view titles = %q / %q, want a.txt / a.txt", dv.LeftTitle, dv.RightTitle)
	}
}

// TestShowCompareFilesByContentResultErrorsAndTooLarge covers the three
// non-happy branches of showCompareFilesByContentResult directly: a read
// error on either side, and textdiff's own size cap (MaxLines) tripping
// NewDiffView's ErrTooLarge path.
func TestShowCompareFilesByContentResultErrorsAndTooLarge(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := &panel.PanelsFrame{}

	showCompareFilesByContentResult(pf, "left.txt", "right.txt", "/no/left.txt", "/no/right.txt", nil, nil, errors.New("boom"), nil)
	msg := vtui.FrameManager.GetTopFrame()
	if msg == nil {
		t.Fatal("a left-side read error did not show a message")
	}
	msg.Close()
	vtui.FrameManager.RemoveFrame(msg)

	showCompareFilesByContentResult(pf, "left.txt", "right.txt", "/no/left.txt", "/no/right.txt", nil, nil, nil, errors.New("boom"))
	msg = vtui.FrameManager.GetTopFrame()
	if msg == nil {
		t.Fatal("a right-side read error did not show a message")
	}
	msg.Close()
	vtui.FrameManager.RemoveFrame(msg)

	// textdiff.MaxLines is 20000 combined lines; two 15000-line inputs trip
	// ErrTooLarge before any content comparison happens.
	huge := make([]string, 15000)
	for i := range huge {
		huge[i] = fmt.Sprintf("line-%d", i)
	}
	showCompareFilesByContentResult(pf, "left.txt", "right.txt", "/no/left.txt", "/no/right.txt", huge, huge, nil, nil)
	msg = vtui.FrameManager.GetTopFrame()
	if msg == nil {
		t.Fatal("an oversized comparison did not show a message")
	}
	if _, ok := msg.(*diffview.DiffView); ok {
		t.Fatal("an oversized comparison opened a diff view instead of a message")
	}
}
