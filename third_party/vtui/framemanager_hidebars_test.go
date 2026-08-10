package vtui

import "testing"

func TestFrameManagerHideBars(t *testing.T) {
	old := FrameManager
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm := &frameManager{}
	fm.Init(scr)
	FrameManager = fm
	t.Cleanup(func() { FrameManager = old })

	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	fm.KeyBar = kb

	f := &mockFrame{}
	f.SetPosition(0, 0, 79, 24)
	fm.Push(f)

	fm.renderPhase()
	if !kb.IsVisible() {
		t.Fatal("the key bar is drawn by default")
	}

	// SetVisible(false) alone would not survive the next frame, because
	// ScreenObject.Show forces the object visible again.
	kb.SetVisible(false)
	fm.renderPhase()
	if !kb.IsVisible() {
		t.Fatal("this test no longer checks what it was written for")
	}

	fm.HideBars = true
	fm.renderPhase()
	if kb.IsVisible() {
		t.Error("a frame that asked for the whole screen still got a key bar")
	}

	fm.HideBars = false
	fm.renderPhase()
	if !kb.IsVisible() {
		t.Error("the key bar did not come back")
	}
}
