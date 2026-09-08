package main

import (
	"bytes"
	"encoding/base64"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestSetF4Clipboard_UsesTerminalClipboardInTTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OSC 52 is the Unix terminal fallback")
	}

	oldProbeTTY := probeHostTTY
	oldRunningGUI := runningGUI
	t.Cleanup(func() {
		probeHostTTY = oldProbeTTY
		runningGUI = oldRunningGUI
	})
	probeHostTTY = func() bool { return true }
	runningGUI = false
	t.Cleanup(swapFrameManager(t))

	var out bytes.Buffer
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	scr.SetOutput(&out)
	vtui.FrameManager.Init(scr)

	const text = "Hello OSC 52"
	setF4Clipboard(text)

	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	if got := out.String(); !strings.Contains(got, want) {
		t.Fatalf("terminal output = %q, want OSC 52 sequence %q", got, want)
	}
	if got := vtui.GetClipboard(); got != text {
		t.Fatalf("internal clipboard = %q, want %q", got, text)
	}
}

func TestSetF4Clipboard_DoesNotWriteTerminalForGUI(t *testing.T) {
	oldProbeTTY := probeHostTTY
	oldRunningGUI := runningGUI
	t.Cleanup(func() {
		probeHostTTY = oldProbeTTY
		runningGUI = oldRunningGUI
	})
	probeHostTTY = func() bool { return true }
	runningGUI = true
	t.Cleanup(swapFrameManager(t))

	var out bytes.Buffer
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	scr.SetOutput(&out)
	vtui.FrameManager.Init(scr)

	setF4Clipboard("GUI clipboard")
	if got := out.String(); got != "" {
		t.Fatalf("GUI terminal output = %q, want no OSC 52 sequence", got)
	}
}
