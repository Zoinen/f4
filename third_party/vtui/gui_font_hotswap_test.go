package vtui

import "testing"

// vtui #136: GUI backends that can hot-swap the window font implement
// fontSetter; frameManager.SetFont dispatches to it and reports whatever it
// reports, exactly like ToggleWindowMaximized does for windowMaximizer.

type fontSetterRenderer struct {
	AnsiRenderer
	calls   int
	gotName string
	gotSize float64
	answer  bool
}

func (r *fontSetterRenderer) SetFont(fontName string, fontSize float64) bool {
	r.calls++
	r.gotName = fontName
	r.gotSize = fontSize
	return r.answer
}

func TestFrameManagerSetFont_UsesRenderer(t *testing.T) {
	for _, answer := range []bool{true, false} {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(10, 5)
		r := &fontSetterRenderer{answer: answer}
		scr.Renderer = r
		fm := &frameManager{scr: scr}

		if got := fm.SetFont("Comic Sans", 14); got != answer {
			t.Errorf("SetFont = %v, want the renderer's %v", got, answer)
		}
		if r.calls != 1 || r.gotName != "Comic Sans" || r.gotSize != 14 {
			t.Errorf("renderer got (%q, %v) x%d calls, want (\"Comic Sans\", 14) x1", r.gotName, r.gotSize, r.calls)
		}
	}
}

// A backend whose renderer cannot hot-swap the font (every GUI backend
// besides Wayland, X11, Win32, gogpu and ebiten, as of this part of #136)
// must report false, not panic or silently pretend to have applied the
// change.
func TestFrameManagerSetFont_UnsupportedRenderer(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	scr.Renderer = &AnsiRenderer{parent: scr}
	fm := &frameManager{scr: scr}
	if fm.SetFont("Comic Sans", 14) {
		t.Error("SetFont = true for a renderer that does not implement fontSetter")
	}
}

func TestFrameManagerSetFont_NoScreen(t *testing.T) {
	fm := &frameManager{}
	if fm.SetFont("Comic Sans", 14) {
		t.Error("SetFont = true with no screen/renderer at all")
	}
}
