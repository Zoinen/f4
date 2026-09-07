package vreactive

import "testing"

func TestEasingFunctions(t *testing.T) {
	funcs := []struct {
		name string
		f    EasingFunc
	}{
		{"Linear", Linear},
		{"EaseInQuad", EaseInQuad},
		{"EaseOutQuad", EaseOutQuad},
		{"EaseInOutQuad", EaseInOutQuad},
		{"EaseInCubic", EaseInCubic},
		{"EaseOutCubic", EaseOutCubic},
		{"EaseInOutCubic", EaseInOutCubic},
		{"EaseInBack", EaseInBack},
		{"EaseOutBack", EaseOutBack},
		{"EaseOutBounce", EaseOutBounce},
	}

	for _, tc := range funcs {
		start := tc.f(0.0)
		end := tc.f(1.0)
		if start < -0.01 || start > 0.01 {
			t.Errorf("%s(0.0) = %f, want 0.0", tc.name, start)
		}
		if end < 0.99 || end > 1.01 {
			t.Errorf("%s(1.0) = %f, want 1.0", tc.name, end)
		}
		mid := tc.f(0.5)
		if mid < -1.0 || mid > 2.0 {
			t.Errorf("%s(0.5) = %f, out of reasonable bounds", tc.name, mid)
		}
	}
}

func TestRGBInterpolator(t *testing.T) {
	black := uint32(0x000000)
	white := uint32(0xFFFFFF)
	mid := RGBInterpolator(black, white, 0.5)
	if mid != 0x808080 {
		t.Errorf("expected 0x808080, got 0x%06X", mid)
	}
}

func TestComputedIf(t *testing.T) {
	cond := NewProperty(false)
	c := ComputedIf(cond, "Active", "Inactive")
	if c.Get() != "Inactive" {
		t.Errorf("expected Inactive, got %s", c.Get())
	}
	cond.Set(true)
	if c.Get() != "Active" {
		t.Errorf("expected Active, got %s", c.Get())
	}
}
