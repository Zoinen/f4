package editor

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// What the editor needs from the application above it. Each of these is one
// call site measured on the extraction wave, and each is here rather than
// imported because what answers it lives at layer 4: the action registry's
// runner reaches for the panels, the macro layer owns the hotkey manager, the
// menu bar is assembled from the whole action table.
//
// The root fills them in. Every default is inert rather than approximate — an
// unwired seam makes the editor decline the key, draw no crosshair and record
// no history, which is wrong but visibly wrong, and never quietly different
// from what the user asked for.
var (
	// RunAction runs a registered action by name and reports whether it was
	// handled. The editor uses it for the one action it raises itself.
	RunAction = func(name string) bool { return false }

	// LookupHotkey gives the macro layer first refusal on a key, so a chord the
	// user has bound to a macro is not eaten by the editor's own handling.
	LookupHotkey = func(e *vtinput.InputEvent) bool { return false }

	// MenuBarItems builds the menu bar for an area from the action table.
	MenuBarItems = func(area string) []vtui.MenuBarItem { return nil }

	// CrossAttrs says whether to draw the cursor crosshair and in which
	// colours. Off is the setting's own default.
	CrossAttrs = func() (horz, vert bool, horzAttr, vertAttr uint64) { return false, false, 0, 0 }

	// RememberEdited records a file in the edited-files history, which the
	// panels own.
	RememberEdited = func(v vfs.VFS, path string) {}

	// KeyBarLabels rewrites the editor's own function-key captions with
	// whatever the user has rebound. Unwired it returns them unchanged, which
	// is right: the fallbacks are what the editor would show anyway.
	KeyBarLabels = func(area string, fallbacks *vtui.KeySet) *vtui.KeySet { return fallbacks }

	// HotkeyAction names the action bound to a key in an area, or "" when none
	// is. The editor asks about one key: F8, to label it for the codepage it
	// would actually switch to.
	HotkeyAction = func(area, key string) string { return "" }

	// SaveSession persists the workspace layout after an edit that changes it.
	SaveSession = func() {}

	// HandleWorkspaceFork answers the fork command for the editor's frame, the
	// same way internal/fileops takes it for the queue.
	HandleWorkspaceFork = func(cmd int, args any) bool { return false }

	// SwitchToViewer hands the current file to the viewer. Opening a viewer is
	// the application's business — it owns the frame stack.
	SwitchToViewer = func(ev *EditorView) {}
)
