package vtui

import (
	"bytes"
	"strings"
	"testing"
)

func TestSession_IODecouplingBytesBuffer(t *testing.T) {
	SetDefaultPalette()

	var buf bytes.Buffer
	scr := NewScreenBuf()
	scr.SetOutput(&buf)
	scr.AllocBuf(40, 10)
	scr.Renderer = &AnsiRenderer{parent: scr}

	dlg := NewDialog(0, 0, 30, 8, " Hello Session ")
	dlg.SetID("dlg1")
	btn := NewButton(dlg.X1+10, dlg.Y1+5, "Submit")
	btn.SetID("btn1")
	dlg.AddItem(btn)

	dlg.Show(scr)
	scr.Flush()

	output := buf.String()
	if output == "" {
		t.Fatal("Expected non-empty ANSI output written to bytes.Buffer")
	}

	// Verify title presence in ANSI output stream
	if !strings.Contains(output, "Hello Session") {
		t.Errorf("ANSI stream missing dialog title: %q", output)
	}
	// Verify button text presence in ANSI output stream
	if !strings.Contains(output, "Submit") {
		t.Errorf("ANSI stream missing button text: %q", output)
	}

	// Stability check: flush again without changes should be empty
	buf.Reset()
	scr.Flush()
	if buf.Len() != 0 {
		t.Errorf("Subsequent flush on unchanged buffer produced output: %q", buf.String())
	}
}

func TestSession_FrameManagerResize(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	f := &mockFrame{}
	fm.Push(f)

	if scr.Width() != 80 || scr.Height() != 25 {
		t.Fatalf("Initial size mismatch: %dx%d", scr.Width(), scr.Height())
	}

	fm.Resize(100, 35)

	if scr.Width() != 100 || scr.Height() != 35 {
		t.Errorf("Expected resized ScreenBuf to be 100x35, got %dx%d", scr.Width(), scr.Height())
	}
	if f.resizedW != 100 || f.resizedH != 35 {
		t.Errorf("Expected frame to be resized to 100x35, got %dx%d", f.resizedW, f.resizedH)
	}
}
