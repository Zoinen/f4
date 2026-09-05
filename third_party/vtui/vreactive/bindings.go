package vreactive

// Effect runs fn immediately and re-executes it whenever any dependency changes.
// It represents the standard Signal Effect pattern (SolidJS / Svelte / Vue).
func Effect(fn func(), deps ...Watcher) func() {
	if fn == nil {
		return func() {}
	}
	fn()
	var unsubs []func()
	for _, dep := range deps {
		if dep != nil {
			unsubs = append(unsubs, dep.Watch(fn))
		}
	}
	return func() {
		for _, u := range unsubs {
			u()
		}
	}
}

// BindTo immediately applies prop.Get() to target and subscribes to all future updates.
func BindTo[T any](prop Property[T], target func(T)) func() {
	if prop == nil || target == nil {
		return func() {}
	}
	target(prop.Get())
	return prop.OnChange(target)
}

// TwoWayBind creates a seamless 2-way binding between a Property[T] and an external control,
// eliminating infinite ping-pong notification echoes.
func TwoWayBind[T comparable](
	prop Property[T],
	get func() T,
	set func(T),
	listen func(onChange func(T)) (unlisten func()),
) func() {
	if prop == nil || get == nil || set == nil {
		return func() {}
	}
	isSyncing := false

	set(prop.Get())

	unsubProp := prop.OnChange(func(newVal T) {
		if isSyncing {
			return
		}
		if get() == newVal {
			return
		}
		isSyncing = true
		set(newVal)
		isSyncing = false
	})

	var unlistenExt func()
	if listen != nil {
		unlistenExt = listen(func(newExtVal T) {
			if isSyncing {
				return
			}
			if prop.Get() == newExtVal {
				return
			}
			isSyncing = true
			prop.Set(newExtVal)
			isSyncing = false
		})
	}

	return func() {
		if unsubProp != nil {
			unsubProp()
		}
		if unlistenExt != nil {
			unlistenExt()
		}
	}
}

type Disabler interface {
	SetDisabled(bool)
}

type VisibilitySetter interface {
	SetVisible(bool)
}

// BindEnabled binds a widget's enabled state to prop (enabled when true).
func BindEnabled(prop Property[bool], target Disabler) func() {
	if prop == nil || target == nil {
		return func() {}
	}
	return BindTo(prop, func(enabled bool) {
		target.SetDisabled(!enabled)
	})
}

// BindVisible binds a widget's visibility to prop.
func BindVisible(prop Property[bool], target VisibilitySetter) func() {
	if prop == nil || target == nil {
		return func() {}
	}
	return BindTo(prop, func(visible bool) {
		target.SetVisible(visible)
	})
}
