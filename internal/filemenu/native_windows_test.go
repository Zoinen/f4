package filemenu

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zzl/go-win32api/v2/win32"
)

func TestNativeWindowsPreparedMenu(t *testing.T) {
	if os.Getenv("F4_FILE_MENU_NATIVE_SMOKE") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	path := filepath.Join(t.TempDir(), "prepared.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := newShellSession()
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	original := selectShellCommand
	defer func() { selectShellCommand = original }()
	selectShellCommand = func(_ *win32.IContextMenu, _ win32.HMENU, _ win32.POINT, _ win32.HWND) uint32 { return 0 }
	request := Request{Paths: []string{path}, Operation: "prepare"}
	start := time.Now()
	if result := s.run(request); result.Outcome != Cancelled {
		t.Fatal(result)
	}
	t.Logf("Background preparation: %v", time.Since(start))
	prepared := s.prepared
	request.Operation = ""
	for i := 0; i < 3; i++ {
		start = time.Now()
		if result := s.run(request); result.Outcome != Cancelled {
			t.Fatal(result)
		}
		t.Logf("Prepared menu ready: %v", time.Since(start))
		if s.prepared != prepared {
			t.Fatal("unchanged selection rebuilt the shell menu")
		}
	}
	if err := os.WriteFile(path, []byte("changed length"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := s.run(request); result.Outcome != Cancelled {
		t.Fatal(result)
	}
	if s.prepared == prepared {
		t.Fatal("changed file reused stale shell menu")
	}
	selectShellCommand = func(_ *win32.IContextMenu, _ win32.HMENU, _ win32.POINT, _ win32.HWND) uint32 {
		if err := os.Remove(path); err != nil {
			t.Error(err)
		}
		return 1
	}
	if result := s.run(request); result.Outcome != Failed {
		t.Fatalf("stale command was not rejected: %+v", result)
	}
	if s.prepared != nil {
		t.Fatal("invocation retained the prepared menu")
	}
}

func TestNativeWindowsProperties(t *testing.T) {
	if os.Getenv("F4_FILE_MENU_NATIVE_SMOKE") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	path := filepath.Join(t.TempDir(), "Properties юникод & test.txt")
	if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
		t.Fatal(err)
	}
	original := selectShellCommand
	defer func() { selectShellCommand = original }()
	selectShellCommand = func(menu *win32.IContextMenu, popup win32.HMENU, point win32.POINT, hwnd win32.HWND) uint32 {
		count, _ := win32.GetMenuItemCount(popup)
		for i := int32(0); i < count; i++ {
			id := win32.GetMenuItemID(popup, i)
			if id == 0 || id == ^uint32(0) {
				continue
			}
			var verb [256]byte
			if menu.GetCommandString(uintptr(id-1), 0, nil, &verb[0], uint32(len(verb))) >= 0 && string(verb[:10]) == "properties" {
				return id
			}
		}
		return 0
	}
	var observed, returned atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		callback := syscall.NewCallback(func(hwnd win32.HWND, _ win32.LPARAM) uintptr {
			var pid uint32
			win32.GetWindowThreadProcessId(hwnd, &pid)
			var class [64]uint16
			win32.GetClassNameW(hwnd, &class[0], int32(len(class)))
			if pid == uint32(os.Getpid()) && syscall.UTF16ToString(class[:]) == "#32770" && win32.IsWindowVisible(hwnd) != 0 {
				// The invoking helper must remain alive while the sheet is visible.
				time.Sleep(300 * time.Millisecond)
				observed.Store(!returned.Load() && win32.IsWindow(hwnd) != 0)
				win32.PostMessageW(hwnd, win32.WM_COMMAND, 2, 0)
			}
			return 1
		})
		for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline) && !observed.Load(); {
			win32.EnumWindows(callback, 0)
			time.Sleep(20 * time.Millisecond)
		}
	}()
	result := showNative(Request{Paths: []string{path}})
	returned.Store(true)
	<-done
	if result.Outcome != Invoked || !observed.Load() {
		t.Fatalf("Properties did not remain open until dismissed: result=%+v observed=%v", result, observed.Load())
	}
}

// Opt-in: displays real shell menus and cancels them without selecting a verb.
func TestNativeWindowsMenuSmoke(t *testing.T) {
	if os.Getenv("F4_FILE_MENU_NATIVE_SMOKE") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "юникод & one.txt"), filepath.Join(dir, "two.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, selection := range [][]string{{paths[0]}, paths, {dir}} {
		done := make(chan struct{})
		stopped := make(chan struct{})
		go func() {
			defer close(stopped)
			cancelMenu := syscall.NewCallback(func(hwnd win32.HWND, _ win32.LPARAM) uintptr {
				var pid uint32
				win32.GetWindowThreadProcessId(hwnd, &pid)
				if pid == uint32(os.Getpid()) {
					var class [64]uint16
					win32.GetClassNameW(hwnd, &class[0], int32(len(class)))
					if syscall.UTF16ToString(class[:]) == "F4FileMenuOwner" {
						win32.PostMessageW(hwnd, win32.WM_CANCELMODE, 0, 0)
					}
				}
				return 1
			})
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					win32.EnumWindows(cancelMenu, 0)
				}
			}
		}()
		result := showNative(Request{Paths: selection})
		close(done)
		<-stopped
		if result.Outcome != Cancelled {
			t.Fatalf("selection %v: %+v", selection, result)
		}
	}
}
