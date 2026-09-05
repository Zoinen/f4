//go:build !windows

package vtui

import "sync"

func isWineOS() bool {
	return false
}

func hasConsoleBufferOS() bool {
	return false
}
func hasVTConsoleSupportOS() bool {
	return true
}
func setAltScreenWin32(enable bool)   {}
func getActiveConsoleHandle() uintptr { return 0 }

// win32ConsoleActive is always false off Windows: there is no separate
// Win32 console screen buffer to worry about, so the terminal is reached via
// VT sequences as usual. See win32_console_windows.go for the real check.
func win32ConsoleActive() bool { return false }

// Win32ConsoleRenderer is a fallback stub for non-Windows platforms.
type Win32ConsoleRenderer struct {
	mu sync.Mutex
}

func NewWin32ConsoleRenderer(parent *ScreenBuf) *Win32ConsoleRenderer {
	return &Win32ConsoleRenderer{}
}

func (r *Win32ConsoleRenderer) SetPalette(pal *[256]uint32)                               {}
func (r *Win32ConsoleRenderer) SetCursor(x, y int, visible bool, shape CursorShape)       {}
func (r *Win32ConsoleRenderer) SetWindowTitle(title string)                               {}
func (r *Win32ConsoleRenderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {}
func (r *Win32ConsoleRenderer) Flush()                                                    {}
