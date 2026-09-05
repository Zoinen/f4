package vreactive

import "testing"

func TestSmoothAnimator(t *testing.T) {
	b := &SmoothBehavior[float64]{
		Duration: 1.0,
		Interp:   Float64Interpolator,
	}

	anim := b.CreateAnimator(0, 100)

	val, done := anim.Tick(0.5)
	if val != 50 || done {
		t.Errorf("expected 50, false, got %v, %v", val, done)
	}

	val, done = anim.Tick(0.5)
	if val != 100 || !done {
		t.Errorf("expected 100, true, got %v, %v", val, done)
	}
}

func TestIntInterpolator(t *testing.T) {
	if v := IntInterpolator(0, 100, 0.5); v != 50 {
		t.Errorf("expected 50, got %v", v)
	}
}
