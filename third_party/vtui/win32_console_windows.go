//go:build windows

package vtui

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	errConsoleHandleInvalid   = errors.New("vtui: win32 console handle invalid")
	errConsoleSizeQueryFailed = errors.New("vtui: GetConsoleScreenBufferInfo failed")
	errConsoleSizeInvalid     = errors.New("vtui: win32 console reported non-positive window size")
)

var (
	ntdllDLL           = syscall.NewLazyDLL("ntdll.dll")
	procWineGetVersion = ntdllDLL.NewProc("wine_get_version")

	procReadConsoleOutputW           = kernel32.NewProc("ReadConsoleOutputW")
	procWriteConsoleOutputW          = kernel32.NewProc("WriteConsoleOutputW")
	procSetConsoleCursorPosition     = kernel32.NewProc("SetConsoleCursorPosition")
	procSetConsoleTitleW             = kernel32.NewProc("SetConsoleTitleW")
	procGetConsoleScreenBufferInfo   = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procCreateConsoleScreenBuffer    = kernel32.NewProc("CreateConsoleScreenBuffer")
	procSetConsoleActiveScreenBuffer = kernel32.NewProc("SetConsoleActiveScreenBuffer")
	procSetConsoleScreenBufferSize   = kernel32.NewProc("SetConsoleScreenBufferSize")
	procSetConsoleWindowInfo         = kernel32.NewProc("SetConsoleWindowInfo")
)

func resetConsoleWindowPos(hConsole syscall.Handle, w, h int16) {
	if hConsole == 0 || hConsole == syscall.InvalidHandle {
		return
	}
	rect := SmallRect{
		Left:   0,
		Top:    0,
		Right:  w - 1,
		Bottom: h - 1,
	}
	procSetConsoleWindowInfo.Call(
		uintptr(hConsole),
		uintptr(1), // TRUE = absolute
		uintptr(unsafe.Pointer(&rect)),
	)
}

var (
	activeWin32ConsoleRenderer *Win32ConsoleRenderer
	activeWin32ConsoleMu       sync.Mutex
)

func setAltScreenWin32(enable bool) {
	activeWin32ConsoleMu.Lock()
	r := activeWin32ConsoleRenderer
	activeWin32ConsoleMu.Unlock()
	if r != nil && r.hFarOut != 0 && r.hFarOut != r.hStdOut {
		if enable {
			procSetConsoleActiveScreenBuffer.Call(uintptr(r.hFarOut))
		} else {
			procSetConsoleActiveScreenBuffer.Call(uintptr(r.hStdOut))
		}
	}
}

func getActiveConsoleHandle() uintptr {
	activeWin32ConsoleMu.Lock()
	r := activeWin32ConsoleRenderer
	activeWin32ConsoleMu.Unlock()
	if r != nil && r.hFarOut != 0 {
		return uintptr(r.hFarOut)
	}
	return 0
}

// win32ConsoleActive reports whether the classic Windows Console API renderer
// currently owns the visible screen via its own dedicated screen buffer
// (hFarOut distinct from hStdOut). When it does, the console buffer the user
// sees after leaving f4's screen (e.g. Ctrl+O under WINE.md's no-PTY console
// view) is not a VT stream: it is hStdOut, painted directly with
// WriteConsoleOutputW, and ANSI escape sequences written into it would show
// up as literal text or move the buffer's own cursor instead of doing
// anything useful.
func win32ConsoleActive() bool {
	activeWin32ConsoleMu.Lock()
	defer activeWin32ConsoleMu.Unlock()
	r := activeWin32ConsoleRenderer
	return r != nil && r.hFarOut != 0 && r.hFarOut != r.hStdOut
}

type consoleScreenBufferInfo struct {
	dwSize              Coord
	dwCursorPosition    Coord
	wAttributes         uint16
	srWindow            SmallRect
	dwMaximumWindowSize Coord
}

func isWineOS() bool {
	return procWineGetVersion.Find() == nil
}

func hasConsoleBufferOS() bool {
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hOut == syscall.InvalidHandle || hOut == 0 {
		return false
	}
	var csbi consoleScreenBufferInfo
	r1, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&csbi)))
	return r1 != 0 && csbi.dwSize.X > 0 && csbi.dwSize.Y > 0
}
func hasVTConsoleSupportOS() bool {
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hOut == syscall.InvalidHandle || hOut == 0 {
		return false
	}
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(hOut), &mode); err != nil {
		return false
	}
	const enableVT = 0x0004 // ENABLE_VIRTUAL_TERMINAL_PROCESSING
	if (mode & enableVT) != 0 {
		return true
	}
	if err := windows.SetConsoleMode(windows.Handle(hOut), mode|enableVT); err != nil {
		return false
	}
	_ = windows.SetConsoleMode(windows.Handle(hOut), mode)
	return true
}

// Win32ConsoleRenderer implements SurfaceRenderer using the classic Windows Console API (WriteConsoleOutputW).
type Win32ConsoleRenderer struct {
	mu          sync.Mutex
	parent      *ScreenBuf
	hStdOut     syscall.Handle
	hFarOut     syscall.Handle
	consoleBuf  []win32CharInfo
	lastCols    int
	lastRows    int
	lastTitle   string
	cursorX     int
	cursorY     int
	cursorVis   bool
	cursorShape CursorShape
	activePal   *[256]uint32
	forceRedraw bool
}

// NewWin32ConsoleRenderer creates a renderer using classic Win32 Console API with a dedicated screen buffer.
func NewWin32ConsoleRenderer(parent *ScreenBuf) *Win32ConsoleRenderer {
	hStdOut, _ := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	hFarOut := hStdOut

	r1, _, _ := procCreateConsoleScreenBuffer.Call(
		uintptr(0xC0000000), // GENERIC_READ | GENERIC_WRITE
		uintptr(3),          // FILE_SHARE_READ | FILE_SHARE_WRITE
		0,
		uintptr(1), // CONSOLE_TEXTMODE_BUFFER
		0,
	)
	if r1 != 0 && syscall.Handle(r1) != syscall.InvalidHandle {
		hFarOut = syscall.Handle(r1)
		procSetConsoleActiveScreenBuffer.Call(uintptr(hFarOut))
	}

	r := &Win32ConsoleRenderer{
		parent:  parent,
		hStdOut: hStdOut,
		hFarOut: hFarOut,
	}

	activeWin32ConsoleMu.Lock()
	activeWin32ConsoleRenderer = r
	activeWin32ConsoleMu.Unlock()

	// Every other renderer (x11, wayland, gogpu, ebiten, win32 GUI) points
	// the shared GetTerminalSize at its own live surface when it takes over.
	// This one didn't: the package default queries os.Stdout's handle, which
	// is hStdOut -- the buffer this renderer just made inactive by switching
	// the console to its own hFarOut. hStdOut's window rect is a snapshot
	// from before the switch and is never updated by the OS while it isn't
	// the active buffer, so every resize from here on read a stale size
	// instead of hFarOut's real, live, currently-resized one. Normally the
	// two stay close enough (the OS keeps a freshly created buffer's initial
	// window rect near the host window's actual size) that this goes
	// unnoticed; it stops being close once inactive hStdOut and active
	// hFarOut are allowed to diverge -- e.g. a shortcut with an explicit,
	// different screen-buffer size and "wrap text output on resize"
	// unchecked, matching the repro narrowed in f4 issue #397.
	GetTerminalSize = func() (int, int, error) {
		return r.terminalSize()
	}

	return r
}

// terminalSize reads the window rectangle of this renderer's own active
// screen buffer (hFarOut, falling back to hStdOut if the dedicated buffer
// could not be created) instead of whatever os.Stdout happens to point at.
func (r *Win32ConsoleRenderer) terminalSize() (int, int, error) {
	handle := r.hFarOut
	if handle == 0 {
		handle = r.hStdOut
	}
	if handle == 0 || handle == syscall.InvalidHandle {
		return 0, 0, errConsoleHandleInvalid
	}
	var csbi consoleScreenBufferInfo
	ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&csbi)))
	if ok == 0 {
		return 0, 0, errConsoleSizeQueryFailed
	}
	w := int(csbi.srWindow.Right-csbi.srWindow.Left) + 1
	h := int(csbi.srWindow.Bottom-csbi.srWindow.Top) + 1
	if w <= 0 || h <= 0 {
		return 0, 0, errConsoleSizeInvalid
	}
	return w, h, nil
}

func (r *Win32ConsoleRenderer) Close() error {
	activeWin32ConsoleMu.Lock()
	defer activeWin32ConsoleMu.Unlock()
	if r.hFarOut != 0 && r.hFarOut != r.hStdOut {
		procSetConsoleActiveScreenBuffer.Call(uintptr(r.hStdOut))
		syscall.CloseHandle(r.hFarOut)
		r.hFarOut = r.hStdOut
	}
	if activeWin32ConsoleRenderer == r {
		activeWin32ConsoleRenderer = nil
	}
	return nil
}

func (r *Win32ConsoleRenderer) SetPalette(pal *[256]uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activePal = pal
}

func (r *Win32ConsoleRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cursorX = x
	r.cursorY = y
	r.cursorVis = visible
	r.cursorShape = shape
}

func (r *Win32ConsoleRenderer) SetWindowTitle(title string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if title == r.lastTitle {
		return
	}
	r.lastTitle = title
	u16, err := syscall.UTF16PtrFromString(title)
	if err == nil {
		procSetConsoleTitleW.Call(uintptr(unsafe.Pointer(u16)))
	}
}

func (r *Win32ConsoleRenderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if w <= 0 || h <= 0 {
		return
	}

	size := w * h
	if len(r.consoleBuf) != size || r.lastCols != w || r.lastRows != h {
		r.consoleBuf = make([]win32CharInfo, size)
		r.lastCols = w
		r.lastRows = h
		forceRedraw = true
	}

	r.forceRedraw = forceRedraw
	pal := r.activePal
	if pal == nil && r.parent != nil {
		if r.parent.ActivePalette != nil {
			pal = r.parent.ActivePalette
		} else {
			pal = r.parent.ThemePalette
		}
	}

	for i := 0; i < size; i++ {
		if forceRedraw || buf[i] != shadow[i] {
			r.consoleBuf[i] = charInfoToWin32(buf[i], pal)
		}
	}
}

func (r *Win32ConsoleRenderer) Flush() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.consoleBuf) == 0 || r.lastCols <= 0 || r.lastRows <= 0 {
		return
	}

	w := int16(r.lastCols)
	h := int16(r.lastRows)

	targetHandle := r.hFarOut
	if targetHandle == 0 {
		targetHandle = r.hStdOut
	}

	resetConsoleWindowPos(targetHandle, w, h)

	bufSize := uintptr(uint32(uint16(w)) | (uint32(uint16(h)) << 16))
	bufCoord := uintptr(0)
	writeRegion := SmallRect{
		Left:   0,
		Top:    0,
		Right:  w - 1,
		Bottom: h - 1,
	}

	procWriteConsoleOutputW.Call(
		uintptr(targetHandle),
		uintptr(unsafe.Pointer(&r.consoleBuf[0])),
		bufSize,
		bufCoord,
		uintptr(unsafe.Pointer(&writeRegion)),
	)

	// Update cursor position and shape
	if r.cursorVis && r.cursorX >= 0 && r.cursorX < int(w) && r.cursorY >= 0 && r.cursorY < int(h) {
		cursorCoord := uintptr(uint32(uint16(r.cursorX)) | (uint32(uint16(r.cursorY)) << 16))
		procSetConsoleCursorPosition.Call(uintptr(targetHandle), cursorCoord)
		SetCursorStyleOS(true, r.cursorShape)
	} else {
		SetCursorStyleOS(false, r.cursorShape)
	}
}
