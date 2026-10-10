//go:build windows

package vtui

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Alt+F9 in a classic Windows console window, done the way Far Manager does
// it (far/interf.cpp, ChangeVideoMode): WM_SYSCOMMAND SC_MAXIMIZE or
// SC_RESTORE to the console window, then the screen buffer sized to what the
// window shows.
//
// The buffer is the part that makes it work. conhost never makes a window
// larger than its screen buffer, and f4 keeps its buffer exactly the size of
// the viewport, so a bare SC_MAXIMIZE has no room to grow into. Far grows the
// buffer to GetLargestConsoleWindowSize; so does this, before the window is
// maximized, and then brings the buffer down to the viewport the maximized
// window actually has, through the same crash-safe sequence the alternate
// screen uses (planFit, f4 #397).
//
// Restoring cannot trust the viewport the restored window shows. At the
// moment of SC_RESTORE the buffer is still the size of the maximized
// viewport, so the restored window is too small for it and conhost puts
// scroll bars into it: its viewport is the client area minus the bars
// (SCREEN_INFORMATION::_CalculateViewportSize), and with "Wrap text output
// on resize" on it also cuts the buffer width to that (_AdjustScreenBuffer;
// both in microsoft/terminal's conhost source).
// Fitting the buffer to that viewport made the window a vertical scroll bar
// narrower on every Alt+F9 pair -- 120x30 came back as 118x30, and 80x25 as
// 78, then 76 (f4 #199). Far keeps the size the window had before it was
// maximized for exactly this reason (interf.cpp, SaveNonMaximisedBufferSize:
// "it could be less than previous because of horizontal scrollbar") and
// restores to that. So does this, as long as the restored window is the one
// that size was taken from; see restoreViewport.
//
// A console with a real window of its own is handled here. Under a
// pseudoconsole GetConsoleWindow answers with a PseudoConsoleWindow that is
// never shown; the window the user sees is the terminal's, which conpty makes
// the owner of that pseudo window (ConptyReparentPseudoConsole; Windows
// Terminal does this for every tab). Far asks that owner to maximize
// (far/console.cpp, console::GetWindow), and so does this. The terminal then
// resizes the pseudoconsole, and the new size reaches vtui as any other
// terminal resize does, so nothing here touches the buffer on that path.
// Without an owner the caller falls back to the xterm sequence as before.

var (
	procGetLargestConsoleWindowSize = kernel32.NewProc("GetLargestConsoleWindowSize")
	procIsZoomedConsole             = user32.NewProc("IsZoomed")
	procSendMessageWConsole         = user32.NewProc("SendMessageW")
	procPostMessageWConsole         = user32.NewProc("PostMessageW")
	procGetWindowConsole            = user32.NewProc("GetWindow")
	procGetWindowRectConsole        = user32.NewProc("GetWindowRect")
)

const gwOwner = 4 // GW_OWNER

// consoleNormalWindow is what the console looked like right before Alt+F9
// maximized it: the viewport, and the outer size of the window that viewport
// was in. Guarded by conhostAltMu.
type consoleNormalWindow struct {
	cols, rows int
	winW, winH int32 // GetWindowRect size, in pixels
	set        bool
}

var consoleBeforeMaximize consoleNormalWindow

// restoreViewport chooses the size the buffer is fitted to after SC_RESTORE.
//
// The size saved before maximizing is used only when the restored window has
// exactly the outer size it had then, which is what makes that viewport the
// one this window holds without scroll bars. When it does not -- nothing was
// saved because the window was maximized some other way, or it was restored,
// resized and maximized again in between -- the saved size belongs to another
// window geometry, and the viewport conhost reports is all there is.
func restoreViewport(saved consoleNormalWindow, winW, winH int32, afterW, afterH int) (w, h int, fromSaved bool) {
	if saved.set && saved.cols > 0 && saved.rows > 0 && saved.winW == winW && saved.winH == winH {
		return saved.cols, saved.rows, true
	}
	return afterW, afterH, false
}

func consoleWindowOuterSize(hwnd uintptr) (int32, int32, bool) {
	var r win32Rect
	if ok, _, _ := procGetWindowRectConsole.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return 0, 0, false
	}
	return r.right - r.left, r.bottom - r.top, true
}

func toggleConsoleMaximizedOS() bool {
	if !classicConsoleWindow() {
		owner := pseudoConsoleOwner()
		logPseudoConsoleLookup(owner)
		if owner != 0 {
			return toggleTerminalWindowMaximized(owner)
		}
		DebugLog("CONSOLE: toggle maximized: no classic console window and no pseudoconsole owner, not handled")
		return false
	}
	hwnd, _, _ := procGetConsoleWindowAlt.Call()
	if hwnd == 0 {
		return false
	}

	// CONOUT$ is the active screen buffer, whichever one that is: the one
	// f4 was started in, the alternate screen's, or the Win32 renderer's.
	out, err := windows.CreateFile(windows.StringToUTF16Ptr("CONOUT$"),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		DebugLog("CONSOLE: toggle maximized: opening the active screen buffer failed: %v", err)
		return false
	}
	defer windows.CloseHandle(out)
	handle := syscall.Handle(out)

	// The alternate screen fits its buffer to the viewport whenever the
	// size is polled; keep that from undoing the grow below mid-way.
	conhostAltMu.Lock()
	defer conhostAltMu.Unlock()

	seq := consoleToggleSeq.Add(1)
	logConsoleState(fmt.Sprintf("toggle #%d before", seq), handle, hwnd)

	var before consoleScreenBufferInfo
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&before))); ok == 0 {
		DebugLog("CONSOLE: toggle maximized: GetConsoleScreenBufferInfo failed")
		return false
	}
	zoomed, _, _ := procIsZoomedConsole.Call(hwnd)
	bw, bh := viewportSize(before)

	cmd := uintptr(scRestore)
	if zoomed == 0 {
		cmd = scMaximize
		largest, _, _ := procGetLargestConsoleWindowSize.Call(uintptr(handle))
		lw, lh := int16(uint16(largest)), int16(uint16(largest>>16))
		if lw > 0 && lh > 0 {
			grown := Coord{X: max(before.dwSize.X, lw), Y: max(before.dwSize.Y, lh)}
			if grown != before.dwSize {
				procSetConsoleScreenBufferSize.Call(uintptr(handle), coordArg(grown))
			}
		}
		// The window size is taken after the grow, right before the
		// maximize, so it is the window SC_RESTORE later brings back if
		// nothing else resizes it in between.
		consoleBeforeMaximize = consoleNormalWindow{}
		if ww, wh, ok := consoleWindowOuterSize(hwnd); ok {
			consoleBeforeMaximize = consoleNormalWindow{cols: bw, rows: bh, winW: ww, winH: wh, set: true}
		}
		DebugLog("CONSOLE: toggle maximized: maximizing, viewport %dx%d, buffer %dx%d, largest window %dx%d, window %dx%d px",
			bw, bh, before.dwSize.X, before.dwSize.Y, lw, lh, consoleBeforeMaximize.winW, consoleBeforeMaximize.winH)
	} else {
		DebugLog("CONSOLE: toggle maximized: restoring, viewport %dx%d, buffer %dx%d",
			bw, bh, before.dwSize.X, before.dwSize.Y)
	}

	procSendMessageWConsole.Call(hwnd, wmSysCommand, cmd, 0)
	logConsoleState(fmt.Sprintf("toggle #%d after the command", seq), handle, hwnd)

	var after consoleScreenBufferInfo
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(handle), uintptr(unsafe.Pointer(&after))); ok == 0 {
		DebugLog("CONSOLE: toggle maximized: GetConsoleScreenBufferInfo after the command failed")
		return true
	}
	aw, ah := viewportSize(after)
	nowZoomed, _, _ := procIsZoomedConsole.Call(hwnd)
	DebugLog("CONSOLE: toggle maximized: after the command IsZoomed=%v, viewport %dx%d, buffer %dx%d",
		nowZoomed != 0, aw, ah, after.dwSize.X, after.dwSize.Y)

	tw, th := aw, ah
	if cmd == scRestore {
		ww, wh, _ := consoleWindowOuterSize(hwnd)
		var fromSaved bool
		tw, th, fromSaved = restoreViewport(consoleBeforeMaximize, ww, wh, aw, ah)
		if fromSaved {
			DebugLog("CONSOLE: toggle maximized: restoring to the viewport before maximizing, %dx%d (window %dx%d px)",
				tw, th, ww, wh)
		} else {
			DebugLog("CONSOLE: toggle maximized: no size saved for this window (saved=%v, saved window %dx%d px, now %dx%d px), keeping the restored viewport %dx%d",
				consoleBeforeMaximize.set, consoleBeforeMaximize.winW, consoleBeforeMaximize.winH, ww, wh, tw, th)
		}
		consoleBeforeMaximize = consoleNormalWindow{}
	}
	fitConsoleBuffer(handle, tw, th)
	logConsoleState(fmt.Sprintf("toggle #%d after the fit", seq), handle, hwnd)
	logConsoleStateLater(fmt.Sprintf("toggle #%d +%v", seq, consoleLateSnapshot))
	return true
}

// pseudoConsoleOwner returns the window that owns this process's pseudo
// console window -- the terminal's own window under Windows Terminal -- or 0
// when the console is not a pseudoconsole or nothing owns it.
func pseudoConsoleOwner() uintptr {
	h, _, _ := procGetConsoleWindowAlt.Call()
	if h == 0 {
		return 0
	}
	var buf [64]uint16
	n, _, _ := procGetClassNameWAlt.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || syscall.UTF16ToString(buf[:n]) != "PseudoConsoleWindow" {
		return 0
	}
	owner, _, _ := procGetWindowConsole.Call(h, gwOwner)
	return owner
}

// toggleTerminalWindowMaximized maximizes or restores the terminal window on
// the far side of a pseudoconsole. The message is posted, not sent: the
// window belongs to another process, and nothing here waits on its answer --
// the size change comes back as a pseudoconsole resize.
func toggleTerminalWindowMaximized(owner uintptr) bool {
	seq := consoleToggleSeq.Add(1)
	logTerminalWindowState(fmt.Sprintf("terminal toggle #%d before the command", seq), owner)
	defer logTerminalWindowStateLater(fmt.Sprintf("terminal toggle #%d +%v", seq, consoleLateSnapshot), owner)

	zoomed, _, _ := procIsZoomedConsole.Call(owner)
	cmd, name := uintptr(scMaximize), "SC_MAXIMIZE"
	if zoomed != 0 {
		cmd, name = scRestore, "SC_RESTORE"
	}
	if ok, _, err := procPostMessageWConsole.Call(owner, wmSysCommand, cmd, 0); ok == 0 {
		DebugLog("CONSOLE: toggle maximized: pseudoconsole owner %#x, IsZoomed=%v, posting %s failed: %v",
			owner, zoomed != 0, name, err)
		return false
	}
	DebugLog("CONSOLE: toggle maximized: pseudoconsole owner %#x, IsZoomed=%v, posted %s",
		owner, zoomed != 0, name)
	return true
}
