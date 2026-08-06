package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// waitForMarkedClipboard polls vtui.GetClipboard until it matches want or
// the deadline expires. Panel.CopySelected* actions dispatch the actual
// SetClipboard on a goroutine (SetClipboard can block up to ~4s on far2l
// IPC and while shelling out to xclip/wl-copy), so the assertion has to
// tolerate a small delay.
func waitForMarkedClipboard(t *testing.T, want string) string {
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

// seedMarkedPanel wires up a PanelsFrame whose active panel has the given
// entries at the given path, with the first `markCount` entries pre-
// marked. The frame is pushed onto FrameManager so withPF handlers find it.
func seedMarkedPanel(t *testing.T, path string, names []string, markCount int) *PanelsFrame {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame()
	t.Cleanup(func() { pf.Close() })
	pf.ResizeConsole(80, 25)

	fsp := pf.getActivePanel()
	fsp.vfs = vfs.NewOSVFS(path)
	fsp.vfs.SetPath(path)

	entries := []*fileEntry{{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	for i, n := range names {
		e := &fileEntry{VFSItem: vfs.VFSItem{Name: n}}
		if i < markCount {
			e.Selected = true
		}
		entries = append(entries, e)
	}
	fsp.entries = entries
	fsp.Refresh()

	vtui.FrameManager.Push(pf)
	return pf
}

func TestAction_PanelCopySelectedNames(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt", "c.txt"} {
		if err := os.WriteFile(filepath.Join(tmp, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	seedMarkedPanel(t, tmp, []string{"a.txt", "b.txt", "c.txt"}, 2)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedNames") {
		t.Fatal("Panel.CopySelectedNames did not run")
	}
	want := "a.txt\nb.txt"
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}
}

func TestAction_PanelCopySelectedNames_NoMarkedFallsBackToCursor(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(tmp, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	pf := seedMarkedPanel(t, tmp, []string{"a.txt", "b.txt"}, 0)
	fsp := pf.getActivePanel()
	// Cursor at index 2 → "b.txt"
	fsp.SetCursorIndex(2)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedNames") {
		t.Fatal("Panel.CopySelectedNames did not run")
	}
	want := "b.txt"
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("fallback clipboard = %q, want %q", got, want)
	}
}

func TestAction_PanelCopySelectedPaths(t *testing.T) {
	tmp := t.TempDir()
	for _, n := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(tmp, n), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	seedMarkedPanel(t, tmp, []string{"a.txt", "b.txt"}, 2)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedPaths") {
		t.Fatal("Panel.CopySelectedPaths did not run")
	}
	want := strings.Join([]string{
		filepath.Join(tmp, "a.txt"),
		filepath.Join(tmp, "b.txt"),
	}, "\n")
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}
}

func TestAction_PanelCopySelectedPaths_CursorOnParentUsesCurrentDir(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	pf := seedMarkedPanel(t, tmp, []string{"a.txt"}, 0)
	fsp := pf.getActivePanel()
	// Cursor at index 0 → ".." (par far2l docs: acts as the current folder).
	fsp.SetCursorIndex(0)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedPaths") {
		t.Fatal("Panel.CopySelectedPaths did not run")
	}
	want := tmp
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("cursor-on-.. clipboard = %q, want %q", got, want)
	}
}

func TestAction_PanelCopySelectedRealPaths_CursorOnParentUsesCurrentDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation on Windows requires elevated permissions")
	}
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	linkDir := filepath.Join(tmp, "link")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}
	// Panel opened via the symlink; cursor on ".." must yield the RESOLVED
	// path of the current folder, not just the symlinked one.
	pf := seedMarkedPanel(t, linkDir, nil, 0)
	fsp := pf.getActivePanel()
	fsp.SetCursorIndex(0)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedRealPaths") {
		t.Fatal("Panel.CopySelectedRealPaths did not run")
	}
	want, err := filepath.EvalSymlinks(linkDir)
	if err != nil {
		t.Fatalf("test setup: EvalSymlinks: %v", err)
	}
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("cursor-on-.. resolved clipboard = %q, want %q", got, want)
	}
}

func TestAction_PanelCopySelectedRealPaths_ResolvesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation on Windows requires elevated permissions")
	}
	tmp := t.TempDir()
	realDir := filepath.Join(tmp, "real")
	linkDir := filepath.Join(tmp, "link")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realDir, "target.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}

	// Panel is opened via the symlink; the file's full path therefore
	// contains "link/target.txt". EvalSymlinks should collapse "link"
	// back to "real".
	seedMarkedPanel(t, linkDir, []string{"target.txt"}, 1)
	vtui.SetClipboard("")

	if !RunAction("Panel.CopySelectedRealPaths") {
		t.Fatal("Panel.CopySelectedRealPaths did not run")
	}
	want, err := filepath.EvalSymlinks(filepath.Join(linkDir, "target.txt"))
	if err != nil {
		t.Fatalf("test setup: EvalSymlinks: %v", err)
	}
	if got := waitForMarkedClipboard(t, want); got != want {
		t.Errorf("clipboard = %q, want %q", got, want)
	}
	if !strings.Contains(want, "real") {
		t.Fatalf("test invariant: expected resolved path to contain %q, got %q", "real", want)
	}
}

func TestHotkeyManager_MarkedClipboardDefaults_Issue289(t *testing.T) {
	hm := NewHotkeyManager("")
	hm.initDefaults()

	cases := []struct {
		key      string
		expected string
	}{
		{"CtrlShiftIns", "Panel.CopySelectedNames"},
		{"AltShiftIns", "Panel.CopySelectedPaths"},
		{"CtrlAltIns", "Panel.CopySelectedRealPaths"},
	}
	for _, tc := range cases {
		if got := hm.GetAction("Shell", tc.key); got != tc.expected {
			t.Errorf("Shell/%s: expected %q, got %q", tc.key, tc.expected, got)
		}
	}
}
