package vtui

import "testing"

type maximizerRenderer struct {
	AnsiRenderer
	toggles int
	answer  bool
}

func (r *maximizerRenderer) ToggleMaximized() bool {
	r.toggles++
	return r.answer
}

func withActiveBackend(t *testing.T, name string) {
	t.Helper()
	backendMu.Lock()
	prev := activeBackend
	activeBackend = name
	backendMu.Unlock()
	t.Cleanup(func() {
		backendMu.Lock()
		activeBackend = prev
		backendMu.Unlock()
	})
}

// Alt+F9 goes to the renderer whenever the renderer can maximize its window,
// and its answer is what the caller gets.
func TestToggleWindowMaximized_UsesRenderer(t *testing.T) {
	withActiveBackend(t, "x11")
	for _, answer := range []bool{true, false} {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(10, 5)
		r := &maximizerRenderer{answer: answer}
		scr.Renderer = r
		fm := &frameManager{scr: scr}
		if got := fm.ToggleWindowMaximized(); got != answer {
			t.Errorf("ToggleWindowMaximized = %v, want the renderer's %v", got, answer)
		}
		if r.toggles != 1 {
			t.Errorf("renderer toggled %d times, want 1", r.toggles)
		}
	}
}

// A GUI backend whose renderer cannot maximize must not reach for the console
// window behind it: that is not the window on screen.
func TestToggleWindowMaximized_GUIWithoutMaximizer(t *testing.T) {
	withActiveBackend(t, "gui-without-maximize")
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	scr.Renderer = &AnsiRenderer{parent: scr}
	fm := &frameManager{scr: scr}
	if fm.ToggleWindowMaximized() {
		t.Error("ToggleWindowMaximized = true for a GUI renderer that cannot maximize")
	}
}

// The GUI renderers all offer the toggle.
func TestGUIRenderersCanMaximize(t *testing.T) {
	var _ windowMaximizer = (*Win32GuiRenderer)(nil)
}
