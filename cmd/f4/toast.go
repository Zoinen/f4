package main

import (
	"time"

	"github.com/unxed/vtui"
)

// toastDurationOverride lets tests shorten f4-owned toast lifetimes without
// changing vtui's timer. Production leaves it nil and uses each caller's
// requested duration unchanged.
var toastDurationOverride func(time.Duration) time.Duration

func showToast(message string, duration time.Duration) time.Duration {
	if toastDurationOverride != nil {
		duration = toastDurationOverride(duration)
	}
	vtui.ShowToast(message, duration)
	return duration
}
