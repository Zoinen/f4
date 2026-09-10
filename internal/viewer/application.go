package viewer

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Application is what the viewer needs from the application above it.
//
// The viewer draws a file and navigates inside it; everything that reaches
// past the file — the action registry behind the menus, the hotkey manager
// behind the key bar, opening the editor on what is being viewed — belongs to
// the layer that owns those, and arrives here through this interface.
type Application interface {
	// MenuBarItems builds the menu for an area from the action registry. The
	// viewer asks on every draw, so shortcuts and toggle states stay current.
	MenuBarItems(area string) []vtui.MenuBarItem

	// KeyBarLabels resolves the bottom bar for an area, letting a user's
	// hotkeys.ini override the labels the caller supplies as fallbacks.
	KeyBarLabels(area string, fallbacks *vtui.KeySet) *vtui.KeySet

	// ActionForKey names the action bound to a key in an area, or "".
	ActionForKey(area, key string) string

	// HandleCommand runs a frame command the viewer does not own itself —
	// switching to the editor, opening the search dialog, forking a
	// workspace — and reports whether it did.
	HandleCommand(v *ViewerView, cmd int, args any) bool

	// LookupHotkey runs the action bound to e without touching macro
	// recording state. Key-bar clicks arrive through InjectEvents, which
	// bypasses the frame manager's event filter, so the viewer has to ask.
	LookupHotkey(e *vtinput.InputEvent) bool
}

// App is the live application. The composition root sets it once at startup;
// it is nil only in tests that never open a viewer, so every use is guarded.
var App Application
