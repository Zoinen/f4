//go:build windows

package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/sys/windows"
)

// The shell fills the context menu of a plain file with its commands; only
// showing it needs a person, so the test stops before TrackPopupMenuEx.
func TestBuildExplorerMenuForAFile(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && err != windows.Errno(1) {
		t.Fatal(err)
	}
	defer windows.CoUninitialize()

	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := buildExplorerMenu(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer m.close()
	if n := m.itemCount(); n <= 0 {
		t.Fatalf("the context menu of %s has %d items", path, n)
	}
	// "Send to" and "Open with" are filled while they open, through the
	// IContextMenu2/3 of the menu; without one they stay empty.
	if m.drawer == nil {
		t.Fatal("the menu has neither IContextMenu3 nor IContextMenu2")
	}
	t.Logf("IContextMenu3: %v", m.v3)
	if _, err := registerExplorerMenuClass(); err != nil {
		t.Fatalf("the window class of the menu: %v", err)
	}
}
