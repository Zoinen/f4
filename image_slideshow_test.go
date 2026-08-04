package main

import (
	"context"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestSlideShowInterval(t *testing.T) {
	was := AppConfig.SlideShowDelay
	t.Cleanup(func() { AppConfig.SlideShowDelay = was })

	AppConfig.SlideShowDelay = 3
	if got := slideShowInterval(); got != 3*time.Second {
		t.Errorf("a configured delay of three seconds gave %v", got)
	}

	for _, bad := range []int{0, -1} {
		AppConfig.SlideShowDelay = bad
		if got := slideShowInterval(); got != defaultSlideShowDelay*time.Second {
			t.Errorf("a delay of %d gave %v instead of the default", bad, got)
		}
	}
}

func TestSlideStepWrapsAround(t *testing.T) {
	withStubPipeline(t, 8, 8)

	iv := newTestImageView(t, 100, 100)
	iv.path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)
	for _, name := range []string{"a.png", "b.png"} {
		if res := ImagePipe.LoadSync(context.Background(), nil, name); res.Err != nil {
			t.Fatalf("%s: %v", name, res.Err)
		}
	}

	iv.slideStep()
	if iv.index != 1 || iv.path != "b.png" {
		t.Fatalf("the first step went to %d, %q", iv.index, iv.path)
	}

	iv.slideStep()
	if iv.index != 0 || iv.path != "a.png" {
		t.Errorf("the step past the last picture went to %d, %q", iv.index, iv.path)
	}
}

func TestSlideShowStartsStopsAndCleansUp(t *testing.T) {
	withStubPipeline(t, 8, 8)
	restoreBars(t)

	iv := newTestImageView(t, 100, 100)
	iv.path = "a.png"
	iv.SetSiblings([]string{"a.png", "b.png"}, 0)

	press := func() bool {
		e := &vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_S}
		e.ControlKeyState |= vtinput.LeftCtrlPressed
		return iv.ProcessKey(e)
	}

	if !press() || iv.slideStop == nil {
		t.Fatal("Ctrl+S did not start the show")
	}
	if !press() || iv.slideStop != nil {
		t.Fatal("Ctrl+S did not stop the show")
	}

	// The grid and the show turn each other off.
	press()
	iv.ToggleGallery()
	if iv.slideStop != nil {
		t.Error("opening the grid must stop the show")
	}
	press()
	if iv.gal != nil {
		t.Error("starting the show must close the grid")
	}

	iv.Close()
	if iv.slideStop != nil {
		t.Error("a closed viewer must not leave a timer running")
	}

	lone := newTestImageView(t, 10, 10)
	lone.ToggleSlideShow()
	if lone.slideStop != nil {
		t.Error("one picture on its own is not a slide show")
	}
}
