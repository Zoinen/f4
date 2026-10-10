package panel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// newIssue1804QuickView previews a file holding content in a focused quick
// view 40 columns wide, after one render has laid the text out.
func newIssue1804QuickView(t *testing.T, content string) (*QuickViewPanel, *vtui.ScreenBuf) {
	t.Helper()
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	vtui.SetDefaultPalette()

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "notes.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(tmp))
	waitForLoad(t, fsp)
	fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "notes.txt", Size: int64(len(content))}}}
	fsp.CursorIdx = 0

	q := NewQuickViewPanel(fsp)
	q.SetPosition(0, 0, 39, 20)
	q.Show(scr)
	q.SetFocus(true)
	return q, scr
}

func issue1804Key(q *QuickViewPanel, vk uint16, shift bool) bool {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
	if shift {
		e.ControlKeyState = vtinput.ShiftPressed
	}
	return q.ProcessKey(e)
}

// C in a focused quick view copies the line under the cursor, the way the
// info panel copies its current row.
func TestIssue1804_QuickViewCopiesCursorLine(t *testing.T) {
	q, scr := newIssue1804QuickView(t, "alpha\nbeta\ngamma\n")
	issue1804Key(q, vtinput.VK_DOWN, false)
	q.Show(scr)
	if !issue1804Key(q, vtinput.VK_C, false) {
		t.Fatal("C must be consumed by a focused quick view")
	}
	if got := vtui.GetClipboard(); got != "beta" {
		t.Fatalf("clipboard = %q, want the cursor line %q", got, "beta")
	}
	testutil.PumpUntilToastActive(t)
	testutil.WaitForToastExpiry(t, 3*time.Second)
}

// Shift+Down and Ins mark whole source lines — a wrapped line is one mark —
// and C copies the marks in file order.
func TestIssue1804_QuickViewMarksAndCopiesLines(t *testing.T) {
	long := strings.Repeat("x", 100) // wraps over several 38-cell rows
	q, scr := newIssue1804QuickView(t, "one\n"+long+"\nthree\nfour\n")

	issue1804Key(q, vtinput.VK_DOWN, true) // marks "one", moves onto the long line
	q.Show(scr)
	issue1804Key(q, vtinput.VK_DOWN, true) // marks the long line, steps past all of its rows
	q.Show(scr)
	if src, _ := q.cursorSource(); src != 2 {
		t.Fatalf("Shift+Down over a wrapped line left the cursor on source %d, want 2", src)
	}
	issue1804Key(q, vtinput.VK_DOWN, false) // skip "three"
	q.Show(scr)
	issue1804Key(q, vtinput.VK_INSERT, false) // marks "four"
	q.Show(scr)

	issue1804Key(q, vtinput.VK_C, false)
	if got, want := vtui.GetClipboard(), "one\n"+long+"\nfour"; got != want {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
	testutil.PumpUntilToastActive(t)
	testutil.WaitForToastExpiry(t, 3*time.Second)

	// Shift+Up on a marked line unmarks it.
	issue1804Key(q, vtinput.VK_UP, true)
	if q.marks[3] {
		t.Fatal("Shift+Up on a marked line should unmark it")
	}

	// Marks survive F2: they belong to source lines, not screen rows.
	issue1804Key(q, vtinput.VK_F2, false)
	q.Show(scr)
	if !q.marks[0] || !q.marks[1] {
		t.Fatalf("wrap toggle dropped the marks: %v", q.marks)
	}
	// F4 shows other lines altogether, so the marks go.
	issue1804Key(q, vtinput.VK_F4, false)
	if len(q.marks) != 0 {
		t.Fatalf("hex toggle kept marks %v pointing into the text", q.marks)
	}
}

// The cursor is drawn only while the panel has the focus, marks always.
func TestIssue1804_QuickViewCursorAndMarkColours(t *testing.T) {
	q, scr := newIssue1804QuickView(t, "a\nb\nc\n")
	issue1804Key(q, vtinput.VK_INSERT, false)
	q.Show(scr)
	if _, special := q.lineAttr(1); !special {
		t.Fatal("focused cursor row must be highlighted")
	}
	if _, special := q.lineAttr(0); !special {
		t.Fatal("marked row must be highlighted")
	}
	q.SetFocus(false)
	if _, special := q.lineAttr(1); special {
		t.Fatal("an unfocused quick view must not draw its cursor")
	}
	if issue1804Key(q, vtinput.VK_C, false) {
		t.Fatal("an unfocused quick view must leave C to the command line")
	}
}

// The folder summary walks like the info panel: the cursor skips the
// spacers and the heading, Shift+arrows and Ins mark rows, and C copies the
// value under the cursor or the marked rows as "label: value".
func TestIssue1804_QuickViewFolderRowsMarkAndCopy(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.SetDefaultPalette()
	q := &QuickViewPanel{cacheValid: true, cacheDir: true, scanDone: true, Focused: true}
	q.scanStats.Dirs = 3 // the folder itself plus two children
	q.scanStats.Files = 7
	q.clearMarks()
	q.dirRows = q.dirSummary("src")

	copied := func() string {
		t.Helper()
		issue1804Key(q, vtinput.VK_C, false)
		testutil.PumpUntilToastActive(t)
		testutil.WaitForToastExpiry(t, 3*time.Second)
		return vtui.GetClipboard()
	}

	if got := copied(); got != "src" {
		t.Fatalf("C on the first row copied %q, want the folder name", got)
	}
	issue1804Key(q, vtinput.VK_DOWN, false) // over the spacers and "Contains:"
	if got := copied(); got != "2" {
		t.Fatalf("C on Folders copied %q, want 2", got)
	}
	issue1804Key(q, vtinput.VK_DOWN, true) // marks Folders
	issue1804Key(q, vtinput.VK_INSERT, false)
	if got, want := copied(), "Folders: 2\nFiles: 7"; got != want {
		t.Fatalf("marked rows copied %q, want %q", got, want)
	}
	if _, cursorOnFilesSize := q.dirMarks["Files size"]; cursorOnFilesSize || q.dirRows[q.dirCursor].label != "Files size" {
		t.Fatalf("Ins should leave the cursor on Files size, got %q", q.dirRows[q.dirCursor].label)
	}
	if q.dirRowAttr(q.dirCursor, 0) == 0 || q.dirRowAttr(4, 0) == 0 || q.dirRowAttr(2, 0) != 0 {
		t.Fatal("cursor and marked rows must be coloured, the heading must not")
	}

	issue1804Key(q, vtinput.VK_HOME, false)
	if q.dirCursor != 0 {
		t.Fatalf("Home left the cursor on row %d", q.dirCursor)
	}
	issue1804Key(q, vtinput.VK_END, false)
	if q.dirRows[q.dirCursor].label != "Files size" {
		t.Fatalf("End left the cursor on %q", q.dirRows[q.dirCursor].label)
	}
}

// For a picture, C hands the file to the image clipboard; where there is
// none, the path is copied as text instead.
func TestIssue1804_QuickViewCopiesImage(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fsp)
	path := filepath.Join(fsp.Vfs.GetPath(), "pic.png")
	q := &QuickViewPanel{src: fsp, cacheValid: true, cacheImage: true, cachePath: path}

	old := quickViewCopyImage
	t.Cleanup(func() { quickViewCopyImage = old })
	calls := make(chan string, 2)
	quickViewCopyImage = func(_ context.Context, _ vfs.VFS, p string) error {
		calls <- p
		return errors.New("no image clipboard here")
	}

	vtui.SetClipboard("before")
	q.copyCurrent()
	select {
	case got := <-calls:
		if got != path {
			t.Fatalf("image copy got path %q, want %q", got, path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("C on a picture never reached the image clipboard")
	}
	deadline := time.Now().Add(5 * time.Second)
	for vtui.GetClipboard() != path {
		if time.Now().After(deadline) {
			t.Fatalf("clipboard = %q, want the path fallback %q", vtui.GetClipboard(), path)
		}
		testutil.DrainUITasks()
		time.Sleep(10 * time.Millisecond)
	}
	testutil.PumpUntilToastActive(t)
	testutil.WaitForToastExpiry(t, 4*time.Second)
}

// B toggles byte units only while the info or quick view panel holds the
// focus; with the file panel focused, B is command-line text (#1804 item c).
func TestIssue1804_AltPanelFocusedCondition(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)
	pf.ShowPanels = true
	pf.ActiveIdx = 0

	q := NewQuickViewPanel(pf.Panels[1].(*FileSystemPanel))
	pf.AltPanels[1] = q
	if !keymap.ConditionTrue("AltPanelVisible") {
		t.Fatal("precondition: the quick view is visible")
	}
	if keymap.ConditionTrue("AltPanelFocused") {
		t.Fatal("AltPanelFocused must be false while the file panel has the focus")
	}

	pf.ActiveIdx = 1
	if !keymap.ConditionTrue("AltPanelFocused") {
		t.Fatal("AltPanelFocused must be true once the quick view has the focus")
	}

	pf.CommandLineFocused = true
	if keymap.ConditionTrue("AltPanelFocused") {
		t.Fatal("AltPanelFocused must be false while the command line has the focus")
	}
}
