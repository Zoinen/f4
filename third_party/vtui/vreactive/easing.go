package vreactive

// EasingFunc maps a normalized linear progress [0..1] to an eased progress value.
type EasingFunc func(t float64) float64

// Linear is the standard default linear progress function.
func Linear(t float64) float64 { return t }

// Quadratic easings
func EaseInQuad(t float64) float64  { return t * t }
func EaseOutQuad(t float64) float64 { return t * (2 - t) }
func EaseInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return -1 + (4-2*t)*t
}

// Cubic easings
func EaseInCubic(t float64) float64 { return t * t * t }
func EaseOutCubic(t float64) float64 {
	t -= 1
	return t*t*t + 1
}
func EaseInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	t -= 1
	return 1 + 4*t*t*t
}

// Back easings (overshooting transitions, corresponding to QML Easing.OutBack / InBack)
func EaseOutBack(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	t -= 1
	return 1 + c3*t*t*t + c1*t*t
}

func EaseInBack(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	return c3*t*t*t - c1*t*t
}

// Bounce easings
func EaseOutBounce(t float64) float64 {
	const n1 = 7.5625
	const d1 = 2.75

	if t < 1/d1 {
		return n1 * t * t
	} else if t < 2/d1 {
		t -= 1.5 / d1
		return n1*t*t + 0.75
	} else if t < 2.5/d1 {
		t -= 2.25 / d1
		return n1*t*t + 0.9375
	}
	t -= 2.625 / d1
	return n1*t*t + 0.984375
}
