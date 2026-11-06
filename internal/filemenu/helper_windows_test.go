package filemenu

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/zzl/go-win32api/v2/win32"
)

func TestNativeWindowsHelperChild(t *testing.T) {
	mode := os.Getenv("F4_NATIVE_HELPER_CHILD")
	if mode == "" {
		return
	}
	if mode == "popup" {
		os.Exit(Serve(os.Stdin, os.Stdout))
	}
	selectShellCommand = func(menu *win32.IContextMenu, popup win32.HMENU, _ win32.POINT, _ win32.HWND) uint32 {
		if mode != "properties" {
			return 0
		}
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
	os.Exit(Serve(os.Stdin, os.Stdout))
}

func TestNativeWindowsPersistentHelper(t *testing.T) {
	if os.Getenv("F4_FILE_MENU_NATIVE_SMOKE") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	for _, mode := range []string{"cancel", "properties", "popup"} {
		t.Run(mode, func(t *testing.T) {
			c := &helperClient{command: func(ctx context.Context) (*exec.Cmd, error) {
				if exe := os.Getenv("F4_NATIVE_MENU_EXE"); exe != "" && mode == "popup" {
					return exec.CommandContext(ctx, exe, HelperFlag), nil
				}
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNativeWindowsHelperChild$")
				cmd.Env = append(os.Environ(), "F4_NATIVE_HELPER_CHILD="+mode)
				return cmd, nil
			}}
			defer c.close()
			path := filepath.Join(t.TempDir(), "persistent properties.txt")
			if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			r := Request{Paths: []string{path}, Operation: "prepare"}
			if result := c.run(ctx, r); result.Outcome != Cancelled {
				t.Fatal(result)
			}
			p := c.process
			r.Operation = ""
			if mode == "popup" {
				r.Position = Point{X: 100, Y: 150, Valid: true}
			}
			start := time.Now()
			var visible chan time.Duration
			var background win32.COLORREF
			var popupRect win32.RECT
			if mode == "popup" {
				visible = make(chan time.Duration, 1)
				go func() {
					var owner win32.HWND
					var popup win32.HWND
					shown := false
					find := syscall.NewCallback(func(hwnd win32.HWND, _ win32.LPARAM) uintptr {
						var pid uint32
						win32.GetWindowThreadProcessId(hwnd, &pid)
						if pid != uint32(p.cmd.Process.Pid) {
							return 1
						}
						var class [64]uint16
						win32.GetClassNameW(hwnd, &class[0], int32(len(class)))
						switch syscall.UTF16ToString(class[:]) {
						case "F4FileMenuOwner":
							owner = hwnd
						case "#32768":
							shown = win32.IsWindowVisible(hwnd) != 0
							popup = hwnd
						}
						return 1
					})
					for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
						win32.EnumWindows(find, 0)
						if shown && owner != 0 {
							win32.GetWindowRect(popup, &popupRect)
							elapsed := time.Since(start)
							time.Sleep(50 * time.Millisecond)
							var rect win32.RECT
							win32.GetClientRect(popup, &rect)
							dc := win32.GetDC(popup)
							background = win32.GetPixel(dc, rect.Right-30, 20)
							win32.ReleaseDC(popup, dc)
							visible <- elapsed
							win32.PostMessageW(owner, win32.WM_CANCELMODE, 0, 0)
							return
						}
						time.Sleep(time.Millisecond)
					}
					visible <- 0
					cancel()
				}()
			}
			result := c.run(ctx, r)
			t.Logf("Prepared request round trip: %v", time.Since(start))
			if mode == "popup" {
				elapsed := <-visible
				if elapsed == 0 || result.Outcome != Cancelled {
					t.Fatalf("native popup did not appear: %+v", result)
				}
				t.Logf("Actual native popup visible after: %v", elapsed)
				if popupRect.Left < 84 || popupRect.Left > 116 || popupRect.Top < 134 || popupRect.Top > 166 {
					t.Errorf("popup ignored desktop anchor: %+v", popupRect)
				}
				t.Logf("Native popup background COLORREF: #%06x", background)
				if os.Getenv("F4_NATIVE_MENU_EXPECT_DARK") == "1" && (background&255 > 100 || (background>>8)&255 > 100 || (background>>16)&255 > 100) {
					t.Errorf("expected dark menu background, got #%06x", background)
				}
			} else if mode == "cancel" {
				if result.Outcome != Cancelled {
					t.Fatal(result)
				}
				for i := 0; i < 3; i++ {
					start = time.Now()
					if result = c.run(ctx, r); result.Outcome != Cancelled {
						t.Fatal(result)
					}
					t.Logf("Repeated request round trip: %v", time.Since(start))
					if c.process != p {
						t.Fatal("helper restarted")
					}
				}
			} else {
				if result.Outcome != Invoked {
					t.Fatal(result)
				}
				var sheet win32.HWND
				find := syscall.NewCallback(func(hwnd win32.HWND, _ win32.LPARAM) uintptr {
					var pid uint32
					win32.GetWindowThreadProcessId(hwnd, &pid)
					var class [64]uint16
					win32.GetClassNameW(hwnd, &class[0], int32(len(class)))
					if pid == uint32(p.cmd.Process.Pid) && syscall.UTF16ToString(class[:]) == "#32770" && win32.IsWindowVisible(hwnd) != 0 {
						sheet = hwnd
					}
					return 1
				})
				for deadline := time.Now().Add(5 * time.Second); sheet == 0 && time.Now().Before(deadline); {
					win32.EnumWindows(find, 0)
					time.Sleep(20 * time.Millisecond)
				}
				if sheet == 0 {
					t.Fatal("Properties did not open after invocation returned")
				}
				time.Sleep(200 * time.Millisecond)
				if win32.IsWindow(sheet) == 0 {
					t.Fatal("Properties disappeared while helper was idle")
				}
				win32.PostMessageW(sheet, win32.WM_COMMAND, 2, 0)
				for deadline := time.Now().Add(3 * time.Second); win32.IsWindow(sheet) != 0 && time.Now().Before(deadline); {
					time.Sleep(20 * time.Millisecond)
				}
				if win32.IsWindow(sheet) != 0 {
					t.Fatal("Properties did not process dismissal")
				}
			}
			_ = p.in.Close()
			select {
			case <-p.done:
			case <-time.After(3 * time.Second):
				t.Fatal("helper survived parent pipe closure")
			}
		})
	}
}
