package vtui

import "github.com/unxed/vtui/vreactive"

// BindText binds the edit's text to a property in both directions: the text
// follows the property, and typing updates it. The property's current value is
// applied at once. The returned function undoes the binding and restores the
// edit's previous OnTextChange handler. Notification echoes between the two
// sides are suppressed by vreactive.TwoWayBind.
func (e *Edit) BindText(prop vreactive.Property[string]) func() {
	if e == nil || prop == nil {
		return func() {}
	}
	previous := e.OnTextChange
	return vreactive.TwoWayBind(prop, e.GetText, e.SetText, func(onChange func(string)) func() {
		e.OnTextChange = func(text string) {
			if previous != nil {
				previous(text)
			}
			onChange(text)
		}
		return func() { e.OnTextChange = previous }
	})
}

// BindChecked binds the checkbox to a boolean property in both directions:
// the box is checked while the property is true, and clicking it updates the
// property. The undefined state of a three-state checkbox reads as false. The
// returned function undoes the binding and restores the previous OnChange.
func (cb *Checkbox) BindChecked(prop vreactive.Property[bool]) func() {
	if cb == nil || prop == nil {
		return func() {}
	}
	previous := cb.OnChange
	return vreactive.TwoWayBind(prop,
		func() bool { return cb.State == 1 },
		func(checked bool) {
			if checked {
				cb.State = 1
			} else {
				cb.State = 0
			}
			cb.NotifyChange()
		},
		func(onChange func(bool)) func() {
			cb.OnChange = func(state int) {
				if previous != nil {
					previous(state)
				}
				onChange(state == 1)
			}
			return func() { cb.OnChange = previous }
		})
}

// BindText makes the text follow a property: the current value is applied at
// once and every later change is shown. The returned function undoes the
// binding.
func (t *Text) BindText(prop vreactive.Property[string]) func() {
	if t == nil || prop == nil {
		return func() {}
	}
	return vreactive.BindTo(prop, t.SetText)
}
