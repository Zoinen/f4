package wheel

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
)

func TestAccelFactor(t *testing.T) {
	base := time.Now()
	const accel = config.WheelAccelerationMax
	window := spinWindow
	cases := []struct {
		name       string
		now        time.Time
		hasLast    bool
		lastFactor int
		want       int
	}{
		{"first notch", base.Add(time.Second), false, 0, 1},
		{"spin over", base.Add(time.Second), true, 4, 1},
		{"clock went backwards", base.Add(-time.Millisecond), true, 4, 1},
		{"slow notch", base.Add(200 * time.Millisecond), true, 1, 2},
		{"faster notch", base.Add(290 * time.Millisecond), true, 8, 1},
		{"fast notch", base.Add(10 * time.Millisecond), true, 1, 2},
		{"fast notch on a running ramp", base.Add(10 * time.Millisecond), true, 4, 8},
		{"notch under the doubling cap", base.Add(250 * time.Millisecond), true, 4, 2},
		{"fastest notch", base.Add(time.Millisecond), true, config.WheelAccelerationMax, config.WheelAccelerationMax},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accelFactor(tc.now, base, tc.hasLast, tc.lastFactor, accel, window)
			if got != tc.want {
				t.Errorf("accelFactor() = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("weakest ramp", func(t *testing.T) {
		if got := accelFactor(base.Add(time.Millisecond), base, true, 8, config.WheelAccelerationMin, window); got != 1 {
			t.Errorf("accelFactor() at the weakest ramp = %d, want 1", got)
		}
	})
	t.Run("no spin window", func(t *testing.T) {
		if got := accelFactor(base.Add(time.Millisecond), base, true, 8, accel, 0); got != 1 {
			t.Errorf("accelFactor() with no window = %d, want 1", got)
		}
	})
}

func TestAccelClampsNonsense(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })

	for _, tc := range []struct{ in, want int }{
		{-5, config.WheelAccelerationMin},
		{0, config.WheelAccelerationMin},
		{config.WheelAccelerationMin, config.WheelAccelerationMin},
		{7, 7},
		{config.WheelAccelerationMax, config.WheelAccelerationMax},
		{999, config.WheelAccelerationMax},
	} {
		config.App.WheelAcceleration = tc.in
		if got := Accel(); got != tc.want {
			t.Errorf("Accel() with Acceleration = %d: got %d, want %d", tc.in, got, tc.want)
		}
	}
}

// countScroll is a stand-in view: it walks a line counter the way a panel
// walks its entries, and reports whether anything was left to move.
func countScroll(pos, limit *int) func(int) bool {
	return func(step int) bool {
		before := *pos
		*pos += step
		if *pos < 0 {
			*pos = 0
		}
		if *pos > *limit {
			*pos = *limit
		}
		return *pos != before
	}
}

func TestCoastSpinCoastsAhead(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	var pos, limit int
	limit = 1000
	scroll := countScroll(&pos, &limit)

	var c Coast
	base := time.Now()
	c.NotchAt(1, base, scroll)
	c.NotchAt(1, base.Add(10*time.Millisecond), scroll)
	c.NotchAt(1, base.Add(20*time.Millisecond), scroll)
	c.NotchAt(1, base.Add(30*time.Millisecond), scroll)
	want := 1 + 3 + 7 // factors 2, 4, 8 queue 1, 3, 7 extra lines
	if c.Pending() != want {
		t.Fatalf("queued lines = %d, want %d", c.Pending(), want)
	}

	steps := 0
	for c.Tick() {
		steps++
		if steps > 1000 {
			t.Fatal("the coast never stopped")
		}
	}
	if pos != want {
		t.Errorf("position after the coast = %d, want %d", pos, want)
	}
	if c.Pending() != 0 {
		t.Errorf("queued lines left after the coast = %d, want 0", c.Pending())
	}
}

func TestCoastWeakestRampQueuesNothing(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMin

	var pos, limit int
	limit = 1000
	scroll := countScroll(&pos, &limit)

	var c Coast
	base := time.Now()
	c.NotchAt(1, base, scroll)
	c.NotchAt(1, base.Add(10*time.Millisecond), scroll)
	c.NotchAt(1, base.Add(20*time.Millisecond), scroll)

	if c.Pending() != 0 {
		t.Errorf("queued %d lines at the weakest ramp, want none", c.Pending())
	}
	if c.IsCoasting() {
		t.Error("the weakest ramp started the coast timer")
	}
}

func TestCoastDirectionChangeDropsQueue(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	var pos, limit int
	limit = 1000
	scroll := countScroll(&pos, &limit)

	var c Coast
	base := time.Now()
	c.NotchAt(1, base, scroll)
	c.NotchAt(1, base.Add(10*time.Millisecond), scroll)
	if c.Pending() == 0 {
		t.Fatal("test setup: the spin queued nothing")
	}

	c.NotchAt(-1, base.Add(20*time.Millisecond), scroll)

	if c.Pending() != 0 {
		t.Errorf("queued lines after the reversal = %d, want 0", c.Pending())
	}
	if c.Direction() != -1 {
		t.Errorf("direction after the reversal = %d, want -1", c.Direction())
	}
	if c.IsCoasting() {
		t.Error("the reversal left the coast timer running")
	}
}

func TestCoastStopDropsEverything(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	var pos, limit int
	limit = 1000
	scroll := countScroll(&pos, &limit)

	var c Coast
	base := time.Now()
	c.NotchAt(1, base, scroll)
	c.NotchAt(1, base.Add(10*time.Millisecond), scroll)
	if c.Pending() == 0 {
		t.Fatal("test setup: the spin queued nothing")
	}

	c.Stop()

	if c.Pending() != 0 || c.Direction() != 0 || c.IsCoasting() {
		t.Errorf("after Stop: pending %d, direction %d, coasting %v, want all clear",
			c.Pending(), c.Direction(), c.IsCoasting())
	}
}

func TestCoastStopsWhenTheViewCannotMove(t *testing.T) {
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.WheelAcceleration = config.WheelAccelerationMax

	// A ten-line view parked at its end: every step the coast asks for
	// lands where it already is, so the very first step ends the coast.
	const limit = 10
	pos := limit
	blocked := 0
	scroll := func(step int) bool {
		before := pos
		pos += step
		if pos < 0 {
			pos = 0
		}
		if pos > limit {
			pos = limit
		}
		if pos == before {
			blocked++
		}
		return pos != before
	}

	var c Coast
	base := time.Now()
	for i := 0; i < 30; i++ {
		c.NotchAt(1, base.Add(time.Duration(i*10)*time.Millisecond), scroll)
	}
	if c.Pending() == 0 {
		t.Fatal("test setup: the spin queued nothing")
	}

	steps := 0
	for c.Tick() {
		steps++
		if steps > 1000 {
			t.Fatal("the coast never stopped at the end of the view")
		}
	}
	if blocked == 0 {
		t.Error("the coast never hit the end of the view")
	}
	if pos != limit {
		t.Errorf("position = %d, want the end of the view %d", pos, limit)
	}
}
