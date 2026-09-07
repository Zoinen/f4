package vtui

import (
	"encoding/base64"
	"os"
	"sync"
	"sync/atomic"
)

var (
	internalClipboard string
	internalClipMu    sync.Mutex

	// noTerminalBehind records that this process is drawing into a native
	// window rather than a terminal, so the OSC 52 escape fallback has
	// nobody to talk to.
	noTerminalBehind atomic.Bool

	testSkipOSClipboard bool
)

// DisableTerminalClipboard tells the clipboard layer that no terminal is
// attached to this process.
//
// SetClipboard ends with an OSC 52 escape sequence written to stdout, which is
// the right last resort in a terminal and the wrong one in a GUI window: there
// the sequence reaches either the shell the application was launched from,
// where it prints as garbage, or on Windows nothing at all. A GUI host calls
// this at startup so the internal buffer becomes the last resort instead,
// which GetClipboard already falls back to, keeping copy and paste working
// inside the application even where no OS clipboard helper is installed.
func DisableTerminalClipboard() { noTerminalBehind.Store(true) }

// TerminalClipboardDisabled reports whether the OSC 52 fallback is suppressed.
func TerminalClipboardDisabled() bool { return noTerminalBehind.Load() }

// SkipOSClipboard routes Set/GetClipboard past the OS clipboard helpers so
// all traffic stays in the process-local buffer. Test suites set it (together
// with DisableTerminalClipboard to silence the OSC 52 fallback): the real
// path shells out to pbcopy/xclip and reads back a clipboard that is global
// to the machine — slow and racy on a shared CI runner, and clobbering the
// developer's clipboard locally. Set it once before spawning goroutines;
// a test that genuinely targets the OS clipboard can switch it back off.
func SkipOSClipboard(skip bool) { testSkipOSClipboard = skip }

// SetClipboard copies text to the system clipboard.
func SetClipboard(text string) {
	DebugLog("CLIPBOARD: SetClipboard called, len: %d", len(text))
	// Global protection against terminal/IPC overload (2MB limit)
	const maxGlobalClipboardSize = 2 * 1024 * 1024
	if len(text) > maxGlobalClipboardSize {
		text = text[:maxGlobalClipboardSize]
		DebugLog("CLIPBOARD: Text truncated to %d bytes to prevent IPC lockup", maxGlobalClipboardSize)
	}
	internalClipMu.Lock()
	internalClipboard = text
	internalClipMu.Unlock()
	if SetFar2lClipboard(text) {
		DebugLog("CLIPBOARD: SetFar2lClipboard SUCCESS")
		return
	}
	DebugLog("CLIPBOARD: SetFar2lClipboard FAILED or DISABLED")
	if !testSkipOSClipboard && setOSClipboard(text) {
		DebugLog("CLIPBOARD: setOSClipboard SUCCESS")
		return
	}
	if noTerminalBehind.Load() {
		DebugLog("CLIPBOARD: setOSClipboard FAILED, no terminal attached; keeping the internal buffer")
		return
	}
	DebugLog("CLIPBOARD: setOSClipboard FAILED, falling back to OSC 52")

	// Cap the OSC 52 payload to 1MB to prevent terminal hangs
	const maxClipboardSize = 1024 * 1024
	if len(text) > maxClipboardSize {
		text = text[:maxClipboardSize]
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	// ANSI OSC 52: \x1b]52;c;<base64>\x07
	os.Stdout.WriteString("\x1b]52;c;" + b64 + "\x07")
}

// SetOSClipboard bypasses terminal extensions and writes directly to the OS clipboard.
func SetOSClipboard(text string) bool {
	if testSkipOSClipboard {
		return false
	}
	return setOSClipboard(text)
}

// GetClipboard retrieves text from the system clipboard.
func GetClipboard() string {
	DebugLog("CLIPBOARD: GetClipboard called")
	if text, ok := GetFar2lClipboard(); ok {
		DebugLog("CLIPBOARD: GetFar2lClipboard SUCCESS, len: %d", len(text))
		return text
	}
	DebugLog("CLIPBOARD: GetFar2lClipboard FAILED or DISABLED")
	if !testSkipOSClipboard {
		if text, ok := getOSClipboard(); ok {
			DebugLog("CLIPBOARD: getOSClipboard SUCCESS, len: %d", len(text))
			return text
		}
	}
	internalClipMu.Lock()
	fallback := internalClipboard
	internalClipMu.Unlock()
	DebugLog("CLIPBOARD: Returning internal buffer, len: %d", len(fallback))
	return fallback
}

// GetOSClipboard bypasses terminal extensions and reads directly from the OS clipboard.
func GetOSClipboard() string {
	if !testSkipOSClipboard {
		if text, ok := getOSClipboard(); ok {
			return text
		}
	}
	internalClipMu.Lock()
	fallback := internalClipboard
	internalClipMu.Unlock()
	return fallback
}
