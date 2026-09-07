package vreactive

// Bind automatically sets dest to the value of src whenever src changes.
func Bind[T any](dest Property[T], src Property[T]) {
	src.OnChange(func(val T) {
		dest.Set(val)
	})
	dest.Set(src.Get())
}

// Computed creates a read-only (in terms of design) property that reacts to dep.
func Computed[T any, A any](dep Property[A], compute func(A) T) Property[T] {
	p := NewProperty(compute(dep.Get()))
	dep.OnChange(func(val A) {
		p.Set(compute(val))
	})
	return p
}

// Computed2 creates a property depending on two reactive sources.
func Computed2[T any, A, B any](dep1 Property[A], dep2 Property[B], compute func(A, B) T) Property[T] {
	p := NewProperty(compute(dep1.Get(), dep2.Get()))
	update := func() {
		p.Set(compute(dep1.Get(), dep2.Get()))
	}
	dep1.OnChange(func(A) { update() })
	dep2.OnChange(func(B) { update() })
	return p
}

// ComputedIf creates a reactive property that evaluates to whenTrue when cond is true, or whenFalse otherwise.
func ComputedIf[T any](cond Property[bool], whenTrue, whenFalse T) Property[T] {
	return Computed(cond, func(c bool) T {
		if c {
			return whenTrue
		}
		return whenFalse
	})
}
