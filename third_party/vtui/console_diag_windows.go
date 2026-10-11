//go:build windows

package vtui

import (
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Diagnostics for Alt+F9 in a classic console window (f4 #199). Nothing in
// this file changes what the console does; it only writes to the debug log.
//
// Two effects are reported that the existing log lines cannot explain:
//
//   - on inbox conhost, after a maximize and restore, the window's client
//     area is 17 px -- one SM_CXVSCROLL at 96 dpi -- wider than the 120
//     columns it shows, with no scroll bar drawn (seldom on OpenConsole);
//   - holding Alt+F9 for a while leaves a screen that does not match the
//     window after the key is released (OpenConsole; not in Windows
//     Terminal).
//
// conhost applies part of a size change later, on its window thread:
// SetConsoleWindowInfo posts CM_SET_WINDOW_SIZE, and scroll bar updates are
// posted as CM_UPDATE_SCROLL_BARS (microsoft/terminal, getset.cpp and
// windowproc.cpp). A snapshot taken right after the console calls can
// therefore miss the final state, so each toggle also takes one some time
// later.

var (
	procGetClientRectConsole      = user32.NewProc("GetClientRect")
	procGetWindowLongWConsole     = user32.NewProc("GetWindowLongW")
	procGetWindowPlacementConsole = user32.NewProc("GetWindowPlacement")
	procMonitorFromWindowConsole  = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoWConsole    = user32.NewProc("GetMonitorInfoW")
	procGetWindowThreadProcessID  = user32.NewProc("GetWindowThreadProcessId")
	procGetAncestorConsole        = user32.NewProc("GetAncestor")
	procGetConsoleProcessList     = kernel32.NewProc("GetConsoleProcessList")
)

const (
	gwlStyleConsole     = -16 // GWL_STYLE
	consoleLateSnapshot = 500 * time.Millisecond
)

var consoleToggleSeq atomic.Int64

// logConsoleState writes one line with everything conhost and user32 report
// about the screen buffer behind handle and the console window hwnd.
func logConsoleState(tag string, handle syscall.Handle, hwnd uintptr) {
	var info consoleScreenBufferInfo
	csbiOK, _, csbiErr := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&info)))
	var wr, cr win32Rect
	wrOK, _, _ := procGetWindowRectConsole.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	crOK, _, _ := procGetClientRectConsole.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	style := int32(gwlStyleConsole)
	st, _, _ := procGetWindowLongWConsole.Call(hwnd, uintptr(style))
	zoomed, _, _ := procIsZoomedConsole.Call(hwnd)
	csbi := "failed: " + fmt.Sprint(csbiErr)
	if csbiOK != 0 {
		csbi = fmt.Sprintf("buffer %dx%d, srWindow L%d T%d R%d B%d (%dx%d), cursor %d,%d",
			info.dwSize.X, info.dwSize.Y,
			info.srWindow.Left, info.srWindow.Top, info.srWindow.Right, info.srWindow.Bottom,
			int(info.srWindow.Right-info.srWindow.Left)+1, int(info.srWindow.Bottom-info.srWindow.Top)+1,
			info.dwCursorPosition.X, info.dwCursorPosition.Y)
	}
	DebugLog("CONSOLE: state %s: %s; window %dx%d px (ok=%v), client %dx%d px (ok=%v), WS_VSCROLL=%v WS_HSCROLL=%v, IsZoomed=%v",
		tag, csbi,
		wr.right-wr.left, wr.bottom-wr.top, wrOK != 0,
		cr.right-cr.left, cr.bottom-cr.top, crOK != 0,
		uint32(st)&wsVScroll != 0, uint32(st)&wsHScroll != 0, zoomed != 0)
}

// logConsoleStateLater takes the same snapshot after conhost has had time to
// run what it posted to its window thread. It opens the active screen buffer
// itself: the caller's handle is closed by then.
func logConsoleStateLater(tag string) {
	go func() {
		time.Sleep(consoleLateSnapshot)
		hwnd, _, _ := procGetConsoleWindowAlt.Call()
		out, err := windows.CreateFile(windows.StringToUTF16Ptr("CONOUT$"),
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			DebugLog("CONSOLE: state %s: opening the active screen buffer failed: %v", tag, err)
			return
		}
		defer windows.CloseHandle(out)
		logConsoleState(tag, syscall.Handle(out), hwnd)
	}()
}

func fitOpLabel(op fitOp) string {
	switch op {
	case fitGrowBuffer:
		return "grow buffer"
	case fitMoveCursor:
		return "move cursor"
	case fitWindow:
		return "window"
	case fitSizeBuffer:
		return "size buffer"
	}
	return fmt.Sprintf("op%d", int(op))
}

// Under a pseudoconsole (Windows Terminal) the window Alt+F9 acts on is the
// terminal's, not the console's. These lines record which window that was,
// and where it was and what state it was in before the command and some time
// after it, next to the monitor's work area -- enough to tell a maximize from
// a plain resize of the window from where it stood.

// logPseudoConsoleLookup records what pseudoConsoleOwner looked at.
func logPseudoConsoleLookup(owner uintptr) {
	h, _, _ := procGetConsoleWindowAlt.Call()
	DebugLog("CONSOLE: pseudoconsole lookup: GetConsoleWindow %#x class %q, GW_OWNER %#x class %q pid %d",
		h, consoleWindowClass(h), owner, consoleWindowClass(owner), consoleWindowPID(owner))
	start := consoleWindowStart
	if start.hwnd != h || start.owner != owner {
		DebugLog("CONSOLE: pseudoconsole lookup: at startup, before this process made a pseudoconsole of its own, GetConsoleWindow was %#x class %q made by pid %d %q, GW_OWNER %#x class %q",
			start.hwnd, start.class, start.pid, processImage(start.pid),
			start.owner, start.ownerClass)
	}
	if owner != 0 || h == 0 {
		return
	}
	// Nothing owns the pseudo window. What follows is where this console
	// came from: which process made that window, what the root owner chain
	// says, whether Windows Terminal started this process (it sets
	// WT_SESSION for what it launches), who started f4, and which processes
	// share the console.
	rootOwner, _, _ := procGetAncestorConsole.Call(h, gaRootOwner)
	hostPID := consoleWindowPID(h)
	_, wtSession := os.LookupEnv("WT_SESSION")
	self := uint32(os.Getpid())
	parents := processParents()
	parent := parents[self]
	DebugLog("CONSOLE: pseudoconsole lookup: no owner; window made by pid %d %q, GA_ROOTOWNER %#x class %q, WT_SESSION set=%v, f4 pid %d started by pid %d %q",
		hostPID, processImage(hostPID), rootOwner, consoleWindowClass(rootOwner), wtSession,
		self, parent, processImage(parent))
	var pids [32]uint32
	n, _, _ := procGetConsoleProcessList.Call(uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	if n > uintptr(len(pids)) {
		n = uintptr(len(pids))
	}
	var list strings.Builder
	for _, pid := range pids[:n] {
		fmt.Fprintf(&list, " %d %q (parent %d);", pid, processImage(pid), parents[pid])
	}
	DebugLog("CONSOLE: pseudoconsole lookup: processes on this console:%s", list.String())
}

const gaRootOwner = 3 // GA_ROOTOWNER

// processImage is the full path of a process's executable, or "" when it
// cannot be read.
func processImage(pid uint32) string {
	if pid == 0 {
		return ""
	}
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(proc)
	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(proc, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// processParents maps every running process to the process that started it.
func processParents() map[uint32]uint32 {
	parents := map[uint32]uint32{}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return parents
	}
	defer windows.CloseHandle(snap)
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		parents[pe.ProcessID] = pe.ParentProcessID
	}
	return parents
}

func consoleWindowClass(hwnd uintptr) string {
	if hwnd == 0 {
		return ""
	}
	var buf [128]uint16
	n, _, _ := procGetClassNameWAlt.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func consoleWindowPID(hwnd uintptr) uint32 {
	if hwnd == 0 {
		return 0
	}
	var pid uint32
	procGetWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

// win32WindowPlacement is WINDOWPLACEMENT.
type win32WindowPlacement struct {
	length           uint32
	flags            uint32
	showCmd          uint32
	ptMinPosition    win32Point
	ptMaxPosition    win32Point
	rcNormalPosition win32Rect
}

// win32MonitorInfo is MONITORINFO.
type win32MonitorInfo struct {
	cbSize    uint32
	rcMonitor win32Rect
	rcWork    win32Rect
	dwFlags   uint32
}

const monitorDefaultToNearest = 2 // MONITOR_DEFAULTTONEAREST

func formatWin32Rect(r win32Rect) string {
	return fmt.Sprintf("L%d T%d R%d B%d (%dx%d)", r.left, r.top, r.right, r.bottom, r.right-r.left, r.bottom-r.top)
}

// logTerminalWindowState writes one line about the terminal window hwnd:
// its rectangle, IsZoomed, the show command and normal position from its
// placement, and the monitor it is on with that monitor's work area.
func logTerminalWindowState(tag string, hwnd uintptr) {
	var wr win32Rect
	wrOK, _, _ := procGetWindowRectConsole.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	zoomed, _, _ := procIsZoomedConsole.Call(hwnd)
	wp := win32WindowPlacement{length: uint32(unsafe.Sizeof(win32WindowPlacement{}))}
	wpOK, _, _ := procGetWindowPlacementConsole.Call(hwnd, uintptr(unsafe.Pointer(&wp)))
	mi := win32MonitorInfo{cbSize: uint32(unsafe.Sizeof(win32MonitorInfo{}))}
	monitor, _, _ := procMonitorFromWindowConsole.Call(hwnd, monitorDefaultToNearest)
	miOK := uintptr(0)
	if monitor != 0 {
		miOK, _, _ = procGetMonitorInfoWConsole.Call(monitor, uintptr(unsafe.Pointer(&mi)))
	}
	DebugLog("CONSOLE: terminal window %s: hwnd %#x, window %s (ok=%v), IsZoomed=%v, placement showCmd=%d normal %s (ok=%v), monitor %s work area %s (ok=%v)",
		tag, hwnd, formatWin32Rect(wr), wrOK != 0, zoomed != 0,
		wp.showCmd, formatWin32Rect(wp.rcNormalPosition), wpOK != 0,
		formatWin32Rect(mi.rcMonitor), formatWin32Rect(mi.rcWork), miOK != 0)
}

// logTerminalWindowStateLater takes the same snapshot after the terminal has
// had time to act on the posted command and resize the pseudoconsole.
func logTerminalWindowStateLater(tag string, hwnd uintptr) {
	go func() {
		time.Sleep(consoleLateSnapshot)
		logTerminalWindowState(tag, hwnd)
	}()
}

// consoleWindowStart is what GetConsoleWindow answered when f4 started.
//
// It is taken before main runs, so before anything in this process creates a
// pseudoconsole of its own. f4 does create one, for the shell it hosts, and
// it loads the ConPTY package, whose console host runs inside this process
// and makes its own PseudoConsoleWindow here (f4 #199: a run where
// GetConsoleWindow later answered with a window made by f4's own process,
// owned by nothing, while f4's output was plainly going to a Windows
// Terminal window, which the xterm resize sequence resized). Comparing the
// two answers says whether the window Alt+F9 looks at is still the one the
// process was started with.
type consoleWindowSnapshot struct {
	hwnd       uintptr
	class      string
	pid        uint32
	owner      uintptr
	ownerClass string
}

var consoleWindowStart = takeConsoleWindowSnapshot()

func takeConsoleWindowSnapshot() consoleWindowSnapshot {
	h, _, _ := procGetConsoleWindowAlt.Call()
	if h == 0 {
		return consoleWindowSnapshot{}
	}
	owner, _, _ := procGetWindowConsole.Call(h, gwOwner)
	return consoleWindowSnapshot{
		hwnd:       h,
		class:      consoleWindowClass(h),
		pid:        consoleWindowPID(h),
		owner:      owner,
		ownerClass: consoleWindowClass(owner),
	}
}
