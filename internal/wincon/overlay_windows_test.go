//go:build windows

package wincon

import (
	"strings"
	"testing"
)

func TestOverlayZeroValueLifecycle(t *testing.T) {
	var o Overlay
	if got := o.Stats(); got != (Stats{}) {
		t.Fatalf("zero overlay stats = %+v, want zero", got)
	}
	if err := o.Place(Rect{X: 1, Y: 2, W: 3, H: 4}); err != nil {
		t.Fatalf("Place failed: %v", err)
	}
	if !o.Visible() {
		t.Fatal("Place did not make the overlay visible")
	}
	if got := o.st.currentRect(); got != (Rect{X: 1, Y: 2, W: 3, H: 4}) {
		t.Fatalf("currentRect = %+v, want {1 2 3 4}", got)
	}

	o.Hide()
	if o.Visible() {
		t.Fatal("Hide left the overlay visible")
	}
	if err := o.Place(Rect{}); err != nil {
		t.Fatalf("empty Place failed: %v", err)
	}
	if o.SetBounds([]Rect{{X: 1, Y: 2, W: 3, H: 4}}) {
		t.Fatal("SetBounds reported success without a window")
	}
	if _, _, ok := o.ClientSize(); ok {
		t.Fatal("ClientSize reported a size for a zero target")
	}

	o.Close()
	if !o.st.isClosed() {
		t.Fatal("Close did not close the overlay state")
	}
	if err := o.Place(Rect{W: 1, H: 1}); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Place after Close error = %v, want closed error", err)
	}
	o.Close()
}

func TestOverlayNoWindowPaths(t *testing.T) {
	var nilOverlay *Overlay
	if got := nilOverlay.Stats(); got != (Stats{}) {
		t.Fatalf("nil overlay stats = %+v, want zero", got)
	}

	var o Overlay
	o.post()
	o.push(0)
	o.track(0)
	if !o.st.isClosed() {
		t.Fatal("tracking a dead console did not close the overlay")
	}
	if o.SetBounds(nil) {
		t.Fatal("SetBounds reported success without a window")
	}
	if _, _, ok := o.ClientSize(); ok {
		t.Fatal("ClientSize reported a size for a zero target")
	}

	withWindow := Overlay{hwnd: 1}
	if !withWindow.SetBounds(nil) {
		t.Fatal("SetBounds reported failure with a window")
	}
	withWindow.Close()
	withWindow.Close()

	hide := Overlay{}
	hide.st.shown = true
	hide.apply(0)
	if got := hide.Stats(); got.Applies != 1 || hide.tracker.OnScreen {
		t.Fatalf("hide apply = stats %+v, tracker %+v; want one apply and hidden tracker", got, hide.tracker)
	}
}

func TestOverlayNewRejectsMissingTrustedConsole(t *testing.T) {
	_, source := ConsoleWindow()
	if source.Trusted() {
		t.Skip("the test process has a trusted classic console")
	}
	if overlay, err := New(); overlay != nil || err == nil {
		t.Fatalf("New() = overlay %v, error %v; want no overlay and an error", overlay, err)
	}
}

func TestOverlayDrawValidationAndUnstartedWindow(t *testing.T) {
	var o Overlay
	for _, test := range []struct {
		name   string
		pix    []byte
		w, h   int
		stride int
	}{
		{name: "zero width", pix: make([]byte, 4), w: 0, h: 1, stride: 4},
		{name: "short stride", pix: make([]byte, 4), w: 2, h: 1, stride: 4},
		{name: "short buffer", pix: make([]byte, 4), w: 1, h: 2, stride: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := o.Draw(test.pix, test.w, test.h, test.stride); err == nil || !strings.Contains(err.Error(), "needs") {
				t.Fatalf("Draw error = %v, want a size validation error", err)
			}
		})
	}

	pix := []byte{1, 2, 3, 255}
	if err := o.Draw(pix, 1, 1, 4); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Fatalf("Draw without a window error = %v, want closed error", err)
	}
	o.mu.Lock()
	got := append([]byte(nil), o.pix...)
	o.mu.Unlock()
	if want := []byte{3, 2, 1, 255}; string(got) != string(want) {
		t.Fatalf("Draw cached pixels = %v, want %v", got, want)
	}

	o.mu.Lock()
	o.hwnd = 1
	o.mu.Unlock()
	if err := o.Draw(pix, 1, 1, 4); err != nil {
		t.Fatalf("Draw with an unstarted pump failed: %v", err)
	}
}

func TestOverlaySystemQueriesHaveConsistentResults(t *testing.T) {
	if hwnd, source := ConsoleWindow(); (hwnd == 0) != (source == SourceNone) {
		t.Fatalf("ConsoleWindow returned hwnd=%#x with source=%v", hwnd, source)
	}
	if w, h, ok := CellSize(); ok && (w <= 0 || h <= 0) {
		t.Fatalf("CellSize returned invalid dimensions %dx%d", w, h)
	}
	if w, h, ok := GridSize(); ok && (w <= 0 || h <= 0) {
		t.Fatalf("GridSize returned invalid dimensions %dx%d", w, h)
	}
}

func TestWndProcIgnoresSyncForUnknownWindow(t *testing.T) {
	if got := wndProc(123, wmOverlaySync, 0, 0); got != 0 {
		t.Fatalf("wndProc returned %#x for an unregistered window", got)
	}
}

func TestWndProcHandlesOverlayMessages(t *testing.T) {
	const hwnd = uintptr(123)
	regMu.Lock()
	reg[hwnd] = &Overlay{}
	regMu.Unlock()
	t.Cleanup(func() {
		regMu.Lock()
		delete(reg, hwnd)
		regMu.Unlock()
	})

	if got := wndProc(hwnd, wmOverlaySync, 0, 0); got != 0 {
		t.Fatalf("sync wndProc returned %#x, want 0", got)
	}
	if got := wndProc(hwnd, wmOverlayQuit, 0, 0); got != 0 {
		t.Fatalf("quit wndProc returned %#x, want 0", got)
	}
	if got := wndProc(hwnd, wmDestroy, 0, 0); got != 0 {
		t.Fatalf("destroy wndProc returned %#x, want 0", got)
	}
	_ = wndProc(hwnd, 0x1234, 0, 0)
}
