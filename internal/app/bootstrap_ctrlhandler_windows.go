//go:build windows

package app

import (
	"sync"
	"syscall"
	"github.com/unxed/f4/internal/panel"
	"time"
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
	ctrlCEvent        = 0
	ctrlBreakEvent    = 1
	ctrlCloseEvent    = 2
	ctrlLogoffEvent   = 5
	ctrlShutdownEvent = 6
)

const consoleCloseSaveTimeout = 4 * time.Second

var saveSessionOnConsoleTermination = saveSessionForConsoleTermination

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
	case ctrlCloseEvent, ctrlLogoffEvent, ctrlShutdownEvent:
		// Windows terminates console processes after this callback returns, so
		// main's deferred SaveSession cannot protect settings on these paths.
		// Marshal the save onto the FrameManager UI goroutine before allowing
		// the system to terminate the process.
		saveSessionOnConsoleTermination()
		return 0
	default:
		return 0
	}
}

func saveSessionForConsoleTermination() {
	if vtui.FrameManager == nil {
		SaveSession()
		return
	}

	done := make(chan struct{})
	vtui.FrameManager.PostTask(func() {
		SaveSession()
		close(done)
	})

	timer := time.NewTimer(consoleCloseSaveTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		// A console control handler has a finite system timeout. Preserve the
		// last observable state even if the UI loop stopped accepting tasks.
		SaveSession()
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
		if pf, ok := fr.(*panel.PanelsFrame); ok {
			if active := pf.GetActivePTY(); active != nil {
				// Treat the interrupt as user input so it cannot interleave a
				// private environment update. Remote PTYs remain passthrough.
				_, _ = pf.WritePTY(active, []byte{3}) // ETX = Ctrl+C
			}
			return
		}
	}
}
