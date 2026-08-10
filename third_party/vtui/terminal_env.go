package vtui

import (
	"github.com/unxed/vtinput"
	"os"
	"sync"
)

const (
	seqAltScreenOn       = "\x1b[?1049h"
	seqAltScreenOff      = "\x1b[?1049l"
	seqAutoWrapOff       = "\x1b[?7l"
	seqAutoWrapOn        = "\x1b[?7h"
	seqBlinkingUnderline = "\x1b[3 q"
	seqDefaultCursor     = "\x1b[0 q"
	seqResetPalette      = "\x1b]104\x07"
	seqResetAttributes   = "\x1b[0m"
)

var (
	termMu            sync.Mutex
	inputRestore      func()
	isPrepared        bool
	inAltScreen       bool
	ManageCursorStyle bool = true
)

var getTermOut = func() interface {
	WriteString(string) (int, error)
	Sync() error
} {
	return os.Stdout
}

// PrepareTerminal puts the terminal into raw mode, enables advanced input,
// and switches to the alternate screen buffer. Returns a restore function.
func PrepareTerminal() (func(), error) {
	initTerminalOS()
	err := Resume()
	if err != nil {
		return nil, err
	}
	return Suspend, nil
}

// Suspend fully restores the terminal state (exits raw mode, alternate screen, etc.).
// Useful when temporarily returning control to the shell or an external program.
func Suspend() {
	termMu.Lock()
	defer termMu.Unlock()
	if isPrepared {
		out := getTermOut()
		out.WriteString(seqAutoWrapOn) // Restore auto-wrap
		if inAltScreen {
			out.WriteString(seqAltScreenOff)
			inAltScreen = false
		}
		if ManageCursorStyle {
			out.WriteString(seqDefaultCursor)
		}
		out.WriteString(seqResetPalette + seqResetAttributes)
		out.Sync()
		if inputRestore != nil {
			inputRestore()
			inputRestore = nil
		}
		isPrepared = false
	}
}

// Resume re-enables raw mode, advanced input, and returns to the alternate screen.
func Resume() error {
	termMu.Lock()
	defer termMu.Unlock()
	if !isPrepared {
		out := getTermOut()

		// 1. Enter AltScreen FIRST. Many terminals (like Kitty) reset
		// their keyboard protocol state when switching screen buffers.
		if !inAltScreen {
			out.WriteString(seqAltScreenOn)
			inAltScreen = true
		}
		out.WriteString(seqAutoWrapOff) // Disable auto-wrap for exact rendering
		out.Sync()

		// 2. Enable advanced input protocols AFTER entering AltScreen.
		r, err := vtinput.Enable()
		if err != nil {
			// Rollback AltScreen if input setup failed
			out.WriteString(seqAltScreenOff)
			inAltScreen = false
			out.Sync()
			return err
		}
		inputRestore = r

		if ManageCursorStyle {
			out.WriteString(seqBlinkingUnderline)
		}
		out.Sync()
		isPrepared = true

		// Force a full redraw if FrameManager is running
		if FrameManager != nil && FrameManager.scr != nil {
			FrameManager.scr.HardReset()
			FrameManager.Redraw()
		}
	}
	return nil
}

// SetAltScreen allows the application to temporarily switch between the
// alternate and main screen buffers without leaving raw mode.
func SetAltScreen(enable bool) {
	termMu.Lock()
	defer termMu.Unlock()

	if inAltScreen != enable {
		inAltScreen = enable
		out := getTermOut()
		if enable {
			out.WriteString(seqAltScreenOn)
			// When returning to alt screen, it's usually empty, so force a redraw
			if FrameManager != nil && FrameManager.scr != nil {
				FrameManager.scr.HardReset()
				FrameManager.Redraw()
			}
		} else {
			out.WriteString(seqAltScreenOff)
		}
		out.Sync()
	}
}
