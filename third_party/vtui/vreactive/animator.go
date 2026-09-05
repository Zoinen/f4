package vreactive

type DiscreteAnimator[T any] struct {
	Target T
}

func (a *DiscreteAnimator[T]) Tick(dt float64) (T, bool) {
	return a.Target, true
}

type DiscreteBehavior[T any] struct{}

func (b *DiscreteBehavior[T]) CreateAnimator(start, end T) Animator[T] {
	return &DiscreteAnimator[T]{Target: end}
}

type Interpolator[T any] func(start, end T, progress float64) T

type SmoothAnimator[T any] struct {
	Start    T
	End      T
	Duration float64
	Elapsed  float64
	Easing   EasingFunc
	Interp   Interpolator[T]
}

func (a *SmoothAnimator[T]) Tick(dt float64) (T, bool) {
	a.Elapsed += dt
	if a.Elapsed >= a.Duration {
		return a.End, true
	}
	p := a.Elapsed / a.Duration
	if a.Easing != nil {
		p = a.Easing(p)
	}
	return a.Interp(a.Start, a.End, p), false
}

// SmoothBehavior smoothly interpolates property transitions over Duration with optional Easing.
type SmoothBehavior[T any] struct {
	Duration float64
	Easing   EasingFunc
	Interp   Interpolator[T]
}

func (b *SmoothBehavior[T]) CreateAnimator(start, end T) Animator[T] {
	return &SmoothAnimator[T]{
		Start:    start,
		End:      end,
		Duration: b.Duration,
		Easing:   b.Easing,
		Interp:   b.Interp,
	}
}

func Float64Interpolator(start, end float64, progress float64) float64 {
	return start + (end-start)*progress
}

func IntInterpolator(start, end int, progress float64) int {
	return start + int(float64(end-start)*progress)
}

// RGBInterpolator smoothly blends between two 24-bit 0xRRGGBB colors across RGB channels.
func RGBInterpolator(start, end uint32, progress float64) uint32 {
	clamp := func(v float64) uint8 {
		if v <= 0 {
			return 0
		}
		if v >= 255 {
			return 255
		}
		return uint8(v + 0.5)
	}
	r1, g1, b1 := (start>>16)&0xFF, (start>>8)&0xFF, start&0xFF
	r2, g2, b2 := (end>>16)&0xFF, (end>>8)&0xFF, end&0xFF
	r := clamp(float64(r1) + float64(int(r2)-int(r1))*progress)
	g := clamp(float64(g1) + float64(int(g2)-int(g1))*progress)
	b := clamp(float64(b1) + float64(int(b2)-int(b1))*progress)
	return (uint32(r) << 16) | (uint32(g) << 8) | uint32(b)
}
