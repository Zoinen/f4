package panel

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// TestHostInputRestoreSeq pins what f4 asks the terminal for on the way back
// from the host console: the same modes vtinput turned on at startup, and
// nothing at all where the request is not a VT one.
func TestHostInputRestoreSeq(t *testing.T) {
	seq := hostInputRestoreSeq(false, false)
	for _, want := range []string{
		"\x1b[?1002h", "\x1b[?1003h", "\x1b[?1015h", "\x1b[?1006h", "\x1b[?1004h", "\x1b[?2004h",
	} {
		if !strings.Contains(seq, want) {
			t.Errorf("hostInputRestoreSeq() is missing %q: %q", want, seq)
		}
	}
	if got := hostInputRestoreSeq(true, false); got != "" {
		t.Errorf("a native console reader asks through the console mode, got %q", got)
	}
	if got := hostInputRestoreSeq(false, true); got != "" {
		t.Errorf("FreeBSD syscons has no protocols to ask for, got %q", got)
	}
}

// TestHostConsole_LeaveRestoresMouseTracking is the regression for #924: a
// command run in the host console leaves the terminal with mouse reporting
// off -- the child turns it off on its way out, or the console host does it
// for the child -- and returning to the panels has to turn it back on, or the
// clicks keep going to the terminal.
func TestHostConsole_LeaveRestoresMouseTracking(t *testing.T) {
	if nativeConsoleInput() {
		t.Skip("native console input: the request is the console mode, not a VT sequence")
	}

	scr := vtui.NewSilentScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ShellMode = terminal.ShellModeHost
	pf.ResizeConsole(80, 25)

	pf.EnterHostConsole()
	out.Reset()
	pf.LeaveHostConsole()

	written := out.String()
	for _, want := range []string{"\x1b[?1003h", "\x1b[?1006h", "\x1b[?2004h"} {
		if !strings.Contains(written, want) {
			t.Fatalf("leaveHostConsole did not re-enable %q: %q", want, written)
		}
	}
	// Order matters as much as presence: the protective reset comes first,
	// the request after it. The other way round would leave the terminal
	// with the mouse off, which is the bug itself.
	if off, on := strings.Index(written, "\x1b[?1003l"), strings.Index(written, "\x1b[?1003h"); off > on {
		t.Fatalf("mouse tracking disabled after being re-enabled: %q", written)
	}
}
