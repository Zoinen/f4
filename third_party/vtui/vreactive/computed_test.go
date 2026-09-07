package vreactive

import "testing"

func TestComputed2(t *testing.T) {
	p1 := NewProperty(2)
	p2 := NewProperty(3)
	c := Computed2(p1, p2, func(a, b int) int { return a + b })

	if c.Get() != 5 {
		t.Errorf("expected 5, got %d", c.Get())
	}

	p1.Set(10)
	if c.Get() != 13 {
		t.Errorf("expected 13, got %d", c.Get())
	}

	p2.Set(20)
	if c.Get() != 30 {
		t.Errorf("expected 30, got %d", c.Get())
	}
}

func TestDiscreteBehavior(t *testing.T) {
	p := NewProperty(0)
	p.SetBehavior(&DiscreteBehavior[int]{})

	p.Set(100)
	if p.Get() != 100 {
		t.Errorf("expected 100, got %d", p.Get())
	}
}
