package vtui

// NewLabel creates a Text object and links it to a focusable element.
// This is a convenience wrapper for NewText(x, y, content, color) + FocusLink.
func NewLabel(x, y int, content string, link UIElement) *Text {
	t := NewText(x, y, content, Palette[ColDialogText])
	t.FocusLink = link
	return t
}
