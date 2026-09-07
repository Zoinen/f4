package main

import (
	"time"

	"github.com/unxed/vtui"
)

// defaultSlideShowDelay is how many seconds a picture stays on screen when
// the configuration has nothing sensible to say about it.
const defaultSlideShowDelay = 5

// slideShowInterval is how long one picture is shown. A configured zero would
// spin the terminal as fast as it can decode and a negative value would never
// fire at all, so both fall back to the default.
func slideShowInterval() time.Duration {
	seconds := AppConfig.SlideShowDelay
	if seconds <= 0 {
		seconds = defaultSlideShowDelay
	}
	return time.Duration(seconds) * time.Second
}

// ToggleSlideShow starts or stops walking the pictures on a timer. The timer
// lives in a goroutine of its own and does nothing but ask the UI thread to
// take the next step: the index, the pipeline and the placement all belong to
// that thread.
func (iv *ImageView) ToggleSlideShow() {
	if iv.slideStop != nil {
		iv.stopSlideShow()
		return
	}
	if len(iv.siblings) < 2 {
		// One picture is not a slide show.
		return
	}

	// The grid and the show cannot both decide which picture is current, and
	// the show is the one that was just asked for.
	iv.gal = nil

	stop := make(chan struct{})
	iv.slideStop = stop
	interval := slideShowInterval()

	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				frames.PostTask(func() {
					// The reader may have stopped the show between the tick
					// and this task reaching the UI thread.
					if iv.slideStop == stop {
						iv.slideStep()
					}
				})
			}
		}
	}()
}

// stopSlideShow puts the timer away. Closing the channel is what the goroutine
// is waiting for, so it wakes up at once rather than at the end of the
// interval, and a viewer that is gone leaves nothing running behind it.
func (iv *ImageView) stopSlideShow() {
	if iv.slideStop == nil {
		return
	}
	close(iv.slideStop)
	iv.slideStop = nil
}

// slideStep shows the next picture and wraps around at the end. Step stops at
// the ends of the directory on purpose, so that it stays obvious where it
// begins and where it ends; a show that stopped on the last picture would
// only be a slow way of pressing space.
func (iv *ImageView) slideStep() {
	total := len(iv.siblings)
	if total == 0 {
		iv.stopSlideShow()
		return
	}
	next := iv.index + 1
	if next < 0 || next >= total {
		next = 0
	}
	iv.GoTo(next)
	vtui.FrameManager.Redraw()
}
