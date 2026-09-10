package terminal

import (
	"bytes"
	"encoding/base64"
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"runtime"
	"strings"
	"testing"
)

func TestSetF4Clipboard_UsesTerminalClipboardInTTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("OSC 52 is the Unix terminal fallback")
	}

	oldProbeTTY := ProbeHostTTY
	oldRunningGUI := gui.Running
	t.Cleanup(func() {
		ProbeHostTTY = oldProbeTTY
		gui.Running = oldRunningGUI
	})
	ProbeHostTTY = func() bool { return true }
	gui.Running = false
	t.Cleanup(testutil.SwapFrameManager(t, func(*testing.T) { WaitForAsyncClipboard() }))

	var out bytes.Buffer
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	scr.SetOutput(&out)
	vtui.FrameManager.Init(scr)

	const text = "Hello OSC 52"
	SetF4Clipboard(text)

	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\x07"
	if got := out.String(); !strings.Contains(got, want) {
		t.Fatalf("terminal output = %q, want OSC 52 sequence %q", got, want)
	}
	if got := vtui.GetClipboard(); got != text {
		t.Fatalf("internal clipboard = %q, want %q", got, text)
	}
}

func TestSetF4Clipboard_DoesNotWriteTerminalForGUI(t *testing.T) {
	oldProbeTTY := ProbeHostTTY
	oldRunningGUI := gui.Running
	t.Cleanup(func() {
		ProbeHostTTY = oldProbeTTY
		gui.Running = oldRunningGUI
	})
	ProbeHostTTY = func() bool { return true }
	gui.Running = true
	t.Cleanup(testutil.SwapFrameManager(t, func(*testing.T) { WaitForAsyncClipboard() }))

	var out bytes.Buffer
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(80, 25)
	scr.SetOutput(&out)
	vtui.FrameManager.Init(scr)

	SetF4Clipboard("GUI clipboard")
	if got := out.String(); got != "" {
		t.Fatalf("GUI terminal output = %q, want no OSC 52 sequence", got)
	}
}
