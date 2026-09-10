package media

import (
	"context"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
	"testing"
	"time"
)

func TestSlideShowInterval(t *testing.T) {
	was := config.App.SlideShowDelay
	t.Cleanup(func() { config.App.SlideShowDelay = was })

	config.App.SlideShowDelay = 3
	if got := slideShowInterval(); got != 3*time.Second {
		t.Errorf("a configured delay of three seconds gave %v", got)
	}

	for _, bad := range []int{0, -1} {
		config.App.SlideShowDelay = bad
		if got := slideShowInterval(); got != config.DefaultSlideShowDelay*time.Second {
			t.Errorf("a delay of %d gave %v instead of the default", bad, got)
		}
	}
}

func TestSlideStepWrapsAround(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	for _, name := range []string{"a.png", "b.png"} {
		if res := ImagePipe.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	iv.slideStep()
	if iv.Index != 1 || iv.Path != "b.png" {
		t.Fatalf("the first step went to %d, %q", iv.Index, iv.Path)
	}

	iv.slideStep()
	if iv.Index != 0 || iv.Path != "a.png" {
		t.Errorf("the step past the last picture went to %d, %q", iv.Index, iv.Path)
	}
}

func TestSlideShowStartsStopsAndCleansUp(t *testing.T) {
	withStubPipeline(t, 8, 8)
	restoreBars(t)

	iv := newTestImageView(t, 100, 100)
	iv.Path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)

	press := func() bool {
		e := &vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_S}
		e.ControlKeyState |= vtinput.LeftCtrlPressed
		return iv.ProcessKey(e)
	}

	if !press() || iv.SlideStop == nil {
		t.Fatal("Ctrl+S did not start the show")
	}
	if !press() || iv.SlideStop != nil {
		t.Fatal("Ctrl+S did not stop the show")
	}

	// The grid and the show turn each other off.
	press()
	iv.ToggleGallery()
	if iv.SlideStop != nil {
		t.Error("opening the grid must stop the show")
	}
	press()
	if iv.Gal != nil {
		t.Error("starting the show must close the grid")
	}

	iv.Close()
	if iv.SlideStop != nil {
		t.Error("a closed viewer must not leave a timer running")
	}

	lone := newTestImageView(t, 10, 10)
	lone.ToggleSlideShow()
	if lone.SlideStop != nil {
		t.Error("one picture on its own is not a slide show")
	}
}
