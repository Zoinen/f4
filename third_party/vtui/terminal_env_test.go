package vtui

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

type mockTermOut struct {
	builder strings.Builder
}

func (m *mockTermOut) WriteString(s string) (int, error) {
	return m.builder.WriteString(s)
}
func (m *mockTermOut) Sync() error { return nil }

func TestTerminalEnv_AltScreenManagement(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// Reset internal state
	isPrepared = true
	inAltScreen = true

	// 1. Test switching AltScreen OFF
	SetAltScreen(false)
	if inAltScreen {
		t.Error("inAltScreen should be false")
	}
	if !strings.Contains(mock.builder.String(), seqAltScreenOff) {
		t.Errorf("AltScreen OFF sequence missing, got %q", mock.builder.String())
	}

	mock.builder.Reset()

	// 2. Test switching AltScreen ON
	SetAltScreen(true)
	if !inAltScreen {
		t.Error("inAltScreen should be true")
	}
	if !strings.Contains(mock.builder.String(), seqAltScreenOn) {
		t.Errorf("AltScreen ON sequence missing, got %q", mock.builder.String())
	}
}

func TestTerminalEnv_Suspend(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// Simulate active TUI in AltScreen
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()

	if isPrepared {
		t.Error("isPrepared should be false after Suspend")
	}
	if inAltScreen {
		t.Error("inAltScreen should be false after Suspend")
	}

	output := mock.builder.String()
	if !strings.Contains(output, seqAltScreenOff) {
		t.Error("Suspend did not exit AltScreen")
	}
	if !strings.Contains(output, seqDefaultCursor) {
		t.Error("Suspend did not restore default cursor")
	}
}

func TestTerminalEnv_ManageCursorDisabled(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// 1. Disable cursor management
	ManageCursorStyle = false
	isPrepared = false
	inAltScreen = false
	inputRestore = func() {}

	// 2. Resume
	Resume()
	if strings.Contains(mock.builder.String(), seqBlinkingUnderline) {
		t.Error("seqBlinkingUnderline sent even though ManageCursorStyle is false")
	}

	mock.builder.Reset()

	// 3. Suspend
	isPrepared = true
	Suspend()
	if strings.Contains(mock.builder.String(), seqDefaultCursor) {
		t.Error("seqDefaultCursor sent even though ManageCursorStyle is false")
	}

	// Reset global state for other tests
	ManageCursorStyle = true
}
func TestTerminalEnv_AutoWrap(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// 1. Test Suspend restores AutoWrap (safely without calling vtinput.Enable)
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()

	output := mock.builder.String()
	if !strings.Contains(output, seqAutoWrapOn) {
		t.Error("Suspend did not restore auto-wrap")
	}

	// 2. Test Resume writes AutoWrapOff (ignore ioctl error in headless test environment)
	isPrepared = false
	inAltScreen = false
	inputRestore = nil
	mock.builder.Reset()

	_ = Resume()

	output = mock.builder.String()
	if !strings.Contains(output, seqAutoWrapOff) {
		t.Error("Resume did not write seqAutoWrapOff")
	}
}
func TestAnsiRendererCursorStyle(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	defer os.Setenv("TERM", oldTerm)

	tests := []struct {
		name       string
		term       string
		shape      CursorShape
		wantCursor string
	}{
		{
			name:       "Standard Underline",
			term:       "xterm-256color",
			shape:      CursorShapeUnderline,
			wantCursor: "\x1b[3 q",
		},
		{
			name:       "Standard Block",
			term:       "xterm-256color",
			shape:      CursorShapeBlock,
			wantCursor: "\x1b[1 q",
		},
		{
			name:       "Linux Console Underline",
			term:       "linux",
			shape:      CursorShapeUnderline,
			wantCursor: "\x1b[?3c",
		},
		{
			name:       "Linux Console Block",
			term:       "linux",
			shape:      CursorShapeBlock,
			wantCursor: "\x1b[?6c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("TERM", tc.term)

			scr := NewScreenBuf()
			var buf bytes.Buffer
			scr.Writer = &buf

			scr.AllocBuf(10, 10)
			scr.SetCursorPos(1, 1)
			scr.SetCursorVisible(true)
			scr.SetCursorShape(tc.shape)

			scr.Flush()

			out := buf.String()
			if !strings.Contains(out, tc.wantCursor) {
				t.Errorf("expected output to contain %q, got %q", tc.wantCursor, out)
			}
		})
	}
}

func TestSetCursorStyleOS(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetCursorStyleOS panicked: %v", r)
		}
	}()
	SetCursorStyleOS(true, CursorShapeUnderline)
	SetCursorStyleOS(false, CursorShapeBlock)
}
