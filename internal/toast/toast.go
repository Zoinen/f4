// Package toast is f4's transient notification: a line the user reads and
// does not dismiss. It is the application's user-facing notification channel,
// which is why nothing here logs.
package toast

import (
	"github.com/unxed/vtui"
	"time"
)

// DurationOverride lets tests shorten f4-owned toast lifetimes without
// changing vtui's timer. Production leaves it nil and uses each caller's
// requested duration unchanged; it is a test seam and nothing else.
var DurationOverride func(time.Duration) time.Duration

// Show displays a toast and reports the duration actually used.
func Show(message string, duration time.Duration) time.Duration {
	if DurationOverride != nil {
		duration = DurationOverride(duration)
	}
	vtui.ShowToast(message, duration)
	return duration
}
