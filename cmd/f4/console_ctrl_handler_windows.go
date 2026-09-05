//go:build windows

package main

import (
	"sync"
	"syscall"

	"github.com/unxed/vtui"
)

// installConsoleCtrlHandler registers a console ctrl handler (FAR3's
// console::SetControlHandler). By default Windows terminates the process on
// CTRL_BREAK_EVENT; f4 must survive it and treat Ctrl+Break like Ctrl+C
// (interrupt the program running in ConPTY, return to panels).
func installConsoleCtrlHandler() {
	ctrlHandlerOnce.Do(func() {
		procSetConsoleCtrlHandler.Call(syscall.NewCallback(consoleCtrlHandlerRoutine), 1)
	})
}

var ctrlHandlerOnce sync.Once

var procSetConsoleCtrlHandler = syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleCtrlHandler")

const (
	ctrlCEvent     = 0
	ctrlBreakEvent = 1
	ctrlCloseEvent = 2
)

// consoleCtrlHandlerRoutine mirrors FAR3's control_handler(): Ctrl+Break is
// translated into an interrupt of the active ConPTY program (same as Ctrl+C),
// and the event is swallowed so f4 itself keeps running.
func consoleCtrlHandlerRoutine(ctrlType uintptr) uintptr {
	switch ctrlType {
	case ctrlCEvent, ctrlBreakEvent:
		if ctrlType == ctrlBreakEvent {
			interruptActivePTY()
		}
		return 1
	default:
		return 0
	}
}

// interruptActivePTY writes an interrupt (ETX, i.e. Ctrl+C) into the ConPTY
// program currently running in the foreground, so the app returns to panels.
func interruptActivePTY() {
	defer func() {
		recover() // vtui state may change concurrently; never crash the handler
	}()
	if vtui.FrameManager == nil {
		return
	}
	screens := vtui.FrameManager.Screens
	if len(screens) == 0 {
		return
	}
	idx := vtui.FrameManager.ActiveIdx
	if idx < 0 || idx >= len(screens) {
		return
	}
	for _, fr := range screens[idx].Frames {
		if pf, ok := fr.(*PanelsFrame); ok {
			if active := pf.getActivePTY(); active != nil {
				// Treat the interrupt as user input so it cannot interleave a
				// private environment update. Remote PTYs remain passthrough.
				_, _ = pf.writePTY(active, []byte{3}) // ETX = Ctrl+C
			}
			return
		}
	}
}
