package vtui

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected and returns what was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stdout = orig
	w.Close()
	out := <-done
	r.Close()
	return out
}

// withTerminalClipboard restores the global flag so these tests do not leak
// their state into the rest of the suite.
func withTerminalClipboard(t *testing.T, disabled bool) {
	t.Helper()
	prev := noTerminalBehind.Load()
	noTerminalBehind.Store(disabled)
	t.Cleanup(func() { noTerminalBehind.Store(prev) })
}

// In a GUI window the OSC 52 escape has no terminal to reach: it would land in
// the shell the application was launched from and print as garbage.
func TestSetClipboard_NoOSC52WhenTerminalDisabled(t *testing.T) {
	withTerminalClipboard(t, true)
	testSkipOSClipboard = true
	defer func() { testSkipOSClipboard = false }()

	out := captureStdout(t, func() { SetClipboard("secret text") })

	if strings.Contains(out, "\x1b]52") {
		t.Errorf("OSC 52 was emitted with no terminal attached: %q", out)
	}
	if out != "" {
		t.Errorf("expected nothing on stdout, got %q", out)
	}
}

// With a terminal attached the escape must still be the last resort, so the
// GUI change does not quietly disable clipboard support for terminal users.
func TestSetClipboard_EmitsOSC52WhenTerminalPresent(t *testing.T) {
	testSkipOSClipboard = true
	defer func() { testSkipOSClipboard = false }()
	withTerminalClipboard(t, false)

	out := captureStdout(t, func() { SetClipboard("hello") })

	if !strings.Contains(out, "\x1b]52;c;") {
		t.Errorf("expected an OSC 52 sequence on stdout, got %q", out)
	}
}

// Copy and paste must keep working inside the application even when no OS
// clipboard helper exists and the escape fallback is suppressed.
func TestClipboard_RoundTripsThroughInternalBuffer(t *testing.T) {
	withTerminalClipboard(t, true)
	testSkipOSClipboard = true
	defer func() { testSkipOSClipboard = false }()

	const want = "выделенный текст\nвторая строка"
	captureStdout(t, func() { SetClipboard(want) })

	if got := GetClipboard(); got != want {
		t.Errorf("GetClipboard() = %q, want %q", got, want)
	}
}

func TestDisableTerminalClipboard(t *testing.T) {
	withTerminalClipboard(t, false)

	if TerminalClipboardDisabled() {
		t.Fatal("expected the terminal fallback to start enabled")
	}
	DisableTerminalClipboard()
	if !TerminalClipboardDisabled() {
		t.Error("DisableTerminalClipboard did not take effect")
	}
}

// The 2MB cap has to survive the early return, or a GUI session could stash an
// unbounded string in the internal buffer.
func TestSetClipboard_TruncatesOversizedTextInGUIMode(t *testing.T) {
	withTerminalClipboard(t, true)
	testSkipOSClipboard = true
	defer func() { testSkipOSClipboard = false }()

	const limit = 2 * 1024 * 1024
	captureStdout(t, func() { SetClipboard(strings.Repeat("x", limit+4096)) })

	if got := len(GetClipboard()); got != limit {
		t.Errorf("stored %d bytes, want the %d byte cap", got, limit)
	}
}
