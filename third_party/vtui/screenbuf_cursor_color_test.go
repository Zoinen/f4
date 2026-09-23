package vtui

import (
	"strings"
	"testing"
)

// cursorColorTestState isolates one test from the package-wide cursor color
// state and puts it back afterwards: there is one terminal, so these globals
// are shared with every other test in the package.
func cursorColorTestState(t *testing.T) {
	t.Helper()
	oldColor, oldSent, oldManage := CursorColor, cursorColorSent, ManageCursorStyle
	CursorColor = -1
	cursorColorSent = -2
	ManageCursorStyle = true
	t.Cleanup(func() {
		CursorColor, cursorColorSent, ManageCursorStyle = oldColor, oldSent, oldManage
	})
}

// ansiRendererWithCapture wires a ScreenBuf whose frames land in a builder
// instead of the terminal.
func ansiRendererWithCapture(t *testing.T) (*AnsiRenderer, *strings.Builder) {
	t.Helper()
	scr := NewScreenBuf()
	r, ok := scr.Renderer.(*AnsiRenderer)
	if !ok {
		t.Fatalf("default renderer is %T, want *AnsiRenderer", scr.Renderer)
	}
	var out strings.Builder
	scr.Writer = &out
	return r, &out
}

func TestCursorColor_SentOnceAndOnChange(t *testing.T) {
	cursorColorTestState(t)
	r, out := ansiRendererWithCapture(t)

	CursorColor = 0xFFCC00
	r.Flush()
	if want := "\x1b]12;#FFCC00\x07"; !strings.Contains(out.String(), want) {
		t.Fatalf("frame does not name the cursor color %q, got %q", want, out.String())
	}

	// The terminal already knows the color, so a second frame must not repeat it.
	out.Reset()
	r.Flush()
	if strings.Contains(out.String(), "\x1b]12;") {
		t.Errorf("cursor color re-sent for an unchanged value: %q", out.String())
	}

	// Handing the color back is the only thing a negative value does.
	out.Reset()
	CursorColor = -1
	r.Flush()
	if !strings.Contains(out.String(), seqResetCursorColor) {
		t.Errorf("clearing CursorColor did not emit OSC 112, got %q", out.String())
	}
}

func TestCursorColor_SuppressedWhenCursorNotManaged(t *testing.T) {
	cursorColorTestState(t)
	r, out := ansiRendererWithCapture(t)

	ManageCursorStyle = false
	CursorColor = 0x00FF00
	r.Flush()
	if strings.Contains(out.String(), "\x1b]12;") {
		t.Fatalf("cursor color sent even though ManageCursorStyle is false: %q", out.String())
	}

	// The value was not consumed while suppressed, so switching management
	// back on in a running session still delivers it.
	out.Reset()
	ManageCursorStyle = true
	r.Flush()
	if want := "\x1b]12;#00FF00\x07"; !strings.Contains(out.String(), want) {
		t.Errorf("cursor color not sent after ManageCursorStyle was re-enabled, got %q", out.String())
	}
}

func TestCursorColor_SuspendRestoresAndResumeResends(t *testing.T) {
	cursorColorTestState(t)

	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	CursorColor = 0xFF0000
	cursorColorSent = CursorColor
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()
	if !strings.Contains(mock.builder.String(), seqResetCursorColor) {
		t.Fatalf("Suspend did not hand the cursor color back: %q", mock.builder.String())
	}
	if cursorColorSent != -2 {
		t.Fatalf("cursorColorSent = %d after Suspend, want -2 (unknown)", cursorColorSent)
	}

	// Whatever ran while vtui was suspended may have restyled the cursor, so
	// the next frame has to name the color again.
	mock.builder.Reset()
	r, out := ansiRendererWithCapture(t)
	r.Flush()
	if want := "\x1b]12;#FF0000\x07"; !strings.Contains(out.String(), want) {
		t.Errorf("cursor color not re-sent after Suspend, got %q", out.String())
	}
}

func TestCursorColor_SuspendSilentWhenNothingWasSent(t *testing.T) {
	cursorColorTestState(t)

	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()
	if strings.Contains(mock.builder.String(), seqResetCursorColor) {
		t.Errorf("Suspend reset a cursor color it never set: %q", mock.builder.String())
	}
}
