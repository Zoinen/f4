package vtui

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// TestSetCursorPos_DoesNotChangeVisibility covers f4 issue #518: an
// out-of-range position used to silently clear cursorVisible, so the same
// coordinate hid the caret in Edit (which shows then positions) and did
// not in EditorView (which positions then shows).
func TestSetCursorPos_DoesNotChangeVisibility(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)

	scr.SetCursorVisible(true)
	scr.SetCursorPos(100, 100)
	x, y, visible, _ := scr.GetCursorStateForTesting()
	if !visible {
		t.Error("an out-of-range position turned the caret off")
	}
	if x != 19 || y != 9 {
		t.Errorf("position clamped to (%d,%d), want (19,9)", x, y)
	}

	scr.SetCursorPos(-5, -5)
	x, y, visible, _ = scr.GetCursorStateForTesting()
	if !visible {
		t.Error("a negative position turned the caret off")
	}
	if x != 0 || y != 0 {
		t.Errorf("position clamped to (%d,%d), want (0,0)", x, y)
	}

	// Hiding is still exactly what SetCursorVisible is for.
	scr.SetCursorVisible(false)
	if _, _, visible, _ = scr.GetCursorStateForTesting(); visible {
		t.Error("SetCursorVisible(false) did not hide the caret")
	}
}

// shortWriter accepts at most limit bytes per call, the way a tty under
// back-pressure can, and records everything it took.
type shortWriter struct {
	limit int
	got   strings.Builder
	fail  bool
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("write failed")
	}
	n := len(p)
	if n > w.limit {
		n = w.limit
	}
	w.got.Write(p[:n])
	return n, nil
}

// TestAnsiRenderer_WriteHonorsShortWrites covers f4 issue #518: the chunked
// write advanced by the length it offered rather than the length actually
// taken, so a short write dropped the rest of its chunk. Every painted
// frame opens with ESC[?25l and only restores the caret at the very end, so
// a frame cut in the middle left the caret hidden.
func TestAnsiRenderer_WriteHonorsShortWrites(t *testing.T) {
	scr := NewSilentScreenBuf()
	w := &shortWriter{limit: 7}
	scr.Writer = w
	r := &AnsiRenderer{parent: scr}

	payload := strings.Repeat("abcdefghij", 2000) + "\x1b[?25h"
	r.write(payload)

	if got := w.got.String(); got != payload {
		t.Errorf("wrote %d of %d bytes; the tail was dropped", len(got), len(payload))
	}
}

// TestAnsiRenderer_WriteStopsOnError makes sure a dead tty ends the loop
// instead of spinning on it forever.
func TestAnsiRenderer_WriteStopsOnError(t *testing.T) {
	scr := NewSilentScreenBuf()
	w := &shortWriter{limit: 7, fail: true}
	scr.Writer = w
	r := &AnsiRenderer{parent: scr}

	done := make(chan struct{})
	go func() {
		r.write(strings.Repeat("x", 100))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("write did not give up on a failing writer")
	}
}
