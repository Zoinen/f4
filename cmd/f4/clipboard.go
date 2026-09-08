package main

import (
	"encoding/base64"
	"runtime"

	"github.com/unxed/vtui"
)

// setF4Clipboard keeps vtui's regular clipboard integrations (far2l, the OS
// clipboard, and the process-local fallback), but also sends a copy through
// the terminal when f4 is running in a Unix TTY. This matters for SSH clients
// such as PuTTY: an SSH session may have DISPLAY set for X11 forwarding, so
// vtui's graphical clipboard driver can report success while updating the
// forwarded X server instead of the clipboard of the terminal on Windows.
const f4TerminalClipboardMaxBytes = 1024 * 1024

func setF4Clipboard(text string) {
	vtui.SetClipboard(text)
	if runtime.GOOS == "windows" || runningGUI || !probeHostTTY() {
		return
	}

	if len(text) > f4TerminalClipboardMaxBytes {
		text = text[:f4TerminalClipboardMaxBytes]
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	vtui.WritePassthrough([]byte("\x1b]52;c;" + b64 + "\x07"))
}
