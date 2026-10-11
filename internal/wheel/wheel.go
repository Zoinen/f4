package wheel

import (
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// Wheel scrolling follows the idea of Max Rusov's FastWheel FAR plugin: a
// fast spin queues extra lines that the view keeps scrolling on its own, so
// the cursor picks up speed instead of stopping at the last notch the wheel
// reported. One notch still scrolls the configured lines at once; the queue
// is the acceleration on top of that.
//
// The ramp shape lives in code: the spin window, the coast step and the gap
// between two steps are constants here. The one tuning knob the user gets is
// the strength of the ramp, [Mouse] Acceleration in settings.ini.
const (
	// spinWindow is the longest gap between two notches that still reads
	// as one continuous spin.
	spinWindow = 300 * time.Millisecond
	// coastStepMax bounds one coast step, so a deep queue still moves the
	// view in readable jumps instead of teleporting it.
	coastStepMax = 100
	// coastStepDelay is the pause between two coast steps.
	coastStepDelay = 25 * time.Millisecond
	// maxPending caps the queue, so a long spin cannot keep the view
	// moving for long after the wheel has stopped turning.
	maxPending = 200
)

// Accel returns the ramp strength from the settings, clamped into
// [config.WheelAccelerationMin, config.WheelAccelerationMax]:
// settings.ini is user text, and no value in it may make the view scroll at
// nonsense speed.
func Accel() int {
	a := config.App.WheelAcceleration
	if a < config.WheelAccelerationMin {
		return config.WheelAccelerationMin
	}
	if a > config.WheelAccelerationMax {
		return config.WheelAccelerationMax
	}
	return a
}

// Coast is what a fast spin leaves behind on a view: lines the view still
// owes the cursor, and the state of the ramp that queued them. The owner
// passes its own scroll function on every notch; the coast keeps calling it
// until the queue drains or the view stops moving.
type Coast struct {
	scroll     func(step int) bool
	lastNotch  time.Time
	hasNotch   bool
	lastFactor int
	direction  int // +1 down/forward, -1 up/back; 0 when idle
	pending    int // lines the coast still owes
	generation uint64
	timer      *time.Timer
}

// Notch records a wheel notch: the owner scrolls its own lines for the notch
// right away, and the coast remembers the extra lines a fast spin queued.
func (c *Coast) Notch(direction int, scroll func(step int) bool) {
	c.NotchAt(direction, time.Now(), scroll)
}

// NotchAt is Notch with an explicit timestamp, so tests can drive the ramp
// without waiting on the clock.
func (c *Coast) NotchAt(direction int, now time.Time, scroll func(step int) bool) {
	c.scroll = scroll
	if direction != c.direction {
		c.Stop()
		c.direction = direction
		c.hasNotch = false
		c.lastFactor = 1
	}
	factor := accelFactor(now, c.lastNotch, c.hasNotch, c.lastFactor, Accel(), spinWindow)
	c.lastNotch = now
	c.hasNotch = true
	c.lastFactor = factor
	if factor <= 1 {
		return
	}
	extra := factor - 1
	if c.pending+extra > maxPending {
		c.pending = maxPending
	} else {
		c.pending += extra
	}
	if c.timer != nil {
		return
	}
	c.schedule(c.generation)
}

// Stop drops the queued lines and invalidates the timer, so a step already
// posted to the UI loop finds nothing to scroll.
func (c *Coast) Stop() {
	c.generation++
	c.pending = 0
	c.direction = 0
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
}

// Tick scrolls one coast step. It reports whether the coast has more to do:
// false when the queue drained or the view stopped moving (an end of the
// list), and the caller stops the timer.
func (c *Coast) Tick() bool {
	step := c.pending/10 + 1
	if step > coastStepMax {
		step = coastStepMax
	}
	if step > c.pending {
		step = c.pending
	}
	if step <= 0 {
		return false
	}
	moved := false
	if c.scroll != nil {
		moved = c.scroll(c.direction * step)
	}
	c.pending -= step
	return moved && c.pending > 0
}

// Pending reports the lines the coast still owes, Direction the way it is
// moving them, and IsCoasting whether the timer is running; all three answer
// what a view's ramp is doing right now.
func (c *Coast) Pending() int     { return c.pending }
func (c *Coast) Direction() int   { return c.direction }
func (c *Coast) IsCoasting() bool { return c.timer != nil }

func (c *Coast) schedule(generation uint64) {
	// Captured here, where the work is started, not inside the callback:
	// the frame manager is process-wide, and background work that reads it
	// while a test swaps it is exactly what the frame-manager audit bans.
	frameManager := vtui.FrameManager
	if frameManager == nil {
		return
	}
	c.timer = time.AfterFunc(coastStepDelay, func() {
		frameManager.PostTask(func() {
			if generation != c.generation || c.pending == 0 {
				return
			}
			if !c.Tick() {
				c.Stop()
				return
			}
			c.schedule(generation)
		})
	})
}

// accelFactor returns how many lines a notch queues: 1 for a lone notch, up
// to accel for a notch that follows the previous one immediately, and always
// 1 while the ramp is at its weakest. The ramp doubles per notch, so the
// view speeds up over a spin instead of jumping to full speed on the second
// turn.
func accelFactor(now, last time.Time, hasLast bool, lastFactor, accel int, window time.Duration) int {
	if accel <= 0 || window <= 0 || !hasLast || last.IsZero() {
		return 1
	}
	period := now.Sub(last)
	if period < 0 || period >= window {
		return 1
	}
	elapsed := window - period
	factor := int((elapsed*time.Duration(accel) + window/2) / window)
	if factor < 1 {
		factor = 1
	}
	if lastFactor < 1 {
		lastFactor = 1
	}
	if max := lastFactor * 2; factor > max {
		factor = max
	}
	if factor > accel {
		factor = accel
	}
	return factor
}
