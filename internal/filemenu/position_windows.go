package filemenu

import (
	"os"

	"github.com/unxed/f4/internal/wincon"
	"github.com/zzl/go-win32api/v2/win32"
)

// PanelPoint translates a TUI cell boundary in the parent process, never in
// the detached helper. Unknown terminal-host geometry is deliberately rejected.
func PanelPoint(x, y, cols, rows int) Point {
	hwnd := win32.GetForegroundWindow()
	var pid uint32
	win32.GetWindowThreadProcessId(hwnd, &pid)
	if pid != uint32(os.Getpid()) {
		console, source := wincon.ConsoleWindow()
		if source != wincon.SourceConsole {
			return Point{}
		}
		hwnd = win32.HWND(console)
	}
	if hwnd == 0 || cols <= 0 || rows <= 0 {
		return Point{}
	}
	var rect win32.RECT
	if ok, _ := win32.GetClientRect(hwnd, &rect); ok == 0 {
		return Point{}
	}
	position := gridPoint(x, y, cols, rows, int(rect.Right-rect.Left), int(rect.Bottom-rect.Top))
	if !position.Valid {
		return Point{}
	}
	point := win32.POINT{X: int32(position.X), Y: int32(position.Y)}
	if win32.ClientToScreen(hwnd, &point) == 0 {
		return Point{}
	}
	return Point{X: int(point.X), Y: int(point.Y), Valid: true}
}

// Use the rendered viewport, not nominal font/graphics cell metrics: those can
// remain at their default size after console zoom or GUI display scaling.
func gridPoint(x, y, cols, rows, width, height int) Point {
	if cols <= 0 || rows <= 0 || width <= 0 || height <= 0 {
		return Point{}
	}
	return Point{X: x * width / cols, Y: y * height / rows, Valid: true}
}
